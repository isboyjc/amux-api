package marketing

import "sync"

// 全局 provider 注册点。由 main.go 在启动时根据配置注入；
// 配置热更新时也通过 SetProvider 切换实例。
//
// 设计取舍：相比"订阅者持有 provider 引用"，全局 registry 让"启动时未配置 +
// 运行时配置生效"的热更新更简单（无需重启订阅者，重新 SetProvider 即可）。

var (
	providerMu sync.RWMutex
	provider   Provider
)

// SetProvider 注入或替换当前 provider。传 nil 表示禁用（订阅者会变成 no-op）。
func SetProvider(p Provider) {
	providerMu.Lock()
	defer providerMu.Unlock()
	provider = p
}

// CurrentProvider 返回当前 provider；未注入时返回 nil。
func CurrentProvider() Provider {
	providerMu.RLock()
	defer providerMu.RUnlock()
	return provider
}
