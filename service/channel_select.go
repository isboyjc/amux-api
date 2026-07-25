package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// 分组链选路。
//
// 替换掉原先只服务于 auto 分组的状态机（ContextKeyAutoGroupIndex /
// AutoGroupRetryIndex）。原实现有三个硬伤，这里一并修掉：
//
//  1. 用 param.SetRetry(0) 清零外层重试计数器来实现「换组」，导致
//     controller/relay.go 的 `retry <= RetryTimes` 预算失效，实际尝试次数变成
//     分组数 ×(RetryTimes+1)，并且传给 shouldRetry 的「剩余次数」永远是满值。
//     现在换组通过 GroupSwitches 显式授予额外预算，计数器不再被篡改。
//
//  2. 「分组内优先级用尽才换组」从未真正生效：GetRandomSatisfiedChannel 在 retry
//     超出优先级层数时会 clamp 到最低优先级，永远返回渠道而不是 nil。现在改用
//     model.GetPriorityLevelCount 拿真实层数。
//
//  3. 重试不排除已失败的渠道，同一优先级层里反复抽中刚失败的那个渠道，重试等于
//     空转。现在 tried 集合真正参与筛选。
//
// 游标放在 gin.Context 里：第 0 次尝试由 middleware/distributor.go 选渠道
// （此时 relay 的 ChannelMeta 还是 nil，controller/relay.go 的 getChannel 会直接
// 复用 distributor 的选择），第 1..N 次才由 relay 循环选，两边必须共享同一个游标。

type chainCursor struct {
	GroupIdx int
	LevelIdx int
}

type RetryParam struct {
	Ctx        *gin.Context
	TokenGroup string
	ModelName  string
	Retry      *int

	// GroupSwitches 本次请求已发生的换组次数。换组不消耗分组内的重试预算：
	// 一个分组把自己的重试次数用光，不应该让链上后面的分组一次机会都没有。
	// 每换一次组，外层循环的预算 +1，总上界仍然确定（RetryTimes + 换组次数），
	// 真正的尾延迟上界由 TotalRetryBudgetMs 时间预算兜底。
	GroupSwitches int
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

// Budget 返回当前允许的最大 retry 序号（含）。
//
// 三部分相加：
//
//	RetryTimes        管理员配置的分组内重试次数
//	GroupSwitches     已发生的换组次数（换组不消耗组内预算，见字段注释）
//	pendingSwitch     还能换组时预留的 1 次
//
// 最后一项是必需的：GroupSwitches 是「换组之后」才增加的，如果没有预留，
// RetryTimes=0 的部署（这就是本项目的默认值）永远走不到第 1 次重试，也就永远
// 触发不了第 1 次换组 —— 用户明明打开了跨分组重试却完全不生效。跨分组回落是
// 用户在令牌上显式开启的能力，不该被一个管理员侧的重试次数配置卡死。
//
// 循环仍然收敛：预留只在「还有分组可换」时给出，而已尝试渠道集合是硬约束，
// 候选终会耗尽让选路返回 nil，再加上 TotalRetryBudgetMs 时间预算兜底。
func (p *RetryParam) Budget() int {
	return common.RetryTimes + p.GroupSwitches + p.pendingSwitchAllowance()
}

func (p *RetryParam) pendingSwitchAllowance() int {
	if p.Ctx == nil {
		return 0
	}
	chain := GetGroupChain(p.Ctx)
	if !chain.CrossGroup || len(chain.Groups) <= 1 {
		return 0
	}
	// 上一次的错误不值得换组（参数非法、内容审核）时不预留
	if isCrossGroupBlocked(p.Ctx) {
		return 0
	}
	if p.GroupSwitches >= len(chain.Groups)-1 {
		return 0
	}
	return 1
}

// SetupGroupChain 把解析好的分组链写入 context，由 middleware/auth.go 在拿到令牌
// 时调用。
func SetupGroupChain(c *gin.Context, chain GroupChain) {
	if c == nil || chain.IsEmpty() {
		return
	}
	common.SetContextKey(c, constant.ContextKeyGroupChain, chain)
}

// GetGroupChain 读取本次请求的分组链。
//
// 没有链的情况有两种，都回退到 ContextKeyUsingGroup 构造单元素链：
//   - 操练场路径（/pg/*）走 UserAuth，根本没有令牌；且它允许在 body 里带 group
//     覆盖 usingGroup，覆盖发生在 distributor 里、选路之前。
//   - 任何未经 TokenAuth 的内部调用。
//
// 回退结果会写回 context，保证同一请求内多次选路拿到的是同一条链、同一个游标。
func GetGroupChain(c *gin.Context) GroupChain {
	if c == nil {
		return GroupChain{}
	}
	if v, ok := common.GetContextKey(c, constant.ContextKeyGroupChain); ok {
		if chain, ok := v.(GroupChain); ok {
			return chain
		}
	}
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if usingGroup == "" {
		return GroupChain{}
	}
	chain := GroupChain{
		Groups: []string{usingGroup},
		Source: GroupChainSourceUserGroup,
	}
	common.SetContextKey(c, constant.ContextKeyGroupChain, chain)
	return chain
}

// OverrideGroupChain 强制替换本次请求的分组链并重置游标。操练场在 body 里显式
// 指定 group 时使用。
func OverrideGroupChain(c *gin.Context, chain GroupChain) {
	if c == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeyGroupChain, chain)
	common.SetContextKey(c, constant.ContextKeyGroupChainCursor, chainCursor{})
}

func getChainCursor(c *gin.Context) chainCursor {
	if v, ok := common.GetContextKey(c, constant.ContextKeyGroupChainCursor); ok {
		if cursor, ok := v.(chainCursor); ok {
			return cursor
		}
	}
	return chainCursor{}
}

func setChainCursor(c *gin.Context, cursor chainCursor) {
	common.SetContextKey(c, constant.ContextKeyGroupChainCursor, cursor)
}

// GetTriedChannels 返回本次请求已经尝试过的渠道集合。
func GetTriedChannels(c *gin.Context) map[int]struct{} {
	if c == nil {
		return nil
	}
	if v, ok := common.GetContextKey(c, constant.ContextKeyTriedChannels); ok {
		if tried, ok := v.(map[int]struct{}); ok {
			return tried
		}
	}
	return nil
}

// MarkChannelTried 记录一个已尝试过的渠道，使其在后续选路中被跳过。
//
// 渠道亲和命中时 distributor 不走选路函数，必须由 distributor 显式调用这个函数
// 播种，否则重试会重新选中刚刚失败的那个亲和渠道。
func MarkChannelTried(c *gin.Context, channelID int) {
	if c == nil || channelID <= 0 {
		return
	}
	tried := GetTriedChannels(c)
	if tried == nil {
		tried = make(map[int]struct{}, 4)
		common.SetContextKey(c, constant.ContextKeyTriedChannels, tried)
	}
	tried[channelID] = struct{}{}
}

// SeedChainCursorForChannel 让游标对齐到某个已被外部选中的渠道所在的分组，并把
// 该渠道记入已尝试集合。渠道亲和命中时 distributor 不走 selectFromChain，必须靠
// 这个函数播种，否则重试会从链首重新扫描并可能再次选中刚失败的那个亲和渠道。
//
// 返回该渠道在链上所属的分组（取链上最靠前的匹配分组，保证计费分组稳定）；
// 链上没有任何分组包含该渠道时返回空串，此时不做任何标记 —— 调用方不会使用它。
func SeedChainCursorForChannel(c *gin.Context, modelName string, channelID int) string {
	chain := GetGroupChain(c)
	for idx, group := range chain.Groups {
		if model.IsChannelEnabledForGroupModel(group, modelName, channelID) {
			setChainCursor(c, chainCursor{GroupIdx: idx, LevelIdx: 0})
			MarkChannelTried(c, channelID)
			common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
			return group
		}
	}
	return ""
}

// CacheGetRandomSatisfiedChannel 按分组链选出下一个候选渠道。
//
// 返回的第二个值是本次实际命中的分组，调用方用它拼错误信息；命中的分组同时被写入
// ContextKeyAutoGroup，计费（relay/helper/price.go）与日志据此取分组倍率。
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	if param == nil || param.Ctx == nil {
		return nil, "", fmt.Errorf("invalid retry param")
	}
	chain := GetGroupChain(param.Ctx)
	if chain.IsEmpty() {
		return nil, param.TokenGroup, fmt.Errorf("无法确定请求分组")
	}

	channel, group, switched := selectFromChain(param.Ctx, chain, param.ModelName)
	if channel == nil {
		return nil, chain.String(), nil
	}
	if switched {
		param.GroupSwitches++
		common.SetContextKey(param.Ctx, constant.ContextKeyGroupSwitches, param.GroupSwitches)
	}
	common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, group)
	return channel, group, nil
}

// selectFromChain 是选路的核心。
//
// 分两个阶段依次放宽约束，每个阶段都从当前游标开始扫描整条链：
//
//	阶段 1：跳过「本次已尝试过」和「熔断冷却中」的渠道
//	阶段 2：只跳过「本次已尝试过」的（忽略熔断）
//
// 阶段 2 是熔断的安全阀：熔断只能作为优先级，不能是硬排除。如果全站抖动导致链上
// 所有渠道都在冷却，硬排除会让请求直接失败，比不做熔断还糟。
//
// 没有「忽略已尝试集合」的第三阶段：重新选中刚失败的渠道正是本次要修掉的问题，
// 已尝试集合是硬约束。候选耗尽就如实返回 nil，由调用方结束重试。
//
// 游标在选中渠道后停在原优先级层不动：同一层里通常有多个渠道，层是否耗尽由
// PickChannelAtLevel 返回 nil 来判定，而不是由「选过一次」来判定。
func selectFromChain(c *gin.Context, chain GroupChain, modelName string) (*model.Channel, string, bool) {
	cursor := getChainCursor(c)
	tried := GetTriedChannels(c)

	// 还没消耗过任何渠道时，沿着链往后找「哪个分组有这个模型」属于首次选路，
	// 不受跨分组开关限制 —— 跨分组开关控制的是失败后的回落，不是模型发现。
	// 首次选路之后，还要看上一次的错误是否值得换组（参数非法、内容审核这类
	// 换多少个分组都是同样的失败）。
	allowGroupAdvance := len(tried) == 0 || (chain.CrossGroup && !isCrossGroupBlocked(c))

	isTried := func(channelID int) bool {
		_, ok := tried[channelID]
		return ok
	}

	skipFuncs := []func(channelID int) bool{
		func(channelID int) bool { return isTried(channelID) || IsChannelCooling(channelID, modelName) },
		isTried,
	}

	for phase, skip := range skipFuncs {
		for groupIdx := cursor.GroupIdx; groupIdx < len(chain.Groups); groupIdx++ {
			if groupIdx > cursor.GroupIdx && !allowGroupAdvance {
				break
			}
			group := chain.Groups[groupIdx]

			// 纯内存索引查找，没有该模型的分组零成本跳过：不发请求、不消耗预算。
			// 这也是链长几乎不影响性能的原因。
			levelCount := model.GetPriorityLevelCount(group, modelName)
			if levelCount == 0 {
				continue
			}

			startLevel := 0
			if groupIdx == cursor.GroupIdx {
				startLevel = cursor.LevelIdx
			}
			for level := startLevel; level < levelCount; level++ {
				channel, err := model.PickChannelAtLevel(group, modelName, level, skip)
				if err != nil {
					logger.LogError(c, fmt.Sprintf("pick channel failed, group=%s model=%s level=%d: %s",
						group, modelName, level, err.Error()))
					continue
				}
				if channel == nil {
					continue
				}
				switched := groupIdx > cursor.GroupIdx && len(tried) > 0
				// 停在当前层：同层可能还有别的渠道，层耗尽由 PickChannelAtLevel
				// 返回 nil 判定。推进到 level+1 会把同层剩余候选整批跳过。
				setChainCursor(c, chainCursor{GroupIdx: groupIdx, LevelIdx: level})
				MarkChannelTried(c, channel.Id)
				if phase > 0 {
					logger.LogDebug(c, fmt.Sprintf("chain select relaxed to phase %d, group=%s model=%s channel=%d",
						phase, group, modelName, channel.Id))
				}
				return channel, group, switched
			}
		}
	}
	return nil, "", false
}
