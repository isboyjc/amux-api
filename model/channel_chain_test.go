package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedMemoryChannelCache 直接灌内存索引，绕开数据库。
// 生产环境只要启用了 Redis，MemoryCacheEnabled 就会被自动置 true（见 main.go），
// 所以内存路径才是真正跑在线上的那条。
func seedMemoryChannelCache(t *testing.T, channels []*Channel, index map[string]map[string][]int) {
	t.Helper()
	channelSyncLock.Lock()
	prevChannels := channelsIDM
	prevIndex := group2model2channels
	idm := make(map[int]*Channel, len(channels))
	for _, channel := range channels {
		idm[channel.Id] = channel
	}
	channelsIDM = idm
	group2model2channels = index
	channelSyncLock.Unlock()

	prevMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true

	t.Cleanup(func() {
		channelSyncLock.Lock()
		channelsIDM = prevChannels
		group2model2channels = prevIndex
		channelSyncLock.Unlock()
		common.MemoryCacheEnabled = prevMemoryCache
	})
}

func testChannel(id int, priority int64, weight int) *Channel {
	p := priority
	w := uint(weight)
	return &Channel{Id: id, Priority: &p, Weight: &w, Status: common.ChannelStatusEnabled}
}

func TestGetPriorityLevelCount(t *testing.T) {
	seedMemoryChannelCache(t,
		[]*Channel{
			testChannel(1, 100, 0),
			testChannel(2, 100, 0),
			testChannel(3, 50, 0),
			testChannel(4, 10, 0),
		},
		map[string]map[string][]int{
			"vip":     {"gpt-4": {1, 2, 3, 4}},
			"default": {"gpt-4": {1}},
		},
	)

	// 三个不同优先级 → 三层
	assert.Equal(t, 3, GetPriorityLevelCount("vip", "gpt-4"))
	assert.Equal(t, 1, GetPriorityLevelCount("default", "gpt-4"))

	// 分组里没有这个模型 / 分组不存在 → 0。调用方据此零成本跳过该分组，
	// 这是链长几乎不影响性能的关键。
	assert.Equal(t, 0, GetPriorityLevelCount("vip", "claude-3"))
	assert.Equal(t, 0, GetPriorityLevelCount("nonexistent", "gpt-4"))
	assert.Equal(t, 0, GetPriorityLevelCount("", "gpt-4"))
	assert.Equal(t, 0, GetPriorityLevelCount("vip", ""))
}

func TestPickChannelAtLevelWalksPrioritiesAndStopsAtEnd(t *testing.T) {
	seedMemoryChannelCache(t,
		[]*Channel{
			testChannel(1, 100, 0),
			testChannel(2, 50, 0),
		},
		map[string]map[string][]int{"vip": {"gpt-4": {1, 2}}},
	)

	first, err := PickChannelAtLevel("vip", "gpt-4", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 1, first.Id, "level 0 应命中最高优先级渠道")

	second, err := PickChannelAtLevel("vip", "gpt-4", 1, nil)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2, second.Id, "level 1 应命中次高优先级渠道")

	// 关键回归：老的 GetRandomSatisfiedChannel 在 retry 超出层数时会 clamp 到最低
	// 优先级并永远返回渠道，导致调用方无法区分「还有下一层」和「这个分组用完了」，
	// 分组链因此永远切不到下一个分组。这里必须如实返回 nil。
	beyond, err := PickChannelAtLevel("vip", "gpt-4", 2, nil)
	require.NoError(t, err)
	assert.Nil(t, beyond, "超出优先级层数必须返回 nil 而不是 clamp 到最低优先级")
}

func TestPickChannelAtLevelSkipPredicate(t *testing.T) {
	seedMemoryChannelCache(t,
		[]*Channel{
			testChannel(1, 100, 0),
			testChannel(2, 100, 0),
			testChannel(3, 100, 0),
		},
		map[string]map[string][]int{"vip": {"gpt-4": {1, 2, 3}}},
	)

	// 同一优先级层里排除掉两个，只可能命中剩下那个。
	// 老实现只按权重随机、不看已尝试集合，重试反复抽中刚失败的渠道 —— 这是
	// 「自动切换看起来没生效」的最大来源。
	skip := func(channelID int) bool { return channelID == 1 || channelID == 2 }
	for i := 0; i < 20; i++ {
		channel, err := PickChannelAtLevel("vip", "gpt-4", 0, skip)
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, 3, channel.Id)
	}

	// 整层都被排除 → nil，调用方继续走下一层 / 下一个分组
	all := func(channelID int) bool { return true }
	channel, err := PickChannelAtLevel("vip", "gpt-4", 0, all)
	require.NoError(t, err)
	assert.Nil(t, channel)
}

func TestPickChannelAtLevelNormalizesModelName(t *testing.T) {
	seedMemoryChannelCache(t,
		[]*Channel{testChannel(1, 0, 0)},
		map[string]map[string][]int{"vip": {"gpt-4-gizmo-*": {1}}},
	)

	// gpts 形态的模型名要能回退到归一化后的名字，与 GetRandomSatisfiedChannel 一致
	assert.Equal(t, 1, GetPriorityLevelCount("vip", "gpt-4-gizmo-abc123"))
	channel, err := PickChannelAtLevel("vip", "gpt-4-gizmo-abc123", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 1, channel.Id)
}

func TestPickChannelByWeightRespectsWeights(t *testing.T) {
	// 权重为 0 时等权随机，不能永远返回同一个
	candidates := []*Channel{testChannel(1, 0, 0), testChannel(2, 0, 0)}
	seen := map[int]bool{}
	for i := 0; i < 200; i++ {
		seen[pickChannelByWeight(candidates).Id] = true
	}
	assert.Len(t, seen, 2, "权重全 0 时两个渠道都应被选中过")

	assert.Nil(t, pickChannelByWeight(nil))
	single := []*Channel{testChannel(7, 0, 5)}
	assert.Equal(t, 7, pickChannelByWeight(single).Id)
}

func TestGetPriorityLevelCountDBPath(t *testing.T) {
	InitCommonColumnsForTest()
	require.NoError(t, DB.AutoMigrate(&Ability{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM abilities") })

	prev := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = prev })

	p100, p50 := int64(100), int64(50)
	require.NoError(t, DB.Create(&[]Ability{
		{Group: "vip", Model: "gpt-4", ChannelId: 1, Enabled: true, Priority: &p100},
		{Group: "vip", Model: "gpt-4", ChannelId: 2, Enabled: true, Priority: &p50},
		{Group: "vip", Model: "gpt-4", ChannelId: 3, Enabled: false, Priority: &p50},
	}).Error)

	assert.Equal(t, 2, GetPriorityLevelCount("vip", "gpt-4"), "禁用的 ability 不计入层数")
	assert.Equal(t, 0, GetPriorityLevelCount("vip", "claude-3"))
}

// TestTokenUpdatePersistsGroupChain 验证分组链的写入与清空真的落库了。
//
// 特别是清空：Token.Update() 用 Select("...","group_chain",...) 指定字段，
// 依赖 GORM「Select 里列出的字段即使是零值也写入」这个语义，并且依赖用
// 列名（group_chain）而不是 Go 字段名（GroupsJSON）也能被正确匹配。
// 这两条任何一条不成立，清空分组链就会静默失败 —— 用户以为改了、其实没改。
func TestTokenUpdatePersistsGroupChain(t *testing.T) {
	InitCommonColumnsForTest()
	t.Cleanup(func() { DB.Exec("DELETE FROM tokens") })

	token := &Token{Name: "chain-persist", Key: "k-chain-persist", UserId: 1, Group: "vip"}
	require.NoError(t, token.SetGroups([]string{"vip", "default"}))
	require.NoError(t, token.Insert())

	reloaded, err := GetTokenById(token.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{"vip", "default"}, reloaded.GetGroups())

	// 改链
	require.NoError(t, reloaded.SetGroups([]string{"default", "vip"}))
	require.NoError(t, reloaded.Update())
	again, err := GetTokenById(token.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{"default", "vip"}, again.GetGroups())

	// 清空：零值必须被写入，否则老链还在，group 与链首对不上
	require.NoError(t, again.SetGroups(nil))
	require.NoError(t, again.Update())
	cleared, err := GetTokenById(token.Id)
	require.NoError(t, err)
	assert.Empty(t, cleared.GetGroups(), "清空分组链必须真的落库")
	assert.Equal(t, "", cleared.GroupsJSON)
}
