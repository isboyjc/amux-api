package events

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// ErrPermanent 是订阅者识别为"永久失败、不应重试"时应返回的哨兵错误。
// Worker 收到此错误会直接把 dispatch 标记为 dead，不再重试。
var ErrPermanent = errors.New("event handler permanent failure")

// Subscriber 是事件订阅者必须实现的接口。
//
// Topics() 支持三种 pattern：
//   - 精确匹配："user.registered"
//   - 末尾通配："user.*"、"billing.topup.*"
//   - 全订阅："*"
//
// Handle() 返回值约定：
//   - nil：成功，标记 done
//   - ErrPermanent：永久失败，标记 dead，不再重试
//   - 其他 error：临时失败，按退避策略重试
//
// 实现需要保证 Handle 是幂等的，因为同一事件在某些情况下可能被重试。
type Subscriber interface {
	Name() string
	Topics() []string
	Handle(ctx context.Context, e Event) error
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Subscriber{}
)

// Register 注册订阅者，重复注册同名 subscriber 会 panic（init 阶段的编程错误）。
func Register(s Subscriber) {
	if s == nil {
		panic("events: Register nil subscriber")
	}
	name := s.Name()
	if name == "" {
		panic("events: subscriber Name() returned empty string")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, ok := registry[name]; ok {
		panic("events: subscriber already registered: " + name)
	}
	registry[name] = s
}

// SubscribersFor 返回订阅了给定 eventType 的所有订阅者名称。
// Publish 调用时用于决定向 event_dispatch 写入哪些行。
func SubscribersFor(eventType string) []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	var matched []string
	for name, s := range registry {
		for _, t := range s.Topics() {
			if topicMatches(eventType, t) {
				matched = append(matched, name)
				break
			}
		}
	}
	return matched
}

// LookupSubscriber 根据名称查订阅者，worker 处理 dispatch 时使用。
func LookupSubscriber(name string) (Subscriber, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	s, ok := registry[name]
	return s, ok
}

// topicMatches 判断一个事件类型是否匹配一个订阅 pattern。
//   - pattern == "*" 匹配任意事件
//   - pattern == eventType 精确匹配
//   - pattern 形如 "user.*" 时匹配 "user." 前缀（但不包含 "user" 自身）
func topicMatches(eventType, pattern string) bool {
	if pattern == "*" || pattern == eventType {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, "*") // 保留末尾的 "."
		return strings.HasPrefix(eventType, prefix)
	}
	return false
}

// resetRegistryForTest 仅供测试使用，清空注册表。
func resetRegistryForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = map[string]Subscriber{}
}
