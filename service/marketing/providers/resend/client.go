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
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/events"
	"github.com/QuantumNous/new-api/service/marketing"

	rd "github.com/resend/resend-go/v3"
	"golang.org/x/time/rate"
)

// client 是 SDK 的薄封装。所有方法都把 SDK 错误分类成 nil / events.ErrPermanent /
// 其他 error（让 worker 退避重试）。
//
// 限流策略：Resend 默认 5 req/s。这里用客户端令牌桶限到 4 req/s（留 20% 余量
// 给系统抖动 + 多实例部署的不均衡），并对 429 错误自动按 retry-after 重试。
// 单进程内所有 goroutine 共享同一个 limiter；多实例部署时各自跑 4/s，
// 总和可能超过 5/s 触发 429，但 do() 的重试会兜底。
type client struct {
	sdk     *rd.Client
	limiter *rate.Limiter

	// topicCache 缓存"按 id 查到的 topic 详情"，避免每次 amux 设置页加载都打多次
	// GET /topics/{id}。topic 变化频率极低，5min 已经足够。
	topicCacheMu  sync.RWMutex
	topicCache    map[string]marketing.Topic // id → Topic
	topicCacheExp time.Time
}

const (
	topicCacheTTL = 5 * time.Minute

	// resendRateLimit 客户端发出请求的目标速率（req/s）。
	// Resend 默认 5/s，4/s 留 20% 余量给抖动 + 多实例分布不均。
	resendRateLimit = 4

	// resendRateBurst 令牌桶突发容量。允许冷启动瞬间 4 个并发请求。
	resendRateBurst = 4

	// maxRetryOn429 同一次调用遇到 429 后最多再试几次（首次失败后 N 次重试 = N+1 次总尝试）。
	// 配合 maxTotalSleep 双保险：到次数或到时长任一上限就放弃。
	maxRetryOn429 = 3

	// maxTotalSleep 单次 do() 在 429 重试上累计 sleep 的上限。
	// Sync 会串 3-4 个 do()；用 20s 上限确保即便每个都到顶，整个 Sync 也
	// 在 ~80s 完成，不会撞 backfill 的 120s SyncTimeout。
	maxTotalSleep = 20 * time.Second
)

func newClient(apiKey string) *client {
	return &client{
		sdk:        rd.NewClient(apiKey),
		limiter:    rate.NewLimiter(rate.Limit(resendRateLimit), resendRateBurst),
		topicCache: map[string]marketing.Topic{},
	}
}

// do 包装一次 Resend SDK 调用：
//  1. 先在客户端令牌桶限流（防止主动打爆 Resend 5/s 限制）
//  2. 真正发起请求
//  3. 遇到 *rd.RateLimitError，按 retry-after sleep 后重试，最多 maxRetryOn429 次
//     且累计 sleep 不超过 maxTotalSleep
//
// 非 429 错误直接返回，由 classifyErr 决定后续语义。ctx 取消时立即返回 ctx.Err()。
//
// 关键不变量：依赖 Resend SDK 把 429 原样返回为 *rd.RateLimitError 裸指针。
// 如果未来某天 SDK 把这个错误用 fmt.Errorf("%w") 包了一层，errors.As 仍能识别；
// 但如果替换成纯字符串包装（fmt.Errorf("rate limit: %v")），识别会失效，需要回退
// 到 strings.Contains 兜底。
func (c *client) do(ctx context.Context, fn func() error) error {
	var (
		lastErr    error
		totalSleep time.Duration
	)
	for attempt := 0; attempt <= maxRetryOn429; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}
		err := fn()
		if err == nil {
			return nil
		}
		var rle *rd.RateLimitError
		if !errors.As(err, &rle) {
			return err
		}
		lastErr = err
		if attempt == maxRetryOn429 {
			common.SysError(fmt.Sprintf("[resend] 429 give up after %d attempts: %s",
				attempt+1, err.Error()))
			break
		}
		// retry-after 是 Resend 给的明确等待秒数；加少量抖动避免多实例同时唤醒
		sleepDur := parseRetryAfter(rle.RetryAfter) +
			time.Duration(150+attempt*200)*time.Millisecond
		if totalSleep+sleepDur > maxTotalSleep {
			common.SysError(fmt.Sprintf(
				"[resend] 429 give up (total sleep cap %s reached after %d attempts): %s",
				maxTotalSleep, attempt+1, err.Error()))
			break
		}
		totalSleep += sleepDur
		// 第 2 次起才 log（每次都 log 会刷屏；首次重试是常见的瞬时抖动）
		if attempt >= 1 {
			common.SysLog(fmt.Sprintf(
				"[resend] 429 retry attempt=%d sleeping=%s retry-after=%s",
				attempt+1, sleepDur, rle.RetryAfter))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepDur):
		}
	}
	return lastErr
}

// parseRetryAfter 解析 RateLimitError.RetryAfter。
// 优先按"秒数"解析（Resend 目前用法）；失败时退回 1 秒。
// HTTP RFC 7231 允许 HTTP-date 格式，但 Resend 现在不用；如果未来切换需要
// 在这里加 http.ParseTime 兜底。
func parseRetryAfter(s string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return time.Second
	}
	return time.Duration(n) * time.Second
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
	err := c.do(ctx, func() error {
		_, e := c.sdk.Contacts.CreateWithContext(ctx, createReq)
		return e
	})
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
		perr := c.do(ctx, func() error {
			_, e := c.sdk.Contacts.UpdateWithContext(ctx, updateReq)
			return e
		})
		if perr != nil {
			return classifyErr(perr, false)
		}

		// 半状态恢复：如果 contact 已存在但完全没有 topic 订阅，说明上次创建后
		// optInTopics 步骤失败（典型场景：429 中断了创建后的 opt_in）。补一次。
		//
		// 仅在「零订阅」时补，避免覆盖用户主动 opt_out 过的偏好（用户改名 → 触发
		// UserProfileUpdated → Sync 走到这里 → 不应重置用户已选择的 topic 偏好）。
		if len(topicIDs) == 0 {
			return nil
		}
		existing, gerr := c.getContactTopics(ctx, email)
		if gerr != nil {
			// 读不到 topic 列表，保守起见不动；下次回填会再试
			return nil
		}
		if len(existing) == 0 {
			return c.optInTopics(ctx, email, topicIDs)
		}
		return nil
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
	err := c.do(ctx, func() error {
		_, e := c.sdk.Contacts.Topics.UpdateWithContext(ctx, &rd.UpdateContactTopicsRequest{
			Email:  email,
			Topics: updates,
		})
		return e
	})
	return classifyErr(err, true) // contact 可能正好被并发删除
}

// addSegment 把 contact 加到指定 segment。如已在该 segment（409）视为成功。
func (c *client) addSegment(ctx context.Context, email, segmentID string) error {
	if segmentID == "" {
		return nil
	}
	err := c.do(ctx, func() error {
		_, e := c.sdk.Contacts.Segments.AddWithContext(ctx, &rd.AddContactSegmentRequest{
			Email:     email,
			SegmentId: segmentID,
		})
		return e
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
	err := c.do(ctx, func() error {
		_, e := c.sdk.Contacts.Segments.RemoveWithContext(ctx, &rd.RemoveContactSegmentRequest{
			Email:     email,
			SegmentId: segmentID,
		})
		return e
	})
	return classifyErr(err, true)
}

// deleteContact 整体删除 contact。404 视为成功。
func (c *client) deleteContact(ctx context.Context, email string) error {
	err := c.do(ctx, func() error {
		_, e := c.sdk.Contacts.RemoveWithContext(ctx, &rd.RemoveContactOptions{Id: email})
		return e
	})
	return classifyErr(err, true)
}

// ping 用一次轻量 GET 验证 API key 是否有效。供"测试令牌"端点用。
//
// 用 Contacts.List 因为它最便宜（即使 audience 为空也返回 200）。
func (c *client) ping(ctx context.Context) error {
	return c.do(ctx, func() error {
		_, e := c.sdk.Contacts.ListWithContext(ctx, &rd.ListContactsOptions{})
		return e
	}) // 不 classifyErr，原样返回给前端展示
}

// ensureContactExists 确保指定 email 的 contact 存在；不存在就用 POST 创建，已存在
// 静默通过。比完整 upsertContact 轻量（不 opt_in 默认 topics，不 PATCH 名字）。
//
// 用途：用户自助 UpdateSubscriptions 之前防御性兜底，处理"刚付费 + worker 还没处理
// 事件 + 用户立即进设置页"的竞态。
func (c *client) ensureContactExists(ctx context.Context, email, displayName string) error {
	firstName, lastName := splitName(displayName)
	err := c.do(ctx, func() error {
		_, e := c.sdk.Contacts.CreateWithContext(ctx, &rd.CreateContactRequest{
			Email:     email,
			FirstName: firstName,
			LastName:  lastName,
		})
		return e
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
	err := c.do(ctx, func() error {
		_, e := c.sdk.Contacts.UpdateWithContext(ctx, req)
		return e
	})
	return classifyErr(err, true) // 404 当成功（已经不在了，效果一致）
}

// setUnsubscribed 显式设置 unsubscribed 为指定布尔值（true=退订 / false=订阅）。
// 用于用户在 amux 设置页主动开关全局订阅。
func (c *client) setUnsubscribed(ctx context.Context, email string, unsubscribed bool) error {
	req := &rd.UpdateContactRequest{Email: email}
	req.SetUnsubscribed(unsubscribed)
	err := c.do(ctx, func() error {
		_, e := c.sdk.Contacts.UpdateWithContext(ctx, req)
		return e
	})
	return classifyErr(err, true)
}

// getContact 获取 contact 全量信息，主要用来读 Unsubscribed 字段。
// 404 返回 nil（contact 还没创建），上层应当 0 值处理。
func (c *client) getContact(ctx context.Context, email string) (*rd.Contact, error) {
	var contact rd.Contact
	err := c.do(ctx, func() error {
		var e error
		contact, e = c.sdk.Contacts.GetWithContext(ctx, &rd.GetContactOptions{Id: email})
		return e
	})
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
	var resp rd.ListContactTopicsResponse
	err := c.do(ctx, func() error {
		var e error
		resp, e = c.sdk.Contacts.Topics.ListWithContext(ctx, email)
		return e
	})
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
	err := c.do(ctx, func() error {
		_, e := c.sdk.Contacts.Topics.UpdateWithContext(ctx, &rd.UpdateContactTopicsRequest{
			Email:  email,
			Topics: updates,
		})
		return e
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
			var t *rd.Topic
			err := c.do(ctx, func() error {
				var e error
				t, e = c.sdk.Topics.GetWithContext(ctx, tid)
				return e
			})
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
