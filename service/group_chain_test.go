package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 装配
// ---------------------------------------------------------------------------

// seedGroups 配置可用分组与分组倍率。ResolveGroupChain / ValidateGroupChain
// 都要过这两张表。
func seedGroups(t *testing.T, usableGroupsJSON string, groupRatioJSON string) {
	t.Helper()
	prevUsable := setting.UserUsableGroups2JSONString()
	prevRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(usableGroupsJSON))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groupRatioJSON))
	t.Cleanup(func() {
		_ = setting.UpdateUserUsableGroupsByJSONString(prevUsable)
		_ = ratio_setting.UpdateGroupRatioByJSONString(prevRatio)
	})
}

// seedAbilities 走数据库路径灌渠道与 ability。
// 内存索引在 model 包里是私有的，service 包够不着；DB 路径与内存路径共用同一套
// 遍历逻辑（selectFromChain），所以用它来验证遍历行为是等价的。
func seedAbilities(t *testing.T, abilities []model.Ability) {
	t.Helper()
	model.InitCommonColumnsForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}, &model.Channel{}))

	prevMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	channelIds := make(map[int]struct{})
	for i := range abilities {
		channelIds[abilities[i].ChannelId] = struct{}{}
	}
	for id := range channelIds {
		require.NoError(t, model.DB.Create(&model.Channel{
			Id: id, Status: common.ChannelStatusEnabled, Name: "test",
		}).Error)
	}
	require.NoError(t, model.DB.Create(&abilities).Error)

	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM abilities")
		model.DB.Exec("DELETE FROM channels")
		common.MemoryCacheEnabled = prevMemoryCache
	})
}

func ability(group, modelName string, channelID int, priority int64) model.Ability {
	p := priority
	return model.Ability{Group: group, Model: modelName, ChannelId: channelID, Enabled: true, Priority: &p}
}

func newTestContext(t *testing.T, chain GroupChain) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	SetupGroupChain(c, chain)
	return c
}

// selectOnce 走一次选路，返回 (渠道 ID, 分组, 是否换组)。-1 表示没选出渠道。
func selectOnce(c *gin.Context, modelName string) (int, string, bool) {
	channel, group, switched := selectFromChain(c, GetGroupChain(c), modelName)
	if channel == nil {
		return -1, group, switched
	}
	return channel.Id, group, switched
}

// ---------------------------------------------------------------------------
// 分组链解析：存量令牌的兼容路径是重点
// ---------------------------------------------------------------------------

func TestResolveGroupChainLegacyPathsUnchanged(t *testing.T) {
	seedGroups(t, `{"vip":"VIP","default":"默认","backup":"备用"}`, `{"vip":2,"default":1,"backup":0.5}`)
	prevAuto := setting.AutoGroups2JsonString()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["vip","default"]`))
	t.Cleanup(func() { _ = setting.UpdateAutoGroupsByJsonString(prevAuto) })

	t.Run("空分组回退到用户分组", func(t *testing.T) {
		chain, err := ResolveGroupChain(&model.Token{}, "default")
		require.NoError(t, err)
		assert.Equal(t, []string{"default"}, chain.Groups)
		assert.Equal(t, GroupChainSourceUserGroup, chain.Source)
	})

	t.Run("单分组令牌", func(t *testing.T) {
		chain, err := ResolveGroupChain(&model.Token{Group: "vip"}, "default")
		require.NoError(t, err)
		assert.Equal(t, []string{"vip"}, chain.Groups)
		assert.Equal(t, GroupChainSourceLegacySingle, chain.Source)
	})

	t.Run("auto 令牌运行时展开且跟随管理员配置", func(t *testing.T) {
		chain, err := ResolveGroupChain(&model.Token{Group: "auto", CrossGroupRetry: true}, "default")
		require.NoError(t, err)
		assert.Equal(t, []string{"vip", "default"}, chain.Groups)
		assert.Equal(t, GroupChainSourceLegacyAuto, chain.Source)
		assert.True(t, chain.CrossGroup)

		// 管理员调整顺序后，存量 auto 令牌必须跟着变 —— 这正是没有把 auto
		// 在迁移时固化成快照的原因。
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))
		chain, err = ResolveGroupChain(&model.Token{Group: "auto"}, "default")
		require.NoError(t, err)
		assert.Equal(t, []string{"default", "vip"}, chain.Groups)
	})
}

func TestResolveGroupChainExplicit(t *testing.T) {
	seedGroups(t, `{"vip":"VIP","default":"默认","backup":"备用"}`, `{"vip":2,"default":1,"backup":0.5}`)

	newChainToken := func(t *testing.T, groups []string) *model.Token {
		token := &model.Token{CrossGroupRetry: true}
		require.NoError(t, token.SetGroups(groups))
		return token
	}

	t.Run("保持用户配置的顺序", func(t *testing.T) {
		chain, err := ResolveGroupChain(newChainToken(t, []string{"backup", "vip", "default"}), "default")
		require.NoError(t, err)
		assert.Equal(t, []string{"backup", "vip", "default"}, chain.Groups)
		assert.Equal(t, GroupChainSourceExplicit, chain.Source)
	})

	t.Run("失效分组被跳过而不是整体失败", func(t *testing.T) {
		// 多选场景下，一个分组被回收权限不该让整个令牌不可用
		chain, err := ResolveGroupChain(newChainToken(t, []string{"vip", "已删除分组", "default"}), "default")
		require.NoError(t, err)
		assert.Equal(t, []string{"vip", "default"}, chain.Groups)
	})

	t.Run("全部失效返回错误而不是静默回退", func(t *testing.T) {
		// 静默回退到用户分组等于悄悄换了计费分组，必须报错
		_, err := ResolveGroupChain(newChainToken(t, []string{"不存在A", "不存在B"}), "default")
		require.Error(t, err)
	})
}

func TestTokenGroupsRoundTrip(t *testing.T) {
	token := &model.Token{}
	assert.Empty(t, token.GetGroups())

	require.NoError(t, token.SetGroups([]string{"a", "b"}))
	assert.Equal(t, `["a","b"]`, token.GroupsJSON)
	assert.Equal(t, []string{"a", "b"}, token.GetGroups())

	// 清空回到旧语义
	require.NoError(t, token.SetGroups(nil))
	assert.Equal(t, "", token.GroupsJSON)
	assert.Empty(t, token.GetGroups())

	// 脏数据不能让请求崩掉，降级成「没有显式链」
	token.GroupsJSON = "{not json"
	assert.Empty(t, token.GetGroups())

	// 去重与去空白
	require.NoError(t, token.SetGroups([]string{" a ", "a", "b"}))
	assert.Equal(t, []string{"a", "b"}, token.GetGroups())
}

func TestNormalizeAndValidateGroupChain(t *testing.T) {
	seedGroups(t, `{"vip":"VIP","default":"默认"}`, `{"vip":2,"default":1}`)

	assert.Equal(t, []string{"a", "b"}, NormalizeGroupChainInput([]string{" a", "a ", "", "b"}))

	require.NoError(t, ValidateGroupChain([]string{"vip", "default"}, "default"))
	require.Error(t, ValidateGroupChain([]string{"vip", "无权分组"}, "default"))

	prevMax := operation_setting.GetGroupChainSetting().MaxChainLength
	operation_setting.GetGroupChainSetting().MaxChainLength = 2
	t.Cleanup(func() { operation_setting.GetGroupChainSetting().MaxChainLength = prevMax })
	require.Error(t, ValidateGroupChain([]string{"vip", "default", "vip2"}, "default"))
}

// ---------------------------------------------------------------------------
// 选路遍历
// ---------------------------------------------------------------------------

func TestSelectFromChainDiscoversModelAcrossGroups(t *testing.T) {
	// 模型只存在于链尾分组。首次选路属于「模型发现」，不受跨分组开关限制，
	// 前面两个分组是纯内存查找零成本跳过的。
	seedAbilities(t, []model.Ability{ability("c", "gpt-4", 30, 0)})

	c := newTestContext(t, GroupChain{Groups: []string{"a", "b", "c"}, CrossGroup: false})
	channelID, group, switched := selectOnce(c, "gpt-4")
	assert.Equal(t, 30, channelID)
	assert.Equal(t, "c", group)
	assert.False(t, switched, "首次发现模型不算换组")
}

func TestSelectFromChainWalksPrioritiesThenNextGroup(t *testing.T) {
	seedAbilities(t, []model.Ability{
		ability("a", "gpt-4", 10, 100), // a 的高优先级
		ability("a", "gpt-4", 11, 50),  // a 的低优先级
		ability("b", "gpt-4", 20, 0),
	})

	c := newTestContext(t, GroupChain{Groups: []string{"a", "b"}, CrossGroup: true})

	id1, group1, sw1 := selectOnce(c, "gpt-4")
	assert.Equal(t, 10, id1)
	assert.Equal(t, "a", group1)
	assert.False(t, sw1)

	// 先把 a 组内的优先级走完，再换组
	id2, group2, sw2 := selectOnce(c, "gpt-4")
	assert.Equal(t, 11, id2)
	assert.Equal(t, "a", group2)
	assert.False(t, sw2, "组内换优先级不算换组，不该额外授予重试预算")

	id3, group3, sw3 := selectOnce(c, "gpt-4")
	assert.Equal(t, 20, id3)
	assert.Equal(t, "b", group3)
	assert.True(t, sw3, "跨到下一个分组要算一次换组")

	// 全部候选用尽
	id4, _, _ := selectOnce(c, "gpt-4")
	assert.Equal(t, -1, id4)
}

func TestSelectFromChainNeverRepeatsTriedChannel(t *testing.T) {
	// 同一优先级层里有三个渠道。老实现只按权重随机、不看已尝试集合，
	// 重试会反复抽中刚失败的那个 —— 这是「自动切换看起来没生效」的主因。
	seedAbilities(t, []model.Ability{
		ability("a", "gpt-4", 10, 0),
		ability("a", "gpt-4", 11, 0),
		ability("a", "gpt-4", 12, 0),
	})

	c := newTestContext(t, GroupChain{Groups: []string{"a"}, CrossGroup: true})
	seen := map[int]bool{}
	for i := 0; i < 3; i++ {
		id, _, _ := selectOnce(c, "gpt-4")
		require.NotEqual(t, -1, id)
		assert.False(t, seen[id], "渠道 %d 被重复选中", id)
		seen[id] = true
	}
	assert.Len(t, seen, 3)
}

func TestSelectFromChainRespectsCrossGroupSwitch(t *testing.T) {
	seedAbilities(t, []model.Ability{
		ability("a", "gpt-4", 10, 0),
		ability("b", "gpt-4", 20, 0),
	})

	c := newTestContext(t, GroupChain{Groups: []string{"a", "b"}, CrossGroup: false})

	id1, group1, _ := selectOnce(c, "gpt-4")
	assert.Equal(t, 10, id1)
	assert.Equal(t, "a", group1)

	// 关掉跨分组开关后，a 用尽就到此为止，不回落到 b
	id2, _, _ := selectOnce(c, "gpt-4")
	assert.Equal(t, -1, id2)
}

func TestSelectFromChainBlockedCrossGroupStaysInGroup(t *testing.T) {
	seedAbilities(t, []model.Ability{
		ability("a", "gpt-4", 10, 100),
		ability("a", "gpt-4", 11, 50),
		ability("b", "gpt-4", 20, 0),
	})

	c := newTestContext(t, GroupChain{Groups: []string{"a", "b"}, CrossGroup: true})

	id1, _, _ := selectOnce(c, "gpt-4")
	assert.Equal(t, 10, id1)

	// 参数非法 / 内容审核这类错误换分组也是同样的失败，只在组内继续
	SetCrossGroupBlocked(c, true)
	id2, group2, _ := selectOnce(c, "gpt-4")
	assert.Equal(t, 11, id2, "组内低优先级仍应尝试")
	assert.Equal(t, "a", group2)

	id3, _, _ := selectOnce(c, "gpt-4")
	assert.Equal(t, -1, id3, "被阻止跨组时不该回落到 b")
}

func TestSelectFromChainCoolingIsPreferenceNotHardExclusion(t *testing.T) {
	seedAbilities(t, []model.Ability{
		ability("a", "gpt-4", 10, 0),
		ability("a", "gpt-4", 11, 0),
	})
	ResetChannelHealth()
	t.Cleanup(ResetChannelHealth)

	// 把 10 打进冷却
	threshold := operation_setting.GetGroupChainSetting().HealthThreshold
	for i := 0; i < threshold; i++ {
		MarkChannelFailure(10, "gpt-4")
	}
	assert.True(t, IsChannelCooling(10, "gpt-4"))

	c := newTestContext(t, GroupChain{Groups: []string{"a"}, CrossGroup: true})
	id1, _, _ := selectOnce(c, "gpt-4")
	assert.Equal(t, 11, id1, "冷却中的渠道应被跳过")

	// 关键安全阀：剩下的候选全在冷却中时，必须放宽约束继续选，
	// 否则一次全站抖动会让所有请求直接失败 —— 比不做熔断更糟。
	id2, _, _ := selectOnce(c, "gpt-4")
	assert.Equal(t, 10, id2, "候选全部冷却时必须放宽熔断而不是直接失败")

	// 成功后立即恢复
	MarkChannelSuccess(10, "gpt-4")
	assert.False(t, IsChannelCooling(10, "gpt-4"))
}

func TestSelectFromChainSkipsGroupsWithoutModel(t *testing.T) {
	// 链上大量分组没有目标模型时，不应影响可用性，也不消耗任何预算
	seedAbilities(t, []model.Ability{
		ability("a", "claude-3", 10, 0),
		ability("b", "claude-3", 11, 0),
		ability("z", "gpt-4", 99, 0),
	})

	c := newTestContext(t, GroupChain{Groups: []string{"a", "b", "z"}, CrossGroup: false})
	id, group, _ := selectOnce(c, "gpt-4")
	assert.Equal(t, 99, id)
	assert.Equal(t, "z", group)
}

// ---------------------------------------------------------------------------
// 重试预算与跨组回退策略
// ---------------------------------------------------------------------------

func TestRetryParamBudgetGrantsExtraOnGroupSwitch(t *testing.T) {
	prev := common.RetryTimes
	common.RetryTimes = 2
	t.Cleanup(func() { common.RetryTimes = prev })

	param := &RetryParam{Retry: common.GetPointer(0)}
	assert.Equal(t, 2, param.Budget())

	// 换组不该消耗分组内的重试预算，否则链首分组把次数用光后，
	// 后面的分组一次机会都拿不到
	param.GroupSwitches = 2
	assert.Equal(t, 4, param.Budget())
}

func TestAllowCrossGroupFallback(t *testing.T) {
	// 可用性类错误：值得换分组
	assert.True(t, AllowCrossGroupFallback(
		types.NewOpenAIError(assertError("upstream busy"), types.ErrorCodeBadResponseStatusCode, 429)))
	assert.True(t, AllowCrossGroupFallback(
		types.NewOpenAIError(assertError("bad gateway"), types.ErrorCodeBadResponseStatusCode, 502)))
	assert.True(t, AllowCrossGroupFallback(nil))

	// 请求本身有问题：换多少个分组都是同样的失败，只会白烧额度和时延
	assert.False(t, AllowCrossGroupFallback(
		types.NewOpenAIError(assertError("bad param"), types.ErrorCodeBadResponseStatusCode, 400)))
	assert.False(t, AllowCrossGroupFallback(
		types.NewOpenAIError(assertError("blocked"), types.ErrorCodeSensitiveWordsDetected, 500)))
}

func TestRetryBudgetDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	prev := operation_setting.GetGroupChainSetting().TotalRetryBudgetMs
	t.Cleanup(func() { operation_setting.GetGroupChainSetting().TotalRetryBudgetMs = prev })

	// 未配置时间预算 → 不写截止时刻，永不判定超时（线上默认就是这个形态）
	operation_setting.GetGroupChainSetting().TotalRetryBudgetMs = 0
	StartRetryBudget(c)
	_, ok := common.GetContextKey(c, constant.ContextKeyRetryDeadline)
	assert.False(t, ok)
	assert.False(t, RetryBudgetExhausted(c))

	// 配置预算 → 写入截止时刻；等它过去后判定耗尽
	operation_setting.GetGroupChainSetting().TotalRetryBudgetMs = 1
	StartRetryBudget(c)
	deadline, ok := common.GetContextKey(c, constant.ContextKeyRetryDeadline)
	require.True(t, ok)
	require.NotNil(t, deadline)
	assert.Eventually(t, func() bool { return RetryBudgetExhausted(c) }, time.Second, 2*time.Millisecond)
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func assertError(msg string) error { return simpleError(msg) }

// TestApplyGroupChainAbsentFieldSemantics 覆盖「字段缺省 vs 显式清空」。
// 这个区分很重要：没有它，任何不带 groups 字段的老客户端 PUT 一次令牌，
// 就会把用户配好的分组链静默抹掉。
func TestGroupChainAbsentVsExplicitClear(t *testing.T) {
	token := &model.Token{}
	require.NoError(t, token.SetGroups([]string{"vip", "default"}))
	assert.Equal(t, []string{"vip", "default"}, token.GetGroups())

	// 显式空数组 → 清空
	require.NoError(t, token.SetGroups([]string{}))
	assert.Empty(t, token.GetGroups())
}
