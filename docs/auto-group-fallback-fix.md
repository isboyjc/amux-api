# Auto 分组跨分组熔断机制 — 问题诊断与修复方案

> 分支:`fix/auto-group-fallback-claude-opus4.8`
> 模型标识:claude / opus 4.8

## 1. 背景与预期

在「设置 / 分组与模型定价设置 / 分组相关设置 / 自动分组」中开启"默认使用 auto 分组"并配置一个**有序的分组 JSON 列表**后,用户创建令牌时可选择 `auto` 分组并开启「跨分组尝试」(`CrossGroupRetry`)。

**预期行为(智能熔断):** 用此令牌调用某模型时,按配置的分组顺序依次:

1. 看当前分组内有没有该模型,没有就跳到下一个分组;
2. 当前分组有该模型则调用;
3. 调用不成功则切换到下一个分组继续查找;
4. 直到所有自动分组按 JSON 顺序全部试完才返回报错。

**实际现象(异常):**

- 有时遇到分组模型调用错误就直接返回了,没有继续切下一个分组;
- 有时没有找到对应的分组。

## 2. 调用链

```
middleware/auth.go        → 设置 ContextKeyTokenGroup="auto"、ContextKeyTokenCrossGroupRetry
middleware/distributor.go → 第一次选渠道(CacheGetRandomSatisfiedChannel),写入 AutoGroupIndex
controller/relay.go 重试循环 → for { getChannel → 失败则 shouldRetry 决定是否继续 }
service/channel_select.go → CacheGetRandomSatisfiedChannel:真正的"按分组顺序熔断"逻辑
model/channel_cache.go    → GetRandomSatisfiedChannel:在单个分组内按优先级选渠道
service/group.go          → GetUserAutoGroup:把配置的 auto 列表与用户可用分组求交集
```

## 3. 问题诊断

### 🔴 Bug A(主因)— 跨分组回退与"分组内重试"共用同一个 `shouldRetry` 闸门

`controller/relay.go`(修复前):

```go
if !shouldRetry(c, newAPIError, common.RetryTimes-retryParam.GetRetry()) {
    break   // 整个熔断链直接终止,根本不会切下一个分组
}
```

`shouldRetry` 只对**可重试的 HTTP 状态码**返回 true。默认配置(`setting/operation_setting/status_code_ranges.go`)下,以下状态码**不重试**:`400`、`408`、`504`、`524`,以及任何 `SkipRetry` 错误、`ErrorCodeBadResponseBody`。

**后果:** 只要当前分组的渠道返回上述任一错误,循环立即 `break` 返回报错,**完全不会尝试后面的分组**——即便后面的分组里有可用的同名模型。这正是两个现象的根源:

- "遇到分组模型调用错误就直接返回了" ← 上游返回 400/504 等;
- "有时没找到对应的分组" ← 第一个分组先报了个不可重试的错,后面的组没机会被命中。

### 🔴 Bug B — `GetRandomSatisfiedChannel` 钳制 retry,导致文档描述的"优先级耗尽→切组"是死代码

`model/channel_cache.go`:

```go
if retry >= len(uniquePriorities) {
    retry = len(uniquePriorities) - 1   // 钳制,永远不会因优先级耗尽返回 nil
}
```

只要分组里对该模型**有任意一个渠道**,它就永远返回 channel,绝不会因优先级耗尽返回 `nil`(nil 只发生在"该组完全没有这个模型"时)。

而修复前的 `channel_select.go` 注释与分支都假设它会返回 nil 来触发切组。实际切组完全由 `priorityRetry >= RetryTimes` 驱动:

- 一个只有 1 个渠道的分组也会被**硬打 `RetryTimes+1` 次**才切走;
- 每次切组都 `param.SetRetry(0)` 重置了全局计数,导致 `RetryTimes` 全局上限被静默绕过,总尝试次数变成 `分组数 ×(RetryTimes+1)`。

### 🟠 Bug C — 切组时还会把"已确认坏掉"的当前组渠道再打一次

触发切组时,本次请求仍然返回**当前组**(已失败 RetryTimes 次)的渠道,白白浪费一次调用和预扣费,下一轮才真正用新组。

### 🔴 Bug D — `RetryTimes=0`(包默认值)时跨分组彻底失效

`common/constants.go` 默认 `RetryTimes=0`。此时 `priorityRetry(0) >= RetryTimes(0)` 立即成立,distributor 直接把 `AutoGroupIndex` 预先 +1(off-by-one),而 `shouldRetry` 拿到 `0-0=0 <= 0` 直接返回 false → 只调一次、零回退。**很多运维默认没改重试次数,跨分组就是哑的。**

### 🟡 Bug E — `ContextKeyAutoGroupRetryIndex` 只写不读

该 context key 只在 `channel_select.go` 写、全代码无人读,是旧设计残留的死状态,且注释仍按它来描述逻辑,误导维护者。

### 🟡 Bug F — distributor 亲和性分支不设 `AutoGroupIndex`

亲和性分支选了组 `g` 但没设 `AutoGroupIndex`,亲和失败后 relay 重试会从 index 0 重新开始,计费组也可能跳变。

### 🟡 Bug G — `auth.go` 对 "auto" 误判 403

当 "auto" 不在用户可用分组里时,`auth.go` 直接 403"无权访问 auto 分组",且没有像 GroupRatio 检查那样给 "auto" 特判。

### 🟡 Bug H — `GetUserAutoGroup` 静默丢弃不可见分组,无诊断

用户 tier 不可见的配置分组被静默过滤,无任何日志。模型明明在某组里却"找不到组",无从排查。

### 根因总结

设计意图是**"按配置顺序逐组熔断"**的智能路由,但实现把它**寄生在了通用重试机制上**:切组依赖 `RetryTimes` 计数 + `shouldRetry` 状态码白名单。于是 (1) 不可重试的错误掐断整条熔断链(Bug A);(2) `RetryTimes` 配置不当时整个机制失效或行为诡异(Bug B/D)。叠加随机权重选渠道,表现为"有时这样、有时那样"。

## 4. 解决方案

核心思路:**把"跨分组熔断"从"分组内重试"里彻底解耦**。让 `CacheGetRandomSatisfiedChannel` 退化为纯粹的"给定分组索引 + 组内重试号,返回渠道",把**是否切换分组的决策上移到 `relay.go` 的重试循环**——因为只有那里才知道错误是什么。

### 改动 1 — `service/channel_select.go`(修 B / C / E)

`CacheGetRandomSatisfiedChannel` 的 auto 分支变成纯选择器:

- 从 `ContextKeyAutoGroupIndex`(默认 0)读起始分组索引,从 `param.Retry` 读组内优先级重试号;
- 跳过没有该模型渠道的分组,返回第一个有渠道的分组里的渠道,并写入 `ContextKeyAutoGroup`(计费用)与命中的 `ContextKeyAutoGroupIndex`;
- **不再**根据 `RetryTimes` 内部切组,**不再**写 `AutoGroupRetryIndex`;
- 所有分组都没有该模型时返回 `nil` channel,告知上层"自动分组已用尽"。

**不变量:** `AutoGroupIndex` 永远等于"当前正在使用的组",`param.Retry` 是该组的组内优先级重试号。

新增两个辅助函数:

- `AdvanceToNextAutoGroup(c)`:读当前 `AutoGroupIndex`,+1 写回;还有可尝试分组返回 true,列表用尽返回 false。空组由选择器内部跳过。
- `CrossGroupShouldFallback(c, err)`:跨分组回退闸门(见改动 3)。

### 改动 2 — `controller/relay.go` 重试循环(修 A / D)

把单一 `shouldRetry → break` 改成两级决策:

```go
crossGroupFallback := relayInfo.TokenGroup == "auto" &&
    common.GetContextKeyBool(c, constant.ContextKeyTokenCrossGroupRetry)

for {
    channel, channelErr := getChannel(c, relayInfo, retryParam)
    if channelErr != nil { newAPIError = channelErr; break } // 所有组试完 → 终止
    ... 发请求 ...
    if newAPIError == nil { return }
    processChannelError(...)

    // ① 组内重试:同一组、更高优先级。用 RetryTimes 硬约束(避免 IsChannelError 死循环)
    if retryParam.GetRetry() < common.RetryTimes &&
        shouldRetry(c, newAPIError, common.RetryTimes-retryParam.GetRetry()) {
        retryParam.IncreaseRetry()
        continue
    }
    // ② 组内耗尽 / 不可重试 → 若允许跨分组且该错误应回退,切下一组并重置组内计数
    if crossGroupFallback &&
        service.CrossGroupShouldFallback(c, newAPIError) &&
        service.AdvanceToNextAutoGroup(c) {
        retryParam.SetRetry(0)
        continue
    }
    break
}
```

> **死循环防护:** 去掉了旧循环 `for ; retry<=RetryTimes` 的固定上界后,特地加了 `retryParam.GetRetry() < common.RetryTimes` 硬约束——因为 `shouldRetry` 对 `IsChannelError` 会**无条件**返回 true,否则持续性渠道错误会无限循环。
>
> **修复 Bug D 的副作用:** `RetryTimes=0` 时,①的 `0<0` 为假,组内重试直接跳过,②接管 → 每个分组各试 1 次后顺序往下切。即使运维没配重试次数,跨分组也能正常工作。

### 改动 3 — `CrossGroupShouldFallback`(采用方案②)

跨分组回退比组内 `shouldRetry` **更宽松**:渠道侧错误(`429/5xx/超时/上游 404`)即便组内不重试也会切下一组。但以下几类**不切组**(在每个分组都会同样失败):

```go
func CrossGroupShouldFallback(c, err) bool {
    if err == nil                                    { return false }
    if 设置了 specific_channel_id                     { return false } // 令牌绑定固定渠道
    if ShouldSkipRetryAfterChannelAffinityFailure(c) { return false } // 亲和性要求停止
    if types.IsSkipRetryError(err)                   { return false } // 413/敏感词等预检错误
    if isClientValidationStatusCode(err.StatusCode)  { return false } // 400/422 客户端参数错误
    return true
}
```

`isClientValidationStatusCode` 列表保持窄:`400 / 422 / 413`。**若想对 400 也回退(部分上游用 400 表达"模型不可用"),把它从该函数移除一行即可。**

### 改动 4 — `middleware/distributor.go`(修 F)

亲和性分支选中组 `g` 后补写 `ContextKeyAutoGroupIndex = gi`,使后续重试从命中处继续跨分组回退,而非从 0 重新开始。

### 改动 5 — 诊断与边界(修 G / H)

- **G:** `auth.go` 对 `tokenGroup=="auto"` 跳过"可用分组成员"硬校验(auto 的可用性已由 `GetUserAutoGroup` 逐组把关),避免误报 403。
- **H:** `GetUserAutoGroup` 在 DEBUG 模式下,当某配置分组因用户不可见被过滤时打 `SysLog`,便于排查"模型在组里却没命中"。

## 5. 行为对照

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| 组A渠道返回 500/429 | 切组(若 RetryTimes 够大) | 组内重试到上限后切组 ✅ |
| 组A渠道返回 **400/504/SkipRetry** | **直接报错返回**(Bug A) | 客户端错误(400/422/SkipRetry)不切;其余切 ✅ |
| `RetryTimes=0` | **完全不回退**(Bug D) | 每组各试 1 次顺序回退 ✅ |
| 组A只有 1 个渠道 | 被硬打 `RetryTimes+1` 次(Bug B) | 组内重试号超出优先级即自然停 → 切组 |
| 切组瞬间 | 坏组渠道再被打 1 次(Bug C) | 直接用新组 ✅ |
| 组A无此模型、组B有 | 正常跳过 | 正常跳过(保留) |
| 用户不可见的配置分组 | 静默跳过 | 跳过 + DEBUG 日志 ✅ |

## 6. 关于"用户不可见分组应被跳过"

这点现状就正确并被完整保留。`service/group.go` 的 `GetUserAutoGroup` 在返回候选分组前,已经把配置的 auto 列表与 `GetUserUsableGroups(userGroup)`(该用户 tier 可见的分组)求交集:

```go
for _, group := range setting.GetAutoGroups() {
    if _, ok := groups[group]; ok {     // 用户不可见 → 直接不进候选列表
        autoGroups = append(autoGroups, group)
    }
}
```

所有选渠道 / 切组路径(`CacheGetRandomSatisfiedChannel`、distributor 亲和性分支、`AdvanceToNextAutoGroup`)都只遍历 `GetUserAutoGroup` 的结果,因此用户分组下不可见的渠道分组从一开始就不在切换序列里,自然被跳过。

## 7. 影响面与风险

- 改动集中在 `channel_select.go` + `relay.go` 两个文件的控制流,外加 `distributor.go`/`auth.go`/`group.go` 少量行;不动 DB、不动 DTO、不动计费金额计算(计费仍读 `ContextKeyAutoGroup`,逻辑不变)。
- 非 auto 令牌行为**逐位保持不变**(已逐 case 核对 `RetryTimes+1` 次尝试语义)。
- 行为变化点:跨分组现在**可能产生更多上游调用**(遇到本来会立刻返回的渠道侧错误时)。每次失败尝试都有预扣费 + 退款,但不会重复扣实际费用。

## 8. 验证

- `go build ./...` ✅ `go vet ./service/... ./controller/... ./middleware/...` ✅
- 新增 `service/channel_select_test.go`:覆盖 `CrossGroupShouldFallback` 全分支、`isClientValidationStatusCode`、`AdvanceToNextAutoGroup`(含过滤后到达列表末尾)、`GetUserAutoGroup` 过滤——全部通过 ✅
- 完整 `go test ./service/` 无回归 ✅

> 端到端联调需带渠道/auto 配置的运行环境;单测覆盖的是新增的决策逻辑。
