package operation_setting

// 事件总线 / Outbox 配置。设计文档见 docs/event-system-design.md。
//
// 这里的默认值经过测算适合 30 万事件/年量级，调整前请阅读设计文档第 9 节
// "数据保留策略"。
var (
	// 是否启用 worker。关闭后业务仍可 Publish（事件继续入库），但不会被分发。
	EventWorkerEnabled = true

	// Worker 轮询间隔（毫秒）。
	EventWorkerPollIntervalMs = 2000

	// Worker 单轮拉取的 pending 行数上限。
	EventWorkerBatchSize = 50

	// Worker 进程内并发 handler 数。
	EventWorkerConcurrency = 4

	// 单次 Handle 上下文超时（毫秒）。订阅者自身应自带更短的客户端超时。
	EventHandleTimeoutMs = 30000

	// event_log 保留天数（按 published_at 计）。0 = 永不删。
	// 365 天默认覆盖常规审计窗口；调大可保留更长，但要观察表体积。
	EventLogRetentionDays = 365

	// event_dispatch 中 done 行的保留天数（按 processed_at 计）。
	// dead 行永不清理，pending/processing 自然不在清理范围。
	EventDispatchDoneRetentionDays = 30

	// 清理任务单批 DELETE 行数，避免长锁。
	EventCleanupBatchSize = 1000

	// 每天清理任务运行的小时（UTC）。默认凌晨 3 点。
	EventCleanupHourUTC = 3
)
