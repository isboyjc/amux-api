# amux-api 事件总线与 Outbox 设计方案

> 状态：待确认 → 待实施
> 负责：sejian@amux.chat
> 适用范围：用户生命周期 / 计费等业务事件的统一发布与异步分发

---

## 1. 背景与目标

### 1.1 直接动机
接入 Resend 做邮件营销，需要在用户注册、注销、充值、分组变化等关键节点同步联系人。为避免在业务代码各处直接调用 Resend API（污染业务、外部 HTTP 拖慢用户请求、Resend 故障影响主流程），建立统一的事件总线 + Outbox 异步分发体系。

### 1.2 长期目标
- 成为项目通用的事件平台，未来可承载审计日志、数据分析、风控、第三方集成等多种订阅场景
- 业务代码只发事件，不关心下游谁在听
- 任何下游故障不影响业务主流程
- 事件可观测、可重放、可审计

### 1.3 非目标
- 不做高 QPS 事件（如 API 调用、额度消耗）的实时流处理 —— 该类事件量级不适合 Outbox，未来如有需求另立聚合方案
- 不引入消息队列中间件（NATS / Kafka 等）—— 当前规模无必要

---

## 2. 核心设计决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 投递模型 | **DB Outbox + 后台 Worker 轮询** | 简单、零中间件依赖；事件与业务变更同事务，原子性强 |
| 表结构 | **两表分离**：`event_log`（事实）+ `event_dispatch`（投递状态） | 事实流不可变、可审计、可补发；状态可清理可重试，互不干扰 |
| 订阅者发现 | **代码侧静态注册**（init 时 Register） | Go 无动态加载需求；新增订阅者 = 新增文件 + 一行注册 |
| 扇出时机 | **发布时扇出**（Publish 时按 registry 写入 dispatch 行） | 状态从 t=0 可观测；后期加订阅者用一条 SQL 补发即可 |
| 事件命名 | **`<domain>.<action>` 2 级为主，必要时 3 级**；订阅支持前缀通配 | 简单、可演进；订阅者可写 `user.*` 或 `*` |
| 事务语义 | 提供 `Publish(tx, ...)` 与 `PublishNoTx(...)` 两套 API | 事务内调用保证事件与业务原子；无事务场景操作完成后立即发布 |
| Worker 多实例 | **乐观 claim**（`UPDATE WHERE status='pending'` 检查 RowsAffected） | 三库兼容；不依赖 `FOR UPDATE SKIP LOCKED` |
| 重试策略 | 指数退避 30s / 2m / 10m / 1h / 6h，6 次后 `dead` | 覆盖短暂抖动到 1 天内的故障窗口 |
| 数据保留 | dispatch 的 `done` 行保留 30 天；event_log 默认 365 天；dispatch 的 `dead` 永久保留 | 平衡审计需求与存储增长 |
| 失败告警 | Phase 1 只记日志，不发告警 | 先观察 dead 量级；Phase 3 接入 user_notify 或独立通道 |
| Phase 1 订阅者 | 仅内置 `logger` 订阅者（订阅 `*`，打日志） | 做端到端验证 + 未来订阅者参考实现；Resend 等后续单独接入 |

---

## 3. 事件命名约定

### 3.1 规则
- 格式：`<domain>.<action>` 或 `<domain>.<subdomain>.<action>`
- 字符集：小写字母 + 数字 + `.` + `_`（下划线用于多词动作，如 `top_up`）
- **核心原则**：**事件名描述「事实」，不描述「原因」**
  - ✅ `user.group.changed`（事实：分组变了），不管触发源是 topup / subscription / admin
  - ❌ `user.topup_caused_group_change`（混入了原因）

### 3.2 所有事件类型常量必须集中定义
位置：`service/events/types.go`，禁止业务代码使用裸字符串。

### 3.3 订阅者匹配支持通配
```go
// 精确匹配
"user.registered"

// 前缀通配（仅支持末尾 .*）
"user.*"           // 匹配所有 user 域事件
"billing.topup.*"  // 匹配所有 topup 子域事件

// 全订阅
"*"                // 匹配所有事件（logger / 审计场景）
```

匹配实现（约 10 行）：
```go
func topicMatches(eventType, pattern string) bool {
    if pattern == "*" || pattern == eventType {
        return true
    }
    if strings.HasSuffix(pattern, ".*") {
        return strings.HasPrefix(eventType, strings.TrimSuffix(pattern, "*"))
    }
    return false
}
```

---

## 4. Phase 1 事件清单（7 个）

### 4.1 `user.registered`
- **埋点**：`controller/user.go:188`（邮箱注册）、`controller/oauth.go:298`（OAuth 注册成功 + Finalize 之后）
- **事务**：无外层事务，使用 `PublishNoTx`
- **Payload**：
  ```go
  type UserRegisteredPayload struct {
      UserId         int    `json:"user_id"`
      Email          string `json:"email"`
      Username       string `json:"username"`
      DisplayName    string `json:"display_name"`
      Group          string `json:"group"`
      RegisterSource string `json:"register_source"` // email|github|discord|oidc|linuxdo|telegram|wechat
      InviterId      int    `json:"inviter_id,omitempty"`
      CreatedAt      int64  `json:"created_at"`
  }
  ```

### 4.2 `user.deleted`
- **埋点**：`controller/user.go:793`（admin 硬删）、`controller/user.go:812`（self 软删）
- **事务**：无
- **Payload**：
  ```go
  type UserDeletedPayload struct {
      UserId     int    `json:"user_id"`
      Email      string `json:"email"`
      Username   string `json:"username"`
      DeleteType string `json:"delete_type"` // admin_hard|self_soft
      DeletedAt  int64  `json:"deleted_at"`
  }
  ```

### 4.3 `user.profile.updated`
- **埋点**：`controller/user.go:745`（UpdateSelf）
- **触发条件**：**diff 后只在 `username` 或 `display_name` 实际变化时才发**（密码、setting、sidebar 等不触发）
- **事务**：无
- **Payload**：
  ```go
  type UserProfileUpdatedPayload struct {
      UserId        int      `json:"user_id"`
      Email         string   `json:"email"`
      Username      string   `json:"username"`
      DisplayName   string   `json:"display_name"`
      ChangedFields []string `json:"changed_fields"` // ["username","display_name"]
      UpdatedAt     int64    `json:"updated_at"`
  }
  ```

### 4.4 `user.email.bound`
- **埋点**：`controller/user.go:1018-1042`（EmailBind） + OAuth 首次绑定路径
- **事务**：无
- **Payload**：
  ```go
  type UserEmailBoundPayload struct {
      UserId   int    `json:"user_id"`
      OldEmail string `json:"old_email,omitempty"` // 首次绑定为空
      NewEmail string `json:"new_email"`
      BoundAt  int64  `json:"bound_at"`
  }
  ```

### 4.5 `user.group.changed`
- **埋点**：`model/topup.go:643`（`CheckAndUpgradeUserGroup` 成功 update 之后，统一收口）
- **事务**：复用上层 caller 的事务上下文（若有）
- **Payload**：
  ```go
  type UserGroupChangedPayload struct {
      UserId    int    `json:"user_id"`
      Email     string `json:"email"`
      FromGroup string `json:"from_group"`
      ToGroup   string `json:"to_group"`
      Trigger   string `json:"trigger"` // topup|subscription|admin|manual
      ChangedAt int64  `json:"changed_at"`
  }
  ```

### 4.6 `billing.topup.succeeded`
- **埋点**：
  - `model/topup.go:148`（Stripe，**事务内** `Publish(tx, ...)`）
  - `model/topup.go:431`（EPay / 其他在线支付）
- **事务**：Stripe 必须事务内；其他根据原代码事务范围决定
- **Payload**：
  ```go
  type BillingTopupSucceededPayload struct {
      UserId           int    `json:"user_id"`
      Email            string `json:"email"`
      TopupId          int    `json:"topup_id"`
      AmountQuota      int    `json:"amount_quota"`       // 到账额度
      AmountMoneyCents int64  `json:"amount_money_cents"` // 支付金额（分）
      Currency         string `json:"currency"`           // CNY/USD/...
      PaymentMethod    string `json:"payment_method"`     // stripe|epay|waffo|creem
      TradeNo          string `json:"trade_no"`
      CompletedAt      int64  `json:"completed_at"`
  }
  ```

### 4.7 `billing.redemption.used`
- **埋点**：`model/topup.go:518`（兑换码使用成功后）
- **事务**：跟随原代码事务范围
- **Payload**：
  ```go
  type BillingRedemptionUsedPayload struct {
      UserId       int    `json:"user_id"`
      Email        string `json:"email"`
      RedemptionId int    `json:"redemption_id"`
      AmountQuota  int    `json:"amount_quota"`
      UsedAt       int64  `json:"used_at"`
  }
  ```

### 4.8 不在 Phase 1 范围

| 事件 | 不收原因 |
|---|---|
| `billing.subscription.activated` | 订阅主要结果已被 `user.group.changed` 覆盖；面向订阅的专用事件等真需求出现再加 |
| `user.logged_in` | 量大、用途窄；有审计/异常登录需求再加 |
| `token.created` / `token.deleted` | 用户能看 token 列表即可，admin 操作走 audit log 更合适 |
| `channel.added` / `channel.disabled` | 同上，admin audit log 范畴 |
| API 调用 / 额度消耗 | QPS 过高，Outbox 不适合；如需另立聚合方案 |
| 签到 / 邀请奖励 | 优先级低 |

---

## 5. 数据库 Schema

> **实施备注**：两表实际定义在 `service/events/dao.go`（而非 `model/` 包）。
> 原因：`model/topup.go`、`model/redemption.go` 等业务代码需要调用 `events.Publish*(...)`，
> 而事件包内部又需要操作这两张表，把表 DAO 留在 model 会形成
> `model → events → model` 的导入循环。把表与 DAO 都放在 events 包内、由
> `events.SetDB(model.DB)` 注入数据库引用，是更干净的解法。
> 表结构、字段、索引完全等同于下文设计，AutoMigrate 由 `events.AutoMigrate()` 完成。
>
> **物理表名注意**：Go 类型 `EventLog` / `EventDispatch`，但 GORM 自动复数化，
> 实际数据库表名是 **`event_logs`** 和 **`event_dispatches`**（带 s）。运维查询时记得加 s。

### 5.1 `event_log`（不可变事实流）

```go
// service/events/dao.go
type EventLog struct {
    Id          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
    EventType   string `gorm:"type:varchar(64);not null;index:idx_event_log_type_time,priority:1" json:"event_type"`
    AggregateId int    `gorm:"index" json:"aggregate_id"` // 一般为 user_id，便于排查
    Payload     string `gorm:"type:text;not null" json:"payload"` // common.Marshal 序列化
    PublishedAt int64  `gorm:"not null;index:idx_event_log_type_time,priority:2;index" json:"published_at"`
}
```

**说明**：
- `Payload` 用 `text` 不用 `JSONB`（Rule 2 三库兼容）
- 复合索引 `(event_type, published_at)` 支撑"按类型查时间窗口"的 admin UI
- 单列索引 `published_at` 支撑 TTL 清理
- 此表**只追加，不更新**

### 5.2 `event_dispatch`（投递状态）

```go
// service/events/dao.go
type EventDispatch struct {
    Id          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
    EventId     int64  `gorm:"not null;index:idx_dispatch_event" json:"event_id"` // → event_log.id
    Subscriber  string `gorm:"type:varchar(64);not null;index:idx_dispatch_poll,priority:2" json:"subscriber"`
    Status      string `gorm:"type:varchar(16);not null;default:'pending';index:idx_dispatch_poll,priority:1" json:"status"` // pending|processing|done|dead
    RetryCount  int    `gorm:"not null;default:0" json:"retry_count"`
    NextRetryAt int64  `gorm:"not null;default:0;index:idx_dispatch_poll,priority:3" json:"next_retry_at"`
    LastError   string `gorm:"type:text" json:"last_error"`
    WorkerId    string `gorm:"type:varchar(64)" json:"worker_id"` // 持有 claim 的 worker 实例 id
    CreatedAt   int64  `gorm:"not null;index" json:"created_at"`
    UpdatedAt   int64  `gorm:"not null" json:"updated_at"`
    ProcessedAt int64  `json:"processed_at"`
}
```

**说明**：
- 不使用 FK 约束（三库行为不一致，应用层保证完整性）
- 复合索引 `(status, subscriber, next_retry_at)` 支撑 worker 拉取
- 单列索引 `event_id` 支撑 JOIN event_log 拿 payload
- 单列索引 `created_at` 支撑 TTL 清理

### 5.3 AutoMigrate

事件子系统自管理迁移。`main.go` 在 `model.InitDB()` 完成之后调用：
```go
events.SetDB(model.DB)
if err := events.AutoMigrate(); err != nil {
    common.FatalLog("failed to migrate event tables: " + err.Error())
}
```
`events.AutoMigrate()` 内部对 `&EventLog{}`、`&EventDispatch{}` 做 GORM AutoMigrate，
三库兼容。`model/main.go` 的 AutoMigrate 列表不需要变更。

---

## 6. Publish API

### 6.1 接口签名

```go
// service/events/bus.go

// Publish 在外层事务内强一致发布；publish 失败会阻塞主业务（让 caller 回滚整个 tx）。
// 几乎不用到 —— 当前所有事件都是营销/分析类，丢一两条可接受。预留给将来"事件丢了
// 不行"的场景。
func Publish(tx *gorm.DB, eventType string, aggregateId int, payload any) error

// PublishBestEffortInTx 在外层事务里发布，但不阻塞主业务。**Phase 1 默认 API。**
// 内部用 GORM 嵌套 Transaction（底层 SAVEPOINT，三库原生支持）把 publish 隔离起来。
// publish 失败只回滚到 savepoint，外层事务可以继续 commit。失败仅记日志，无返回值。
// 用于：所有现有埋点（充值、订阅、兑换、组变化等）。
func PublishBestEffortInTx(tx *gorm.DB, eventType string, aggregateId int, payload any)

// PublishNoTx 直接发布事件，内部起新事务；用于无外层事务的场景（注册、删除等 controller 层）。
func PublishNoTx(eventType string, aggregateId int, payload any) error
```

### 6.2 选哪个？

| 场景 | 用什么 | 原因 |
|---|---|---|
| controller 层调用，无外层事务 | `PublishNoTx` | 自己起新事务即可；返回错误仅日志，不影响 HTTP 响应 |
| model 层在外层事务内，营销/分析事件 | `PublishBestEffortInTx` | savepoint 隔离，publish 失败不让用户付款失败 |
| 需要"事件丢失则业务回滚"的强一致场景 | `Publish` | 当前**无人使用**，预留 |

**为什么 Phase 1 默认用 best-effort？**
"用户无感知"是底线——付款流程不能因为营销事件子系统的 DB 抖动而失败。事件丢失虽然不希望，但通过 worker 重试 + 后续事件触发再次 sync，最终态依然能收敛。

### 6.2 行为

1. `common.Marshal(payload)` 序列化为 JSON 字符串
2. INSERT 1 行到 `event_log`
3. 查 registry 找匹配该 topic 的所有订阅者
4. 对每个匹配订阅者 INSERT 1 行到 `event_dispatch`（`status='pending'`, `next_retry_at=0`）
5. 全部 INSERT 在同一事务内

### 6.3 失败处理

- payload 序列化失败 / DB INSERT 失败 → 返回 error，由 caller 决定是否回滚业务
- 调用方一般可选择**忽略错误**（事件丢失不阻断业务），但 Stripe 等强一致场景必须检查 error 并回滚事务

### 6.4 命令行示例

```go
// 无事务（OAuth 注册完成后）
events.PublishNoTx(events.UserRegistered, user.Id, &events.UserRegisteredPayload{
    UserId:         user.Id,
    Email:          user.Email,
    Username:       user.Username,
    DisplayName:    user.DisplayName,
    Group:          user.Group,
    RegisterSource: "github",
    CreatedAt:      time.Now().Unix(),
})

// 事务内（Stripe 充值成功）
err := model.DB.Transaction(func(tx *gorm.DB) error {
    // ... 既有业务 update ...
    return events.Publish(tx, events.BillingTopupSucceeded, topUp.UserId,
        &events.BillingTopupSucceededPayload{...})
})
```

---

## 7. 订阅者机制

### 7.1 接口定义

```go
// service/events/subscriber.go

type Event struct {
    Id          int64
    Type        string
    AggregateId int
    Payload     []byte   // 原始 JSON，订阅者按需 Unmarshal 到自己的 struct
    PublishedAt int64
}

type Subscriber interface {
    Name() string                                  // 唯一标识，如 "logger" / "resend"
    Topics() []string                              // 订阅的事件类型（支持通配）
    Handle(ctx context.Context, e Event) error    // 返回 nil = done；返回 err = 重试；返回 ErrPermanent = dead
}

// 哨兵错误：订阅者识别为永久失败时返回
var ErrPermanent = errors.New("event handler permanent failure")
```

### 7.2 注册

```go
// service/events/subscriber.go
var registry = map[string]Subscriber{}

func Register(s Subscriber) { registry[s.Name()] = s }

func subscribersFor(eventType string) []Subscriber {
    var matched []Subscriber
    for _, s := range registry {
        for _, t := range s.Topics() {
            if topicMatches(eventType, t) {
                matched = append(matched, s)
                break
            }
        }
    }
    return matched
}
```

### 7.3 Phase 1 内置订阅者：logger

```go
// service/events/subscribers/logger/subscriber.go
type Subscriber struct{}

func (Subscriber) Name() string              { return "logger" }
func (Subscriber) Topics() []string          { return []string{"*"} }
func (Subscriber) Handle(ctx context.Context, e events.Event) error {
    common.SysLog(fmt.Sprintf("[event] %s aggregate=%d payload=%s",
        e.Type, e.AggregateId, string(e.Payload)))
    return nil
}

// init.go
func init() { events.Register(Subscriber{}) }
```

### 7.4 新增订阅者标准流程
1. 在 `service/events/subscribers/<name>/` 下新建 `subscriber.go`
2. 实现 `Subscriber` interface
3. 在 `init()` 中 `events.Register(...)`
4. 在 `main.go` 顶部 `_ "github.com/QuantumNous/new-api/service/events/subscribers/<name>"` 触发 init
5. 重启服务

---

## 8. Worker 设计

### 8.1 启动

```go
// main.go
go events.StartWorker(ctx, events.WorkerOpts{
    PollInterval: 2 * time.Second,
    BatchSize:    50,
    Concurrency:  4,                   // 同一实例内并发 handler 数
    WorkerId:     common.GenerateUUID(), // 进程启动时生成
})
```

所有实例都启动 worker，靠 DB 乐观 claim 互斥。

### 8.2 主循环

每轮（默认 2s）：

```
1. 启动时 reclaim 一次（仅首次）：
   UPDATE event_dispatch SET status='pending'
   WHERE status='processing' AND worker_id=? AND updated_at < (now - 5min)
   // 自身上次崩溃残留

2. 拉一批 pending：
   SELECT id FROM event_dispatch
   WHERE status='pending' AND next_retry_at <= ?
   ORDER BY id LIMIT 50

3. 对每行 claim（乐观锁）：
   UPDATE event_dispatch
   SET status='processing', worker_id=?, updated_at=?
   WHERE id=? AND status='pending'
   // RowsAffected==1 才处理，否则被其他实例抢走

4. claim 成功后：
   JOIN event_log 拿 payload
   调用 subscriber.Handle(ctx, event)
   - nil → UPDATE status='done', processed_at=...
   - ErrPermanent → UPDATE status='dead', last_error=...
   - 其他 error → 计算 next_retry_at，UPDATE status='pending', retry_count+1, last_error=...
   - retry_count 已达上限 → UPDATE status='dead'
```

### 8.3 重试退避

```go
var retrySchedule = []time.Duration{
    30 * time.Second,
    2 * time.Minute,
    10 * time.Minute,
    1 * time.Hour,
    6 * time.Hour,
}
// 第 N 次失败后 next_retry_at = now + retrySchedule[min(N, len-1)]
// 第 6 次失败 → dead
```

### 8.4 处理超时
- 单次 `Handle` 调用上下文超时：30s（worker 层 `context.WithTimeout`）
- 订阅者自己长 HTTP 调用应自带更短的客户端超时

### 8.5 优雅停机
- main.go cancel 顶层 context → worker 停止拉取新任务
- 等待已 claim 的任务完成或上下文超时
- 未完成的 `processing` 行下次启动时被 reclaim

---

## 9. 数据保留策略

### 9.1 规则

| 表 / 状态 | 保留 | 理由 |
|---|---|---|
| `event_log` 全部 | 默认 365 天（可配置 0 = 永不删） | 审计窗口；admin UI 查询的主数据 |
| `event_dispatch` `done` | 30 天 | 排查窗口；本身只是状态记录 |
| `event_dispatch` `dead` | **永久** | 量极小，需人工干预，删了无法追溯 |
| `event_dispatch` `pending` / `processing` | 不删 | 正在工作 |

### 9.2 实现

新文件 `service/event_cleanup_task.go`，模仿 `service/subscription_reset_task.go` 的模式，每天凌晨 3:00 UTC 运行一次：

```go
func cleanupOnce(ctx context.Context) {
    batchSize := operation_setting.EventCleanupBatchSize

    // 1. 清 done dispatch（30 天前）
    deleteInBatches(ctx,
        "DELETE FROM event_dispatch WHERE status='done' AND processed_at < ? LIMIT ?",
        time.Now().Unix()-30*86400, batchSize)

    // 2. 清旧 event_log（前提：无活跃 dispatch 引用）
    if days := operation_setting.EventLogRetentionDays; days > 0 {
        deleteInBatches(ctx, `
            DELETE FROM event_log
            WHERE published_at < ?
              AND NOT EXISTS (
                  SELECT 1 FROM event_dispatch
                  WHERE event_id = event_log.id
                    AND status IN ('pending','processing','dead')
              )
            LIMIT ?`,
            time.Now().Unix()-int64(days)*86400, batchSize)
    }
}
```

**关键点**：
- **分批 DELETE + 批间 sleep 100ms** 避免长锁
- **三库兼容**：SQLite 默认不支持 `DELETE ... LIMIT`，需用 `DELETE WHERE id IN (SELECT id ... LIMIT)` 包装；GORM 内 raw exec 时按 `common.UsingSQLite` 分支
- **NOT EXISTS 子查询** 防止删了 event_log 但 dispatch 还在引用，造成孤儿
- **永远不动 `dead` 行**

### 9.3 稳态预估
按 30 万事件/年估算：
- `event_log`：~30 万行，~90 MB
- `event_dispatch`：~3 万 active 行 + 累积 dead（量极小）
- **总占用稳定在 ~150 MB 上下浮动**

---

## 10. 配置项

### 10.1 新增文件 `setting/operation_setting/event_system.go`

```go
package operation_setting

var (
    // 保留策略
    EventLogRetentionDays          = 365  // 0 = 永不删
    EventDispatchDoneRetentionDays = 30
    EventCleanupBatchSize          = 1000
    EventCleanupHourUTC            = 3    // 每天该小时跑清理

    // Worker
    EventWorkerEnabled     = true
    EventWorkerPollIntervalMs = 2000
    EventWorkerBatchSize   = 50
    EventWorkerConcurrency = 4
    EventHandleTimeoutMs   = 30000
)

// 标准 getter/setter 接 Option 表（参考 setting/auto_group.go 模式）
```

### 10.2 持久化
通过 `Option` 表标准 setter 注册，admin 后台 → 系统设置页可调整。Phase 1 暂不做 UI，靠默认值即可。

---

## 11. 埋点位置一览

**实际落地情况见第 19 节"Phase 1 实施总结 → 埋点实际位置"表**。
原计划 10 处，Phase 1.5 / Phase 2 / 后续 review 补漏后共 **20 处**，分布在：
- `controller/user.go`（7 处：Register, CreateUser, DeleteUser, DeleteSelf, ManageUser-delete, UpdateSelf, UpdateUser, EmailBind 加 admin 改组路径）
- `controller/oauth.go`（1 处：findOrCreateOAuthUser）
- `controller/wechat.go`（1 处：WeChatAuth 首次注册）
- `controller/topup.go`（1 处：EpayNotify 同步回调）
- `model/topup.go`（5 处：Recharge / ManualCompleteTopUp / RechargeCreem / RechargeWaffo / CheckAndUpgradeUserGroup）
- `model/subscription.go`（3 处：升级 / 取消降级 / 到期回退）
- `model/redemption.go`（1 处：Redeem）

> 每处一行 `events.Publish*` 调用；in-tx 路径都用 `PublishBestEffortInTx`（SAVEPOINT 隔离）保证业务事务不被 publish 失败拖累。

---

## 12. 启动接线

### 12.1 `main.go`

```go
import (
    _ "github.com/QuantumNous/new-api/service/events/subscribers/logger" // 触发 init 注册
)

func main() {
    // ... 现有初始化 ...

    // 启动事件 worker（所有实例都跑）
    if operation_setting.EventWorkerEnabled {
        go events.StartWorker(ctx, events.WorkerOpts{
            PollInterval: time.Duration(operation_setting.EventWorkerPollIntervalMs) * time.Millisecond,
            BatchSize:    operation_setting.EventWorkerBatchSize,
            Concurrency:  operation_setting.EventWorkerConcurrency,
            WorkerId:     common.GenerateUUID(),
        })
    }

    // 启动清理 cron
    go service.StartEventCleanupTask(ctx)

    // ... 现有 router / server ...
}
```

### 12.2 `model/main.go`
`AutoMigrate` 列表追加 `&EventLog{}`、`&EventDispatch{}`。

---

## 13. 包结构（实际实施）

```
service/
  events/
    dao.go                  # EventLog / EventDispatch struct + SetDB + AutoMigrate + 所有 DAO 函数（包级私有）
    types.go                # 事件类型常量 + 各事件的 Payload struct + Event 结构体
    bus.go                  # Publish / PublishNoTx 实现
    subscriber.go           # Subscriber interface + registry + topicMatches + ErrPermanent
    worker.go               # StartWorker + 主循环 + 退避 + reclaim
    testhelpers_test.go     # 测试 DB helper（SQLite 文件 + 注入）
    subscriber_test.go
    bus_test.go
    worker_test.go
    subscribers/
      logger/
        subscriber.go       # 内置 logger 订阅者（订阅 *）
  event_cleanup_task.go     # 每日清理任务（仅 master 节点运行）

setting/operation_setting/
  event_system.go           # 配置项（默认值，不入 Option 表；Phase 2 再做后台 UI）
```

> 与原计划差异：`event_log.go` / `event_dispatch.go` 不在 `model/` 包下。
> 详见第 5 节"实施备注"。

---

## 14. 测试策略

### 14.1 单元测试
- `subscriber_test.go`：`topicMatches` 各种通配场景；registry 注册去重
- `bus_test.go`：
  - 有/无外层事务的 Publish 行为
  - 无订阅者时仍写 event_log
  - 多订阅者扇出正确写入对应 dispatch 行
  - payload 序列化失败处理
- `worker_test.go`：
  - 单实例顺序处理
  - 多实例并发 claim 互斥（启动 2 个 worker 同时跑同一批数据，验证每行只被处理一次）
  - 重试退避时间计算
  - retry_count 达上限标记 dead
  - ErrPermanent 直接 dead
  - reclaim 卡住的 processing 行

### 14.2 集成测试
- 模拟一次完整 publish → worker pull → logger handle 链路
- 模拟事务回滚导致事件不入库
- 模拟订阅者反复失败到 dead 的全过程
- 清理任务对 done / dead / pending 的不同处理

### 14.3 三库兼容
- 单元测试默认 SQLite
- CI 至少跑一次 MySQL / PostgreSQL 的关键路径（claim SQL、清理 SQL、JOIN 查询）

---

## 15. Phase 划分

### Phase 1（本方案落地范围，~2 天）
- ✅ 两表 + AutoMigrate
- ✅ `service/events` 包（types / bus / subscriber / worker）
- ✅ 内置 logger 订阅者
- ✅ 11 处埋点（覆盖 7 个事件类型）
- ✅ 清理 cron + 配置项
- ✅ 单元测试 + 集成测试

### Phase 2（Resend 接入，~2 天）

**抽象层**（平台无关，未来可换 MailChimp/Sendgrid）：
- `service/marketing/types.go` —— Tier（None/Default/VIP）+ Intent + Provider interface
- `service/marketing/rules.go` —— 事件 → Intent 翻译，**状态从 DB 当前态算**，顺序无关 + 幂等
  - VIP 组用户：直接 TierVIP（含线下充值/admin 调额度的）
  - Default 组用户：必须 `GetUserTotalTopupAmount > 0` 才 TierDefault；纯免费用户 TierNone
  - 其他组（企业/admin 自定义）：TierNone → 从平台移除
- `service/marketing/registry.go` —— SetProvider / CurrentProvider 全局注入

**Resend 实现**：
- `service/marketing/providers/resend/{provider,client}.go`
- 用官方 SDK `github.com/resend/resend-go/v3`
- 4 个原子操作：upsertContact / addSegment / removeSegment / deleteContact
- 错误分类：4xx (非 429) → ErrPermanent；5xx/429/网络错 → worker 退避

**订阅者**：
- `service/events/subscribers/marketing/subscriber.go`
- 订阅：`BillingTopupSucceeded` / `UserGroupChanged` / `UserDeleted` / `UserEmailBound` / `UserProfileUpdated`
- 不订阅 `BillingRedemptionUsed`（兑换码不计入 `GetUserTotalTopupAmount`，不应触发"成为付费用户"）

**配置 + 后台 UI**：
- `setting/operation_setting/marketing.go`：MarketingEnabled / ResendAPIKey（加密入库）/ ResendDefaultSegmentID / ResendVIPSegmentID / ResendDefaultTopicIDs
- 后台"邮件营销"设置页：开关 + Provider 选择 + Resend 配置字段 + **"测试令牌"按钮**（点击调用 Resend `/audiences` 列表接口验证 key 有效性）
- 启用并配置令牌后**全自动运行**，无需任何其他操作

### Phase 3（运维与可观测，按需）
- Admin UI：`event_log` 浏览页（按 type / 时间 / aggregate_id 筛选）
- Admin UI：`event_dispatch` 失败队列页（dead 列表 + 手动重放按钮）
- Dead 告警接入 `service/user_notify.go`
- Prometheus metrics（pending 积压、各订阅者成功率与延迟）

---

## 16. 决策记录（对话过程中关键 Q&A）

| 问题 | 决策 | 原因 |
|---|---|---|
| 用同步 hook 还是异步事件总线？ | 异步事件总线 | Resend HTTP 慢且可能失败，不能挂在用户路径 |
| 内存事件总线还是 DB Outbox？ | DB Outbox | 充值/注册等是钱与合规相关，进程重启不能丢；可重放 |
| 单表 + per-subscriber 行，还是两表？ | **两表**（event_log + event_dispatch） | 事实/状态分离；无订阅者时也能审计；新订阅者可补发；admin UI 干净 |
| 事件命名分层？ | 命名约定 + 前缀通配（物理扁平） | 业界范式；零额外结构成本；订阅者灵活 |
| 多实例 worker 怎么互斥？ | 乐观 claim（UPDATE + RowsAffected） | 三库兼容；不依赖 FOR UPDATE SKIP LOCKED |
| 多实例都跑 worker 还是 master only？ | 所有实例都跑 | 天然 HA；某实例挂另一个继续 |
| 数据保留 | dispatch.done 30 天；event_log 365 天（可配）；dispatch.dead 永久 | 平衡审计与存储；事后加 TTL 比预先设计痛苦 |
| Phase 1 失败告警？ | 只记日志 | 先观察 dead 量级；Phase 3 接 user_notify |
| Phase 1 是否含 Resend？ | 否 | 先把基础设施做扎实，订阅者作为可插拔模块 |
| 是否收 `billing.redemption.used`？ | 收 | 跟真金白银充值要分开，便于财务分析 / 反薅羊毛 |
| 是否收 `billing.subscription.activated`？ | 暂不收 | 主要结果已被 `user.group.changed` 覆盖 |

---

## 17. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 订阅者 Handle 阻塞过久 | worker 层 30s 超时；订阅者自带更短客户端超时 |
| 大量 dead 堆积无人处理 | Phase 3 接入告警；Phase 1 通过 admin UI 人工查 |
| 清理任务在大表上锁竞争 | 分批 + 批间 sleep；NOT EXISTS 子查询走索引 |
| 多实例 reclaim 误抢正在工作的行 | reclaim 限制 `updated_at < now - 5min` 且 `worker_id=self` |
| 事件命名前期不规范导致后期改名痛苦 | 全部走常量；新增事件 review 时强制对齐命名规则 |
| event_log 表无 FK，dispatch 引用 event_log 可能出现孤儿 | 清理顺序：先删 done dispatch，再删 NOT EXISTS dispatch 引用的 event_log；不会出现孤儿 |
| Stripe 事务内 Publish 失败导致充值回滚 | 这是**期望行为**，事件与业务原子；Publish 本身只做本地 INSERT，失败概率极低 |

---

## 18. 后续可能的演进（不在当前方案）

- 把 `event_log` 作为统一审计日志的事实源（admin/channel/token 等 CRUD 也发事件）
- 引入 Inbox 模式：外部 webhook 进来也走同一套（如 Stripe webhook → publish event）
- 接 ClickHouse / 数据仓库做 OLAP 分析
- 引入 NATS / Kafka 替代 DB 轮询（仅在 QPS > 100 事件/秒时考虑）

---

## 19. Phase 1 实施总结

**已落地（2026-05）**：

| 模块 | 文件 | 状态 |
|---|---|---|
| 事件包基础类型 | `service/events/types.go` | ✅ 7 个事件常量 + 对应 Payload struct |
| 订阅者机制 | `service/events/subscriber.go` | ✅ interface + registry + 通配匹配 + ErrPermanent |
| 数据层 | `service/events/dao.go` | ✅ 两表 + SetDB + AutoMigrate + 全部 DAO（包级私有） |
| 发布 API | `service/events/bus.go` | ✅ Publish / PublishNoTx |
| Worker | `service/events/worker.go` | ✅ 主循环 + 退避 + reclaim + 优雅停机 |
| Logger 订阅者 | `service/events/subscribers/logger/subscriber.go` | ✅ 订阅 "*" |
| 配置 | `setting/operation_setting/event_system.go` | ✅ 默认值（暂未持久化到 Option 表） |
| 清理任务 | `service/event_cleanup_task.go` | ✅ master-only，每天检查到点跑 |
| 启动接线 | `main.go` | ✅ SetDB / AutoMigrate / StartWorker / StartEventCleanupTask |
| 测试 | `service/events/*_test.go` | ✅ 12 个测试，race-clean |

**埋点实际位置**（20 处 Publish 调用，含 Phase 1.5 / Phase 2 / 后续 review 补漏）：

| 事件 | 文件:函数 | trigger / source / delete_type | 事务 |
|---|---|---|---|
| `user.registered` | `controller/user.go:Register` | source=email | NoTx |
| `user.registered` | `controller/user.go:CreateUser` (admin 创建) | source=admin | NoTx |
| `user.registered` | `controller/oauth.go:findOrCreateOAuthUser` | source=github/discord/oidc/... | NoTx |
| `user.registered` | `controller/wechat.go:WeChatAuth` (首次登录建账) | source=wechat | NoTx |
| `user.deleted` | `controller/user.go:DeleteUser` | admin_hard | NoTx |
| `user.deleted` | `controller/user.go:DeleteSelf` | self_soft | NoTx |
| `user.deleted` | `controller/user.go:ManageUser` action=delete | admin_soft | NoTx |
| `user.profile.updated` | `controller/user.go:UpdateSelf` (diff-gated) | — | NoTx |
| `user.profile.updated` | `controller/user.go:UpdateUser` (admin edit, diff-gated) | — | NoTx |
| `user.email.bound` | `controller/user.go:EmailBind` | — | NoTx |
| `user.group.changed` | `model/topup.go:CheckAndUpgradeUserGroup` | topup | NoTx |
| `user.group.changed` | `controller/user.go:UpdateUser` (admin edit) | admin | NoTx |
| `user.group.changed` | `model/subscription.go:CreateUserSubscriptionFromPlanTx` (upgrade) | subscription | **Tx (best-effort)** |
| `user.group.changed` | `model/subscription.go:downgradeUserGroupForSubscriptionTx` | subscription | **Tx (best-effort)** |
| `user.group.changed` | `model/subscription.go` expire 流程（下游回退分组） | subscription | **Tx (best-effort)** |
| `billing.topup.succeeded` | `model/topup.go:Recharge` (Stripe) | — | **Tx (best-effort)** |
| `billing.topup.succeeded` | `model/topup.go:ManualCompleteTopUp` | — | **Tx (best-effort)** |
| `billing.topup.succeeded` | `model/topup.go:RechargeCreem` | — | **Tx (best-effort)** |
| `billing.topup.succeeded` | `model/topup.go:RechargeWaffo` | — | **Tx (best-effort)** |
| `billing.topup.succeeded` | `controller/topup.go:EpayNotify` (EPay 同步回调) | — | NoTx (回调已在事务外) |
| `billing.redemption.used` | `model/redemption.go:Redeem` | — | **Tx (best-effort)** |

> 所有 "Tx (best-effort)" 项实际调用的是 `events.PublishBestEffortInTx`（SAVEPOINT 隔离），publish 失败不阻塞业务事务。详见第 6 节"Publish API"与第 20 节。

**与原计划的差异 / 完整性补漏**：

1. **两表的归属**：从 `model/` 移到 `service/events/`（见第 5 节"实施备注"）
2. **OAuth 绑定埋点取消**：`handleOAuthBind` 仅绑定 provider id 不更新 email，
   归为 `user.email.bound` 语义上不合适；后期如有需要可加 `user.oauth.bound` 新事件
3. **设置项未持久化**：`EventLogRetentionDays` 等暂用代码常量，不入 Option 表；
   Phase 3 做管理 UI 时统一接入
4. **topup 路径细化**：覆盖了 Creem / Waffo / ManualCompleteTopUp 三条 Stripe 之外的路径
   （原设计只标了"EPay 等"）
5. **补漏：admin 用户操作**：原计划只覆盖 `DeleteUser`，review 后补加 `ManageUser` 软删
   与 `UpdateUser` 的 profile/group 变化（admin trigger）
6. **补漏：subscription 分组变化**：subscription 激活/降级/到期会**绕过** `CheckAndUpgradeUserGroup`
   直接 `tx.Model(&User).Update("group", ...)`。已在 `CreateUserSubscriptionFromPlanTx`、
   `downgradeUserGroupForSubscriptionTx`、subscription 到期下游分组回退流程三处补加
   `Publish(tx, UserGroupChanged, ..., Trigger: "subscription")`
7. **OAuth Group 默认值兜底**：OAuth 注册分支 `user.Group` 在内存里始终为空（DB default
   不会回填到 struct），事件 payload 显式回退到 `"default"`

**确认 Phase 1 落地完成。后续 Phase 2（Resend 订阅者）与 Phase 3（管理后台 / 告警）按本文件第 15 节推进。**

---

## 20. Phase 1.5 增量：best-effort in-tx publish

**问题**：原 Phase 1 在 model 层用 `events.Publish(tx, ...)`，publish 失败会回滚整个外层事务（充值 / 兑换 / 订阅 / 分组升级）。虽然概率极低（仅 event 表 INSERT 故障），但违背"营销子系统对用户无感知"原则。

**解决**：新增 `events.PublishBestEffortInTx`，用 GORM 嵌套 Transaction（底层 SAVEPOINT，三库支持）隔离 publish 的所有 DB 写入。publish 失败仅回滚到 savepoint，外层事务正常 commit。无返回值，失败仅日志。

**改造范围**：8 处 in-tx publish 全部从 `Publish(tx, ...)` 切到 `PublishBestEffortInTx(tx, ...)`：
- `model/topup.go`：Recharge / ManualCompleteTopUp / RechargeCreem / RechargeWaffo
- `model/redemption.go`：Redeem
- `model/subscription.go`：CreateUserSubscriptionFromPlanTx / downgradeUserGroupForSubscriptionTx / 到期回退流程

**测试**：4 个新增测试，覆盖
- 失败隔离（chan 类型让 marshal 失败 → 外层 tx 仍 commit 主数据）
- 成功路径（event_log 与主数据一起 commit）
- nil tx fallback（自动转 PublishNoTx）
- savepoint 回滚干净（drop dispatch 表强制失败，验证 event_log 行也被回滚）

**保留** `Publish(tx, ...)` 不删除，留给将来"事件丢失则业务必须回滚"的强一致场景。

**验收**：`go build ./...` 通过；`go test ./service/events/... -race` 全绿（17 个测试）。

---

## 21. Phase 2 实施总结：Resend 接入

### 21.1 落地清单

| 模块 | 文件 | 说明 |
|---|---|---|
| 平台无关抽象 | `service/marketing/types.go` | Tier（None/Default/VIP）+ Intent + Provider interface |
| 平台无关抽象 | `service/marketing/registry.go` | SetProvider / CurrentProvider 全局注入 |
| 业务规则引擎 | `service/marketing/rules.go` | Resolve：事件 → Intent，状态从 DB 当前态算 |
| 业务规则测试 | `service/marketing/rules_test.go` | 10 个场景：default 付/未付、vip 不查 topup、enterprise、删除、邮箱迁移、空邮箱、改名 fallback、顺序无关 |
| Resend Provider | `service/marketing/providers/resend/provider.go` | 实现 marketing.Provider；Sync 3 步：cleanup → upsert → 加目标 segment + 移除另一个 |
| Resend Client | `service/marketing/providers/resend/client.go` | SDK 薄封装 + 错误分类（429/4xx auth/404/未知） |
| Resend 测试 | `service/marketing/providers/resend/client_test.go` | 错误分类、already-exists 检测、name 拆分 |
| 事件订阅者 | `service/events/subscribers/marketing/subscriber.go` | 注册到 events 总线；Handle 调 Resolve + CurrentProvider.Sync |
| 配置 | `setting/operation_setting/marketing.go` | MarketingEnabled / MarketingProvider / ResendAPIKey / ResendDefaultSegmentID / ResendVIPSegmentID / ResendDefaultTopicIDs + OnMarketingConfigChanged 钩子 |
| Option 表集成 | `model/option.go` | InitOptionMap 注册 + updateOptionMap 处理 + 改动时触发 TriggerMarketingReload |
| 测试令牌端点 | `controller/marketing.go` + `router/api-router.go` | `POST /api/option/test_resend`，调一次 Contacts.List 验证 key |
| 启动接线 | `main.go` | 导入订阅者触发 init；安装 OnMarketingConfigChanged = rebuildMarketingProvider；启动时调一次 |

### 21.2 核心规则（service/marketing/rules.go）

```
tierForUser(user):
  group == "vip"     → TierVIP（不查 topup，含线下/admin 调额度的）
  group == "default" → 充值过 → TierDefault；纯免费 → TierNone
  其他              → TierNone（企业组 / admin 自定义组 → 不入平台 / 移除）

Resolve(event):
  user.deleted        → 用 payload 的 email 走 TierNone（DB 已查不到）
  user.email.bound    → 旧邮箱进 CleanupEmail，按当前 tier 同步新邮箱
  其他事件             → 查 DB 当前态 → tierForUser → Intent
```

### 21.3 订阅事件清单

```
billing.topup.succeeded    （触发首次进 TierDefault）
user.group.changed         （Default ↔ VIP / 进企业组 / 退企业组）
user.deleted               （从平台移除）
user.email.bound           （邮箱迁移：旧删 + 新加）
user.profile.updated       （更新姓名；免费用户 Resolve 返回 nil 自然 no-op）
```

不订阅 `user.registered`（注册时未付费）、`billing.redemption.used`（兑换码不计入 GetUserTotalTopupAmount，不应触发"成为付费用户"）。

### 21.4 热更新机制

```
admin 改后台配置
  → controller.UpdateOption（PUT /api/option）
  → model.UpdateOption → model.updateOptionMap
  → case "ResendAPIKey": ... operation_setting.TriggerMarketingReload()
  → main.go 注册的回调 rebuildMarketingProvider()
  → marketing.SetProvider(new resend provider)
  → 下一条事件 Handle 用上新 Provider
```

多实例场景下，`SyncOptions` 每隔 `common.SyncFrequency` 秒重新加载 DB 中的 option 表并 apply，因此其他实例也会自动 rebuild。

### 21.5 用户开启 Resend 的完整流程

1. admin 在后台"邮件营销"设置页填 ResendAPIKey、Segment IDs、Topic IDs
2. 点击"测试令牌"按钮 → `POST /api/option/test_resend` 返回有效/无效
3. 验证有效后打开总开关 → 后台保存
4. updateOptionMap 触发 `rebuildMarketingProvider` → Provider 注入 registry
5. 之后所有 billing.topup.succeeded / user.group.changed 等事件自动被 marketing 订阅者处理，对应 contact 自动出现在 Resend audience 里
6. 关闭总开关 → Provider 设为 nil → 后续事件不再同步（队列继续 drain）

### 21.6 与原设计的差异

1. **Resend Contacts API 现在是"全局 contact + segments 关联"模式**（不是 audience-centric）。
   实施时确认 SDK 不在 `CreateContactRequest` 暴露 `segments` 字段，必须分两步走：
   POST contact → POST /contacts/{email}/segments/{seg_id}。Provider.Sync 内部已经是
   这个序列。
2. **Topics 也必须分两步**：Create 后立刻 `Contacts.Topics.Update` 把默认 topics opt_in。
3. **错误分类靠 substring 匹配**：resend-go v3 没有把 HTTP status code 结构化暴露
   （除 429 有 `ErrRateLimit` 哨兵），所以 4xx 永久错误靠匹配 "invalid api key" /
   "unauthorized" / "forbidden" / "401" / "403" 等关键词识别。
4. **API key 不加密入库**：与现有 `EpayKey` / `StripeApiSecret` 处理一致，明文存储。
   依赖前端 UI mask 显示作为防御。

### 21.7 后台 UI（已完成）

新增 admin 设置 tab "邮件营销设置"，位置在"支付设置"之后。

- `web/src/components/settings/MarketingSetting.jsx` —— 单组件容器，6 个表单字段 +
  保存按钮 + "测试令牌"按钮
- `web/src/pages/Setting/index.jsx` —— 注册 tab，icon 用 `Mail`，itemKey="marketing"
- 翻译：`bun i18n:extract` 自动抽取到所有 7 个 locale 文件；`en.json` 已批量手工译出，
  其他语言走 zh-CN fallback（后续可补）

字段：
- 启用邮件营销同步（Switch）
- Provider（Select，目前只有 Resend）
- Resend API Key（password input，保存后不回显原文）
- Default User Segment ID
- VIP User Segment ID
- 默认 Topic IDs（逗号分隔）

"测试令牌"按钮的逻辑：用户在输入框里填了新 key 但还没保存时，前端把新 key 透传给
`POST /api/option/test_resend`，**不持久化、不切换 Provider**，只调一次 Resend
ListContacts 验证；否则后端回退到已保存的 key。

构建：`cd web && bun run build` ✅

### 21.8 仍未做

- **Marketing 订阅者集成测试**：subscriber.go 是 5 行胶水代码，已被 init 注册到
  events 总线（启动失败即报错），rules.go 和 client.go 都有独立单测。端到端集
  成测试（注入 mock Provider 验证 Handle 调用）可选；目前未做。

### 21.9 验收
- `go build ./...` ✅
- `go test ./service/events/... ./service/marketing/... ./model/... -race`：全过
- `cd web && bun run build` ✅
- `bun i18n:lint`：MarketingSetting.jsx 只剩 2 个全项目通用的 Switch 占位字符警告（`｜` / `〇`），与所有现有 settings 一致

---

## 22. Phase 2.5：历史付费用户回填

### 22.1 背景
启用 Resend 之前已经存在的付费用户不会被事件系统自动同步（事件只覆盖"从现在起"的新动作）。
提供一次性手工触发的回填机制。

### 22.2 设计选择

**不走 outbox** —— 直接调 `Provider.Sync`。原因：
- 历史数据可能上万行，灌成事件会污染 `event_log`（占 retention、被 logger 等订阅者重复处理）
- `Provider.Sync` 本就幂等，重跑只会刷新到当前状态
- 失败逐条记日志（`common.SysError`），admin 自行排查
- 不需要 dead 队列

**进程级互斥** —— `atomic.Bool` 防误触双击；多实例不互斥，因为 `Sync` 幂等

### 22.3 实现

| 模块 | 文件 | 说明 |
|---|---|---|
| 数据查询 | `model/user.go:GetPaidUserIDsBatch` | 游标分页（按 id 升序），过滤条件与 `tierForUser` 严格对齐（VIP 全量 + default 有成功 topup money>0）；三库兼容 |
| 回填逻辑 | `service/marketing/backfill.go:Backfill` | bounded worker pool（默认并发 2，配合 Resend 免费档 2 req/s 限流）；调 `resolveForUser` → `Provider.Sync`；统计 total/synced/skipped/failed |
| 状态记录 | `service/marketing/backfill.go` | `IsBackfillRunning` / `LastBackfillResult` 全局 atomic，运行期可实时查询进度 |
| API 端点 | `controller/marketing.go` + `router/api-router.go` | `POST /api/option/backfill_marketing`（异步触发）+ `GET /api/option/backfill_marketing/status`（查上次结果），都在 RootAuth 下 |
| 前端 UI | `web/src/components/settings/MarketingSetting.jsx` | "回填历史付费用户"按钮 + Popconfirm 确认 + 状态轮询 3s/次 + 上次结果显示 |

### 22.4 测试覆盖（5 个单测）

- `TestBackfill_RefusesWhenProviderNil`：Provider 未注入时返回 `ErrProviderNotConfigured`
- `TestBackfill_RefusesWhenAlreadyRunning`：互斥锁正确
- `TestBackfill_SyncsVIPAndPaidDefault_SkipsFreeAndEnterprise`：VIP 全收、default 付费收、免费 default + 企业组被查询条件直接排除
- `TestBackfill_CountsFailures`：Provider.Sync 失败计入 `Failed` + `LastError`
- `TestBackfill_FinishedResultStored`：结束后 `LastBackfillResult` 含 `FinishedAt`
- `TestBackfill_SkipsUsersWithoutEmail`：无邮箱用户被 SQL 直接排除（`email <> ''`）

### 22.5 运维注意

- **首次启用 Resend 后跑一次** —— 把存量用户灌进去
- **换 Segment ID 后跑一次** —— 让所有付费用户自动迁移到新 segment
- **不会重复创建** —— Resend 的 POST contact 对已存在邮箱返回 4xx，client.go 转 PATCH 处理
- **多实例部署** —— 进程级互斥不跨节点，但 Sync 幂等所以"两个实例同时跑回填"也不会出错，最多浪费一些 API 调用
- **小测试遗留** —— 添加了 `model.InitCommonColumnsForTest`（仅供测试初始化跨库列名变量）

---

## 23. Phase 2.7：软退订语义 + 用户自助订阅管理

### 23.1 两个相关需求合在一起做

1. **删用户硬删 / 退付费组软退订**：之前 `Tier=TierNone` 一律 DELETE contact，会丢失用户的退订状态、topic 偏好等。改成区分对待：
   - `user.deleted` 事件 → `RemovalHardDelete`（DELETE，GDPR 合规）
   - 其他事件导致 TierNone（如改企业组、自定义组）→ `RemovalSoftUnsubscribe`（PATCH unsubscribed=true，保留所有偏好）

2. **用户自助管理订阅**：让付费用户在 amux 个人设置页能看到细粒度 topic 列表 + 全局退订开关，自助勾选。

### 23.2 核心设计：amux 不存订阅状态，每次实时跟 provider 双向交互

- **零状态**：amux 不持久化用户的 `unsubscribed` 字段或 topic 偏好
- **天然同步**：用户在 Resend 邮件里点退订 → Resend 标记 `unsubscribed=true` → 下次用户进 amux 设置页时实时拉到这个新状态
- **零迁移**：换 provider（如 MailChimp）时 amux 没有 provider-specific 数据要清理
- **接口分层**：未来真要加缓存，可以包装 Provider 不动业务代码

### 23.3 Provider interface 扩展（3 个新方法）

```go
type Provider interface {
    Name() string
    Sync(ctx context.Context, intent Intent) error

    // 给 amux 用户设置页用：
    ListTopics(ctx context.Context, topicIDs []string) ([]Topic, error)
    GetSubscriptions(ctx context.Context, email string) (*Subscriptions, error)
    UpdateSubscriptions(ctx context.Context, email string, subs Subscriptions) error
}
```

新增类型：`Topic`、`TopicSubscription`、`Subscriptions`、`RemovalMode`（枚举 + Intent.RemovalMode 字段）。
`IsEligible(*model.User)` 公开方法供 controller 复用 tierForUser 业务规则。

### 23.4 Resend Provider 实现

`client.go` 新增 6 个方法：
- `markUnsubscribed` —— 软退订（PATCH unsubscribed=true，用 SDK 的 `SetUnsubscribed` setter 处理 bool omitempty 问题）
- `setUnsubscribed(email, bool)` —— 用户主动开关
- `getContact` —— 读 unsubscribed 字段
- `getContactTopics` —— 读 contact 的 topic 订阅
- `setContactTopics` —— 批量更新 topic 订阅
- `listTopicsByIDs` —— 按 admin 配的 ID 列表去 Resend 拉每个 topic 详情；**5min 内存缓存**避免每次设置页加载都打多次 GET /topics/{id}

`provider.go`：
- `Sync` 根据 `intent.RemovalMode` 分流走 `deleteContact` 或 `markUnsubscribed`
- 实现 `ListTopics` / `GetSubscriptions` / `UpdateSubscriptions` 三个新接口方法

### 23.5 用户端 API

| 端点 | 用途 |
|---|---|
| `GET /api/user/self/marketing_subscriptions` | 返回 `{ eligible, provider_configured, available_topics, current }` |
| `PUT /api/user/self/marketing_subscriptions` | 接收 `{ global_unsubscribed, topics: [{topic_id, subscribed}] }`，直接写回 Resend |

都在 `selfRoute`（`UserAuth` 中间件），用户只能改自己的。

### 23.6 前端

`web/src/components/settings/personal/cards/MarketingSubscriptions.jsx` —— 新增卡片，挂在 PersonalSetting 右侧 `NotificationSettings` 上方。

- provider 未配置 → 整个卡片**不展示**
- 非付费用户 → 显示卡片但**整个表单 disabled** + 顶部 Banner 提示"充值任意金额后即可设置"
- 付费用户：
  - 顶部 Switch："接收营销邮件"（对应 `global_unsubscribed` 反向）
  - 下方 topic 列表（admin 配的）：每个 topic 一行带 name + description + checkbox
  - 全局开关关闭时下方 checkbox 全部禁用（视觉提示 + 后端语义一致）
- 保存按钮 → PUT API → 写回 Resend

进卡片时 `GET /marketing_subscriptions` 用 `skipErrorHandler: true`，避免后端错误打扰用户。

### 23.7 i18n

新增 9 个 key × 7 种语言 = 63 条翻译，全部填充无空白。

### 23.8 测试

新增 `service/marketing/subscriptions_test.go`：
- `TestResolve_UserDeletedSetsHardDelete` —— user.deleted → RemovalHardDelete
- `TestResolve_NonDeletedKeepsDefaultSoftRemoval` —— 其他 TierNone → 默认软退订
- `TestIsEligible` —— 各种用户状态的资格判定（VIP / 付费 default / 免费 / 企业组 / 无邮箱 / nil）
- `TestProvider_RemovalMode_Counter` —— Sync 按 RemovalMode 分流正确
- `TestProvider_SubscriptionsRoundtrip` —— ListTopics/GetSubscriptions/UpdateSubscriptions 端到端

### 23.9 关键行为表

| 场景 | name | unsubscribed | topics | segment | contact 是否在 |
|---|---|---|---|---|---|
| 用户首次成为付费 | 设置 | false | opt_in 默认 topics | 加目标 segment | ✅ 新建 |
| Default ↔ VIP 切换 | PATCH | 不动 | 不动 | 切换 | ✅ 保留 |
| 退到企业组（软退订） | 不动 | PATCH true | 不动 | 不动 | ✅ 保留 |
| 用户删除（硬删） | — | — | — | — | ❌ DELETE |
| 用户在邮件里点退订 | 不动 | Resend 自动 true | 不动 | 不动 | ✅ 保留 |
| 用户在 amux 关全局开关 | 不动 | PATCH true | PATCH 用户选 | 不动 | ✅ 保留 |
| 用户在 amux 重新开 + 调 topic | 不动 | PATCH false | PATCH 用户选 | 不动 | ✅ 保留 |
| 多次 backfill | PATCH 名字 | 不动 | 不动（创建时设置后从不覆盖） | 加目标 segment | ✅ |

### 23.10 验收
- `go build ./...` ✅
- `go test ./service/events/... ./service/marketing/... ./model/... -race`：全过
- `cd web && bun run build` ✅
- 7 语言 marketing 翻译 0 空白
