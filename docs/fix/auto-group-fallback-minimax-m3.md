# Auto-Group 智能熔断机制 Bug 排查与修复

> 分支:`fix/auto-group-fallback-minimax-m3`
> 作者:isboyjc
> 涉及文件:`service/channel_select.go`、`middleware/distributor.go`

---

## 一、问题描述

管理员在「分组与模型定价设置 → 分组相关设置 → 自动分组」中配置好自动分组的 JSON 顺序后,用户创建令牌时选择「auto」分组并开启「跨分组重试」,预期行为是:

> 使用此令牌调用某个模型时,会按照配置的分组顺序依次查看对应分组内有没有对应的模型;没有就继续找下一个分组;如果对应分组有此模型则调用,调用不成功就继续切换下一个分组;按 JSON 顺序找完所有自动分组后返回报错信息。

实际行为存在两类异常:

1. **「有时候遇到分组模型调用错误就直接返回了」** — 应当跨分组回退时,系统直接抛错。
2. **「有时候就没有找到对应的分组」** — 系统挑选的分组与用户预期不一致,或跳过了实际可用的分组。

---

## 二、根因分析

### 2.1 关键代码位置

| 模块 | 文件 | 关键函数 / 行 |
|---|---|---|
| 自动分组迭代引擎 | `service/channel_select.go` | `CacheGetRandomSatisfiedChannel` |
| 上游路由选择 | `controller/relay.go` | `getChannel` / `for ; retryParam.GetRetry() <= common.RetryTimes;` |
| 任务型重试 | `controller/relay.go` | `RelayTask` 中的 for 循环 |
| 中间件首次选渠道 | `middleware/distributor.go` | `Distribute` 内的亲和路径 / `service.CacheGetRandomSatisfiedChannel` 调用 |
| 渠道缓存 | `model/channel_cache.go` | `GetRandomSatisfiedChannel` |
| 上下文 Key | `constant/context_key.go` | `ContextKeyAutoGroupIndex` / `ContextKeyAutoGroupRetryIndex` |

外层重试 loop 的简化结构(取自 `controller/relay.go`):

```go
for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
    channel, channelErr := getChannel(c, relayInfo, retryParam) // -> service.CacheGetRandomSatisfiedChannel
    if channelErr != nil { break }
    newAPIError = relayHandler(c, relayInfo) // 实际调用上游
    if newAPIError == nil { return }
    if !shouldRetry(c, newAPIError, ...) { break }
}
```

`CacheGetRandomSatisfiedChannel` 在 `param.TokenGroup == "auto"` 时按下述 for 循环挑选渠道(修复前):

```go
for i := startGroupIndex; i < len(autoGroups); i++ {
    autoGroup := autoGroups[i]
    priorityRetry := param.GetRetry()                       // ← Bug 1
    if i > startGroupIndex { priorityRetry = 0 }
    channel, _ = model.GetRandomSatisfiedChannel(autoGroup, model, priorityRetry)
    if channel == nil {
        SetContextKey(AutoGroupIndex, i+1)
        SetContextKey(AutoGroupRetryIndex, 0)               // ← Bug 1
        param.SetRetry(0)
        continue
    }
    if crossGroupRetry && priorityRetry >= common.RetryTimes {
        SetContextKey(AutoGroupIndex, i+1)                  // ← 正确推进 index
        param.SetRetry(0); param.ResetRetryNextTry()        // ← 关键:重置
    } else {
        SetContextKey(AutoGroupIndex, i)                    // 停在当前分组
    }
    break
}
```

### 2.2 Bug 1(关键):`ContextKeyAutoGroupRetryIndex` 写而不读,`startRetryIndex` 跟踪是空壳

代码 docstring 写得很漂亮:

> Uses `ContextKeyAutoGroupRetryIndex` to track the global Retry count when current group started.
> `priorityRetry = Retry - startRetryIndex`, represents the priority level within current group.

但实现上:

- `ContextKeyAutoGroupRetryIndex` 只被 **写**(且永远写死 `0`),从未被 **读**。
- `priorityRetry` 实际取 `param.GetRetry()`(全局 retry),并没有减去 startRetryIndex。

为什么「`i > startGroupIndex` 时把 priorityRetry 设为 0」能暂时蒙混过关?

- **同一次调用内**的 fall-through(i > startGroupIndex),`i > startGroupIndex` 这个分支确实把新分组的优先级强制设成 0,所以 for 循环内部跨分组时是按「0 → 1 → 2」走的,看着没问题。
- **跨调用**(下一次 for 循环迭代)就出问题了:`Retry` 已经被 `IncreaseRetry()` 推到 `1`,而 `startRetryIndex` 没有被记录,`priorityRetry` 直接等于 `Retry=1`。结果:**新分组的优先级 0 被彻底跳过**,先尝试优先级 1(若不存在则 nil → fall through),优先级 2,……真正的可用渠道在优先级 0,被绕开。

举例(2 个分组,各 1 个优先级,`RetryTimes=3`,开启跨分组):

| 迭代 | Retry | 应得 priority | 实际 priority(修复前) | 实际 priority(修复后) | 实际选中的渠道 |
|---|---|---|---|---|---|
| 1 | 0 | GroupA p0 | GroupA p0 ✓ | GroupA p0 ✓ | GroupA 渠道 |
| 2 | 1 | GroupA p1(nil)→ GroupB p0 | GroupA p1(nil)→ **GroupB p1(nil)→ fall through 错** | GroupA p1(nil)→ GroupB p0 ✓ | GroupB 渠道 |

「错」那一列是修复前的行为:跨调用后直接用 `Retry=1` 跳过了 GroupB 优先级 0,导致 **GroupB 唯一的渠道永远选不到**,错误信息里只会出现 `group auto 下模型 X 不存在` 之类。这就是用户说的「有时候就没有找到对应的分组」。

### 2.3 Bug 2(关键):`Distribute` 亲和路径未设置 `AutoGroupIndex`

`middleware/distributor.go` 的亲和路径:

```go
} else if usingGroup == "auto" {
    autoGroups := service.GetUserAutoGroup(userGroup)
    for _, g := range autoGroups {                            // ← 没有 idx
        if model.IsChannelEnabledForGroupModel(g, model, preferred.Id) {
            selectGroup = g
            common.SetContextKey(c, constant.ContextKeyAutoGroup, g)   // 设置了 group
            // 缺:AutoGroupIndex / AutoGroupRetryIndex        // ← Bug 2
            channel = preferred
            service.MarkChannelAffinityUsed(c, g, preferred.Id)
            break
        }
    }
}
```

后果:如果用户的 affinity 命中的是「自动分组 JSON 链里靠后」的某个分组(比如 index=5),`Distribute` 第一次用了它,后续 relay controller 触发重试时 `CacheGetRandomSatisfiedChannel` 读到的 `AutoGroupIndex` 还是 `0`(默认),于是**从第一个分组重新走**,经常跳过真正该继续尝试的亲和分组,触发不必要的 fall-through 或者干脆选错分组。这直接对应用户报告的「有时候就没有找到对应的分组」。

### 2.4 Bug 3(UX):`GetUserAutoGroup` 空列表时,错误信息具有误导性

`CacheGetRandomSatisfiedChannel` 之前只校验 `setting.GetAutoGroups()` 非空,但没校验 `GetUserAutoGroup(userGroup)` 非空。当用户分组限制把所有配置的自动分组都过滤掉(比如管理员对某个用户组配置了 `+:groupA`、`-:groupB` 之类),`autoGroups` 为空,for 循环不执行,函数返回 `channel=nil, selectGroup="auto"`,最终错误信息是「**分组 auto 下模型 X 的可用渠道不存在**」 — 但 `auto` 根本不是一个真实的分组,这条错误对管理员排查问题没有任何帮助。

### 2.5 关于「直接返回」

`shouldRetry` 还会因为以下原因**主动 break**,不会切分组:

- 错误码命中 `operation_setting.IsAlwaysSkipRetryCode`(可由管理员配置)
- `IsSkipRetryError`(`GetChannelFailed`、`ErrOptionWithSkipRetry` 等)
- `service.ShouldSkipRetryAfterChannelAffinityFailure(c)`(亲和规则的 `SkipRetryOnFailure`)
- 上游返回 4xx 非可重试状态码(参见 `operation_setting.ShouldRetryByStatusCode`)

这些是**设计行为**而非 bug,但用户容易误以为是「跨分组熔断坏了」。建议后续在管理后台或文档中显式提示「跨分组重试仅在错误码被标记为可重试时生效」。

---

## 三、修复方案

### 3.1 `service/channel_select.go`

1. 真正读 `ContextKeyAutoGroupRetryIndex`,作为 `startRetryIndex`。
2. `priorityRetry` 计算改为 `param.GetRetry() - startRetryIndex`,下限 `0`。
3. `channel==nil` 与「exhausted」两个分支都把 `param.GetRetry()` 写入 `AutoGroupRetryIndex`,保证跨调用时新分组的优先级从 0 开始递增。
4. `channel==nil` 分支**故意不**调用 `param.SetRetry(0)` / `param.ResetRetryNextTry()` — 让外层 for 循环 `IncreaseRetry()` 正常推进,避免 `Retry` 卡在 0 导致 for 循环无界(直到 channel==nil 触发 `ErrOptionWithSkipRetry`)。
5. `GetUserAutoGroup` 返回空时立即返回明确错误。

### 3.2 `middleware/distributor.go`

亲和命中时同时设置 `ContextKeyAutoGroupIndex` 和 `ContextKeyAutoGroupRetryIndex`,把迭代器钉在该亲和分组,后续重试从该分组继续。

### 3.3 单元测试 `service/channel_select_test.go`

覆盖 `RetryParam.IncreaseRetry()` 在 `resetNextTry` 各种状态下的行为、上下文状态机、优先级计算下限,以及「空 autoGroups 错误」守卫。

---

## 四、验证

### 4.1 单元测试

```bash
$ go test -count=1 -run "TestAuto|TestRetry" ./service/...
ok  github.com/QuantumNous/new-api/service  1.011s
```

子测试全部通过:

- `TestRetryParamIncreaseRetry`(3 子测试)
- `TestAutoGroupContextStateTransitions`(4 子测试)
- `TestEmptyAutoGroupsGuard`

### 4.2 构建

```bash
$ go build ./...
$ go vet ./service/... ./middleware/...
(无输出,全部通过)
```

### 4.3 推荐手工回归(在你本地 dev 环境)

1. 准备 3 个分组 `g1/g2/g3`,把同一个模型 M 同时挂在 `g1`(priority=0)和 `g3`(priority=0),`g2` 不挂 M。
2. 把自动分组 JSON 配置为 `["g1","g2","g3"]`,创建 token,group=`auto`,cross_group_retry=true。
3. 关闭 g1 的渠道可用性(临时 disable),发请求:
   - 修复前:可能直接报「group auto 下模型 M 不存在」,或在 g2 反复重试。
   - 修复后:应回退到 g3,正确选到 g3 的渠道,日志里 `use_channel` 序列应包含 g3 渠道 ID。
4. 把 g1 重新启用,人为让 g1 渠道返回 500:
   - 修复后:第一次失败 → 进入 g3 → 成功;`use_channel` 序列应包含 g1、g3 两个渠道 ID。
5. 把所有渠道 disable,验证最终错误信息不再是「分组 auto 下 …」这种含糊措辞。

---

## 五、变更清单

| 文件 | 类型 | 行数变化 |
|---|---|---|
| `service/channel_select.go` | 修改 | +59 / −12 |
| `middleware/distributor.go` | 修改 | +12 / −1 |
| `service/channel_select_test.go` | 新增 | +119 |

---

## 六、后续建议(未在本次修改中处理)

1. `controller/relay.go` 的 `shouldRetry` 决定是否继续重试,但当前 4xx(400/401/403/404)被默认视作不可重试,而部分 4xx(尤其 429)需要重试。建议梳理 `operation_setting.ShouldRetryByStatusCode` 的默认配置,让跨分组熔断在「上游限流」场景下也能正常切分组。
2. 当前对 `ErrOptionWithSkipRetry()` 的传播比较激进,任何在 `getChannel` 里 `channel==nil` 的情况都会立刻终止重试。这对「全部自动分组都没有该模型」是对的,但对「中间分组模型暂时熔断」应当允许继续往下个分组走。可考虑在 `cache_get_channel_failed` 错误里区分「本分组无渠道」与「全部无渠道」两种情形。
3. 增加 e2e 集成测试,覆盖「自动分组回退 + 跨分组重试 + 亲和命中」组合场景,避免类似回归。
