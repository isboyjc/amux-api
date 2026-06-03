# Auto 分组跨分组回退机制修复

## 问题描述

用户创建令牌选择 **auto 分组**并开启**跨分组尝试**后，使用此令牌调用模型时：

- **预期行为**：按照配置的分组顺序依次查找，当前分组调用失败后自动切换到下一个分组，直到所有分组都尝试完毕才返回错误
- **实际行为**：有时候遇到分组模型调用错误就直接返回了，没有继续尝试其他分组

## 问题分析

### 核心 Bug：RetryTimes=0 导致跨分组回退失效

#### 关键代码流程

```
请求 → Distribute() 中间件选择渠道 → Relay() 重试循环处理调用
```

#### 根本原因

**1. Distribute() 和 Relay() 使用不同的 RetryParam 实例**

```go
// Distribute() 中创建本地 RetryParam
channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
    Ctx:        c,
    ModelName:  modelRequest.Model,
    TokenGroup: usingGroup,
    Retry:      common.GetPointer(0),
})

// Relay() 中创建新的 RetryParam
retryParam := &service.RetryParam{
    Ctx:        c,
    TokenGroup: relayInfo.TokenGroup,
    ModelName:  relayInfo.OriginModelName,
    Retry:      common.GetPointer(0),
}
```

`Distribute()` 中的 `CacheGetRandomSatisfiedChannel` 会通过 `ResetRetryNextTry()` 准备分组切换，但这个状态存储在 `RetryParam` 实例中。`Relay()` 创建了新的 `RetryParam`，导致准备好的切换状态丢失。

**2. CacheGetRandomSatisfiedChannel 的分组切换条件**

```go
// service/channel_select.go
if crossGroupRetry && priorityRetry >= common.RetryTimes {
    // 准备切换到下一个分组
    common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
    param.SetRetry(0)
    param.ResetRetryNextTry()  // 状态存储在 RetryParam 中！
}
```

当 `RetryTimes=0`（默认值）时，条件 `priorityRetry >= 0` 始终为 true，系统会准备切换。但切换状态丢失了。

**3. Relay() 循环中的 shouldRetry 检查**

```go
// controller/relay.go
for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
    // ... 调用逻辑 ...

    if !shouldRetry(c, newAPIError, common.RetryTimes-retryParam.GetRetry()) {
        break  // 直接退出！
    }
}

// shouldRetry 函数
func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
    // ...
    if retryTimes <= 0 {
        return false  // RetryTimes=0 时，retryTimes=0-0=0，返回 false
    }
    // ...
}
```

当 `RetryTimes=0` 时，`shouldRetry` 收到的 `retryTimes = 0 - 0 = 0`，直接返回 false，循环退出。

#### 问题流程图解

```
配置: RetryTimes=0, auto groups=[A, B], model only in group B, crossGroupRetry=true

Distribute():
  → CacheGetRandomSatisfiedChannel(Retry=0)
  → group A 没有渠道 → AutoGroupIndex=1, 继续
  → group B 有渠道 → 准备切换(AutoGroupIndex=2, ResetRetryNextTry)
  → 返回 channel_B ✓

Relay() Retry=0:
  → getChannel() → 返回 context 中的 channel_B
  → 调用失败 → ERROR
  → shouldRetry(0-0=0) → false → break ✗
  → 循环结束，没有机会尝试其他分组！
```

### 次要问题：错误信息不明确

当所有 auto 分组都用尽时，错误信息只显示 `auto`，没有告诉用户具体尝试了哪些分组：

```
分组 auto 下模型 gpt-4 的可用渠道不存在
```

## 解决方案

### 1. 将 pending switch 状态存储在 gin.Context 中

**问题**：`ResetRetryNextTry` 状态存储在 `RetryParam` 实例中，无法跨 `Distribute()` 和 `Relay()` 传递。

**方案**：新增 context key `ContextKeyAutoGroupPendingSwitch`，在准备分组切换时同时在 context 中设置标志。

```go
// constant/context_key.go
ContextKeyAutoGroupPendingSwitch ContextKey = "auto_group_pending_switch"
```

```go
// service/channel_select.go - CacheGetRandomSatisfiedChannel
if crossGroupRetry && priorityRetry >= common.RetryTimes {
    common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
    // 存储到 context 中，跨 Distribute() 和 Relay() 传递
    common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupPendingSwitch, true)
    param.SetRetry(0)
    param.ResetRetryNextTry()
}
```

### 2. 在 Relay 循环中检查是否有更多分组可尝试

**方案**：新增 `hasMoreAutoGroupsToTry()` 辅助函数，在 `shouldRetry()` 返回 false 后检查是否有更多分组。

```go
// controller/relay.go
func hasMoreAutoGroupsToTry(c *gin.Context, info *relaycommon.RelayInfo) bool {
    if info == nil || info.TokenGroup != "auto" {
        return false
    }
    // 检查 context 中的 pending switch 标志
    pendingSwitch := common.GetContextKeyBool(c, constant.ContextKeyAutoGroupPendingSwitch)
    if !pendingSwitch {
        return false
    }
    // 清除标志防止无限循环
    common.SetContextKey(c, constant.ContextKeyAutoGroupPendingSwitch, false)
    // 检查是否还有更多分组
    autoGroupIndex, exists := common.GetContextKey(c, constant.ContextKeyAutoGroupIndex)
    if !exists {
        return false
    }
    idx, ok := autoGroupIndex.(int)
    if !ok {
        return false
    }
    userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
    autoGroups := service.GetUserAutoGroup(userGroup)
    return idx < len(autoGroups)
}
```

**在 Relay() 和 RelayTask() 中使用**：

```go
if !shouldRetry(c, newAPIError, common.RetryTimes-retryParam.GetRetry()) {
    // 检查是否有更多 auto 分组可尝试
    if hasMoreAutoGroupsToTry(c, relayInfo) {
        logger.LogInfo(c, "Cross-group fallback: trying next auto group")
        continue  // 继续循环，尝试下一个分组
    }
    break
}
```

### 3. 改进错误信息

当所有 auto 分组都用尽时，显示更详细的信息：

```go
// middleware/distributor.go
if channel == nil {
    showGroup := usingGroup
    if usingGroup == "auto" {
        userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
        autoGroups := service.GetUserAutoGroup(userGroup)
        showGroup = fmt.Sprintf("auto[%s]", strings.Join(autoGroups, ","))
    }
    abortWithOpenAiMessage(c, http.StatusServiceUnavailable,
        i18n.T(c, i18n.MsgDistributorNoAvailableChannel,
            map[string]any{"Group": showGroup, "Model": modelRequest.Model}),
        types.ErrorCodeModelNotFound)
}
```

错误信息示例：`分组 auto[default,premium] 下模型 gpt-4 的可用渠道不存在`

## 修复后的流程

### 场景 1：所有分组都没有可用渠道

```
RetryTimes=0, auto groups=[A, B], model 不存在于任何分组

Distribute():
  → group A 没有 → group B 没有
  → 返回 nil → 错误: "分组 auto[A,B] 下模型 xxx 的可用渠道不存在"
```

### 场景 2：只有最后一个分组有渠道

```
RetryTimes=0, auto groups=[A, B, C], model only in group C

Distribute():
  → group A 没有 → group B 没有 → group C 有
  → 准备切换(AutoGroupIndex=3), pendingSwitch=true
  → 返回 channel_C

Relay() Retry=0:
  → channel_C 调用成功 ✓
```

### 场景 3：第一个分组失败，切换到第二个分组

```
RetryTimes=0, auto groups=[A, B, C], model in group A (fails) and C

Distribute():
  → group A 有渠道
  → 准备切换(AutoGroupIndex=1), pendingSwitch=true
  → 返回 channel_A

Relay() Retry=0:
  → channel_A 调用失败
  → shouldRetry(0) → false
  → hasMoreAutoGroupsToTry(): pendingSwitch=true, idx=1, len=3
  → 1 < 3 = true → continue! ✓

Relay() Retry=0 (重置):
  → CacheGetRandomSatisfiedChannel(Retry=0), startGroupIndex=1
  → group B 没有 → group C 有 → 返回 channel_C
  → 调用成功 ✓
```

### 场景 4：所有分组的渠道都失败

```
RetryTimes=0, auto groups=[A, B], model in both, both fail

Distribute():
  → group A 有渠道
  → 准备切换(AutoGroupIndex=1), pendingSwitch=true
  → 返回 channel_A

Relay() Retry=0:
  → channel_A 失败
  → hasMoreAutoGroupsToTry(): true → continue

Relay() Retry=0:
  → group B 有渠道 → channel_B
  → 准备切换(AutoGroupIndex=2), pendingSwitch=true
  → channel_B 失败
  → hasMoreAutoGroupsToTry(): idx=2, len=2, 2<2=false → break
  → 返回错误 ✓ (所有分组已尝试)
```

## 修改文件清单

| 文件 | 修改内容 |
|------|----------|
| `constant/context_key.go` | 新增 `ContextKeyAutoGroupPendingSwitch` |
| `service/channel_select.go` | 新增 `HasPendingReset()` 方法；在准备分组切换时设置 context 标志 |
| `controller/relay.go` | 新增 `hasMoreAutoGroupsToTry()` 函数；在 Relay() 和 RelayTask() 中使用 |
| `middleware/distributor.go` | 改进 auto 分组用尽时的错误信息 |

## 注意事项

1. **此修复向后兼容**：不影响现有的非 auto 分组逻辑，也不影响 `RetryTimes > 0` 的正常重试机制
2. **防止无限循环**：`hasMoreAutoGroupsToTry()` 在检查后会清除 context 中的标志，确保每个 pending switch 只被检查一次
3. **需要 RetryTimes > 0 才能发挥完整功能**：虽然修复了 `RetryTimes=0` 时的分组切换，但如果想在同一分组内尝试不同优先级的渠道，仍需要设置 `RetryTimes > 0`
