// Package resend 是 marketing.Provider 的 Resend 实现。
//
// 设计说明：
//   - 用官方 SDK github.com/resend/resend-go/v3 做底层 HTTP，避免重复造轮子
//   - 对外只暴露 Provider；内部 client.go 负责薄包装 + 错误分类
//   - 错误分类基于 SDK 暴露的 ErrRateLimit 哨兵 + 错误消息的 substring 匹配
//     （SDK 没有把 HTTP 状态码做结构化暴露，只能这样）
package resend

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/service/events"
	"github.com/QuantumNous/new-api/service/marketing"

	rd "github.com/resend/resend-go/v3"
)

// client 是 SDK 的薄封装。所有方法都把 SDK 错误分类成 nil / events.ErrPermanent /
// 其他 error（让 worker 退避重试）。
type client struct {
	sdk *rd.Client

	// topicCache 缓存"按 id 查到的 topic 详情"，避免每次 amux 设置页加载都打多次
	// GET /topics/{id}。topic 变化频率极低，5min 已经足够。
	topicCacheMu  sync.RWMutex
	topicCache    map[string]marketing.Topic // id → Topic
	topicCacheExp time.Time
}

const topicCacheTTL = 5 * time.Minute

func newClient(apiKey string) *client {
	return &client{
		sdk:        rd.NewClient(apiKey),
		topicCache: map[string]marketing.Topic{},
	}
}

// classifyErr 把 Resend SDK 返回的 error 分类成 worker 友好的形式。
//
// - nil：成功
// - 内含 "404" / "not found"：视为"对象不存在"，对 DELETE/Remove 操作来说是成功
//   （isIgnoreNotFound=true 时返回 nil，否则返回原 err）
// - 401/403/422/400 等永久性 4xx 错误：返回 events.ErrPermanent
// - 429 (errors.Is ErrRateLimit)：返回原 err，worker 退避重试
// - 其他（5xx、网络错、SDK 内部错）：返回原 err，worker 退避重试
func classifyErr(err error, ignoreNotFound bool) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())

	// 429 速率限制 → 温和重试
	if errors.Is(err, rd.ErrRateLimit) {
		return err
	}

	// 404 / 不存在 → 对幂等的 DELETE 类操作是成功
	if ignoreNotFound && (strings.Contains(msg, "not found") || strings.Contains(msg, "404")) {
		return nil
	}

	// 永久性配置/权限错误 → 让 worker 直接 dead，不要白白重试 6 次
	if strings.Contains(msg, "invalid api key") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "401") ||
		strings.Contains(msg, "403") {
		return events.ErrPermanent
	}

	// 已存在（POST 同 email）→ 调用方按"already exists"处理，先返回原 err
	// 调用方通过 errIsAlreadyExists 检测
	return err
}

// errIsAlreadyExists 检测 POST contact 返回"邮箱已存在"类的错误。
// Resend 对重复 email 返回 422 + 消息含 "already exists" 之类。
func errIsAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "already_exists") ||
		strings.Contains(msg, "contact_already_exists")
}

// upsertContact 创建或更新 contact。先 POST，409/422 (已存在) 转 PATCH。
//
// 创建时附带 topic opt-in 列表（仅创建时生效，PATCH 不带 topics）。
func (c *client) upsertContact(ctx context.Context, email, displayName string, topicIDs []string) error {
	firstName, lastName := splitName(displayName)

	createReq := &rd.CreateContactRequest{
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
	}
	_, err := c.sdk.Contacts.CreateWithContext(ctx, createReq)
	if err == nil {
		// 创建成功后，立刻 opt-in 默认 topics（SDK CreateContactRequest 不含 topics 字段，
		// 必须分两步走）
		return c.optInTopics(ctx, email, topicIDs)
	}
	if errIsAlreadyExists(err) {
		// 转 PATCH 更新名字
		updateReq := &rd.UpdateContactRequest{
			Email:     email,
			FirstName: firstName,
			LastName:  lastName,
		}
		_, perr := c.sdk.Contacts.UpdateWithContext(ctx, updateReq)
		return classifyErr(perr, false)
	}
	return classifyErr(err, false)
}

// optInTopics 把 contact 订阅到指定 topics 列表（opt_in）。
// topicIDs 为空时直接返回 nil。
func (c *client) optInTopics(ctx context.Context, email string, topicIDs []string) error {
	if len(topicIDs) == 0 {
		return nil
	}
	updates := make([]rd.TopicSubscriptionUpdate, 0, len(topicIDs))
	for _, id := range topicIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		updates = append(updates, rd.TopicSubscriptionUpdate{
			Id:           id,
			Subscription: "opt_in",
		})
	}
	if len(updates) == 0 {
		return nil
	}
	_, err := c.sdk.Contacts.Topics.UpdateWithContext(ctx, &rd.UpdateContactTopicsRequest{
		Email:  email,
		Topics: updates,
	})
	return classifyErr(err, true) // contact 可能正好被并发删除
}

// addSegment 把 contact 加到指定 segment。如已在该 segment（409）视为成功。
func (c *client) addSegment(ctx context.Context, email, segmentID string) error {
	if segmentID == "" {
		return nil
	}
	_, err := c.sdk.Contacts.Segments.AddWithContext(ctx, &rd.AddContactSegmentRequest{
		Email:     email,
		SegmentId: segmentID,
	})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "already") {
		return nil // 已在该 segment
	}
	return classifyErr(err, false)
}

// removeSegment 从指定 segment 移除 contact。404 视为成功。
func (c *client) removeSegment(ctx context.Context, email, segmentID string) error {
	if segmentID == "" {
		return nil
	}
	_, err := c.sdk.Contacts.Segments.RemoveWithContext(ctx, &rd.RemoveContactSegmentRequest{
		Email:     email,
		SegmentId: segmentID,
	})
	return classifyErr(err, true)
}

// deleteContact 整体删除 contact。404 视为成功。
func (c *client) deleteContact(ctx context.Context, email string) error {
	_, err := c.sdk.Contacts.RemoveWithContext(ctx, &rd.RemoveContactOptions{Id: email})
	return classifyErr(err, true)
}

// ping 用一次轻量 GET 验证 API key 是否有效。供"测试令牌"端点用。
//
// 用 Contacts.List 因为它最便宜（即使 audience 为空也返回 200）。
func (c *client) ping(ctx context.Context) error {
	_, err := c.sdk.Contacts.ListWithContext(ctx, &rd.ListContactsOptions{})
	return err // 不分类，原样返回给前端展示
}

// ensureContactExists 确保指定 email 的 contact 存在；不存在就用 POST 创建，已存在
// 静默通过。比完整 upsertContact 轻量（不 opt_in 默认 topics，不 PATCH 名字）。
//
// 用途：用户自助 UpdateSubscriptions 之前防御性兜底，处理"刚付费 + worker 还没处理
// 事件 + 用户立即进设置页"的竞态。
func (c *client) ensureContactExists(ctx context.Context, email, displayName string) error {
	firstName, lastName := splitName(displayName)
	_, err := c.sdk.Contacts.CreateWithContext(ctx, &rd.CreateContactRequest{
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
	})
	if err == nil {
		return nil
	}
	if errIsAlreadyExists(err) {
		return nil // 已存在，正合我意
	}
	return classifyErr(err, false)
}

// markUnsubscribed 把 contact 标记为全局退订（PATCH unsubscribed=true）。
// 用于"用户退出付费组"场景：保留 contact + 保留所有偏好（topic 订阅、Resend 后台手工设置），
// 只是不再发营销邮件。是 RemovalSoftUnsubscribe 的实现。
//
// 注意：SDK 的 Unsubscribed 字段是 `json:"unsubscribed,omitempty"`，bool false 会被 omit；
// 必须用 SetUnsubscribed setter 来强制把字段写进 JSON。
func (c *client) markUnsubscribed(ctx context.Context, email string) error {
	req := &rd.UpdateContactRequest{Email: email}
	req.SetUnsubscribed(true)
	_, err := c.sdk.Contacts.UpdateWithContext(ctx, req)
	return classifyErr(err, true) // 404 当成功（已经不在了，效果一致）
}

// setUnsubscribed 显式设置 unsubscribed 为指定布尔值（true=退订 / false=订阅）。
// 用于用户在 amux 设置页主动开关全局订阅。
func (c *client) setUnsubscribed(ctx context.Context, email string, unsubscribed bool) error {
	req := &rd.UpdateContactRequest{Email: email}
	req.SetUnsubscribed(unsubscribed)
	_, err := c.sdk.Contacts.UpdateWithContext(ctx, req)
	return classifyErr(err, true)
}

// getContact 获取 contact 全量信息，主要用来读 Unsubscribed 字段。
// 404 返回 nil（contact 还没创建），上层应当 0 值处理。
func (c *client) getContact(ctx context.Context, email string) (*rd.Contact, error) {
	contact, err := c.sdk.Contacts.GetWithContext(ctx, &rd.GetContactOptions{Id: email})
	if err != nil {
		// 404 → 当不存在
		if msg := strings.ToLower(err.Error()); strings.Contains(msg, "not found") || strings.Contains(msg, "404") {
			return nil, nil
		}
		return nil, classifyErr(err, false)
	}
	return &contact, nil
}

// getContactTopics 取 contact 当前订阅的 topic 列表（含 opt_in/opt_out 状态）。
// 404 视为"contact 不存在"，返回空切片。
func (c *client) getContactTopics(ctx context.Context, email string) ([]rd.ContactTopic, error) {
	resp, err := c.sdk.Contacts.Topics.ListWithContext(ctx, email)
	if err != nil {
		if msg := strings.ToLower(err.Error()); strings.Contains(msg, "not found") || strings.Contains(msg, "404") {
			return nil, nil
		}
		return nil, classifyErr(err, false)
	}
	return resp.Data, nil
}

// setContactTopics 批量更新 contact 对各 topic 的订阅状态。
func (c *client) setContactTopics(ctx context.Context, email string, subs []marketing.TopicSubscription) error {
	if len(subs) == 0 {
		return nil
	}
	updates := make([]rd.TopicSubscriptionUpdate, 0, len(subs))
	for _, s := range subs {
		sub := "opt_out"
		if s.Subscribed {
			sub = "opt_in"
		}
		updates = append(updates, rd.TopicSubscriptionUpdate{
			Id:           s.TopicID,
			Subscription: sub,
		})
	}
	_, err := c.sdk.Contacts.Topics.UpdateWithContext(ctx, &rd.UpdateContactTopicsRequest{
		Email:  email,
		Topics: updates,
	})
	return classifyErr(err, true)
}

// listTopicsByIDs 把 admin 配置的 topic id 列表 → 带 name/description 的完整 Topic 列表。
//
// 缓存策略：在 topicCacheTTL（5min）内复用上一轮拉到的全部数据，
// 缓存失效后并发拉取所有 id（最多 16 个并发，避免轻易触发限流）。
//
// 单个 id 拉取失败时该 id 被静默跳过（不抛错），返回结果可能比入参 ids 少；
// 上层 UI 据此知道该 topic 配错了或已被删除。
func (c *client) listTopicsByIDs(ctx context.Context, ids []string) ([]marketing.Topic, error) {
	ids = uniqueNonEmpty(ids)
	if len(ids) == 0 {
		return nil, nil
	}

	c.topicCacheMu.RLock()
	if time.Now().Before(c.topicCacheExp) {
		// 命中：从缓存按入参顺序取
		out := make([]marketing.Topic, 0, len(ids))
		hit := true
		for _, id := range ids {
			if t, ok := c.topicCache[id]; ok {
				out = append(out, t)
			} else {
				hit = false
				break
			}
		}
		c.topicCacheMu.RUnlock()
		if hit {
			return out, nil
		}
	} else {
		c.topicCacheMu.RUnlock()
	}

	// 未命中：并发拉所有 id
	type result struct {
		id    string
		topic *marketing.Topic
		err   error
	}
	const maxConcurrent = 16
	sem := make(chan struct{}, maxConcurrent)
	results := make(chan result, len(ids))
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(tid string) {
			defer func() { <-sem; wg.Done() }()
			t, err := c.sdk.Topics.GetWithContext(ctx, tid)
			if err != nil {
				results <- result{id: tid, err: err}
				return
			}
			results <- result{id: tid, topic: &marketing.Topic{
				ID:          t.Id,
				Name:        t.Name,
				Description: t.Description,
			}}
		}(id)
	}
	wg.Wait()
	close(results)

	// 收集 + 重新按入参顺序排序
	byID := map[string]marketing.Topic{}
	for r := range results {
		if r.err == nil && r.topic != nil {
			byID[r.id] = *r.topic
		}
	}
	out := make([]marketing.Topic, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			out = append(out, t)
		}
	}

	// 更新缓存
	c.topicCacheMu.Lock()
	c.topicCache = byID
	c.topicCacheExp = time.Now().Add(topicCacheTTL)
	c.topicCacheMu.Unlock()

	return out, nil
}

func uniqueNonEmpty(s []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(s))
	for _, v := range s {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// splitName 把 "Foo Bar" 拆成 first / last。只有一个词就全放 first。
func splitName(name string) (first, last string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	parts := strings.SplitN(name, " ", 2)
	first = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		last = strings.TrimSpace(parts[1])
	}
	return first, last
}
