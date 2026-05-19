package resend

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/service/marketing"
)

// Provider 实现 marketing.Provider，把 Intent 同步到 Resend。
//
// Sync 步骤：
//  1. CleanupEmail 非空：先 DELETE 旧邮箱（404 视为成功）
//  2. Tier == None：DELETE 目标邮箱
//  3. Tier == Default/VIP：upsert contact → 加目标 segment → 移除另一个 segment
//
// 三步序列设计为幂等：任何中断/重试，最终态都是"contact 存在且只在目标 segment"。
// Worker 失败重试会跑同样的序列，达到收敛。
type Provider struct {
	client          *client
	defaultSegment  string   // "Default User" segment ID
	vipSegment      string   // "VIP User" segment ID
	defaultTopicIDs []string // 创建 contact 时默认 opt_in 的 topic IDs
}

// Config 是构造 Provider 的入参，方便从 setting 解析后传入。
type Config struct {
	APIKey          string
	DefaultSegment  string
	VIPSegment      string
	DefaultTopicIDs []string
}

// New 创建一个 Resend Provider。APIKey 必填，其他可选（空 segment ID 时跳过对应操作）。
func New(cfg Config) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("resend: APIKey is required")
	}
	return &Provider{
		client:          newClient(cfg.APIKey),
		defaultSegment:  cfg.DefaultSegment,
		vipSegment:      cfg.VIPSegment,
		defaultTopicIDs: cfg.DefaultTopicIDs,
	}, nil
}

func (p *Provider) Name() string { return "resend" }

func (p *Provider) Sync(ctx context.Context, intent marketing.Intent) error {
	// 1. 旧邮箱清理（邮箱迁移场景）
	if intent.CleanupEmail != "" && intent.CleanupEmail != intent.TargetEmail {
		if err := p.client.deleteContact(ctx, intent.CleanupEmail); err != nil {
			return err
		}
	}

	// 2. TierNone：从平台移除。按 RemovalMode 走"软退订"还是"硬删"。
	//    软退订（默认）：用户退出付费组，保留 contact + 偏好（topic 订阅、Resend 后台手工设置）
	//    硬删（仅 user.deleted）：账号注销，GDPR 真删
	if intent.Tier == marketing.TierNone {
		if intent.TargetEmail == "" {
			return nil
		}
		if intent.RemovalMode == marketing.RemovalHardDelete {
			return p.client.deleteContact(ctx, intent.TargetEmail)
		}
		return p.client.markUnsubscribed(ctx, intent.TargetEmail)
	}

	// 3. Tier Default 或 VIP：upsert + 维护 segments
	if intent.TargetEmail == "" {
		return nil
	}

	targetSeg, otherSeg := p.segmentsFor(intent.Tier)

	if err := p.client.upsertContact(ctx, intent.TargetEmail, intent.DisplayName, p.defaultTopicIDs); err != nil {
		return err
	}
	if err := p.client.addSegment(ctx, intent.TargetEmail, targetSeg); err != nil {
		return err
	}
	if otherSeg != "" {
		if err := p.client.removeSegment(ctx, intent.TargetEmail, otherSeg); err != nil {
			return err
		}
	}
	return nil
}

// ListTopics 实现 marketing.Provider 接口：把 admin 配置的 topic id 列表
// 转成带 name/description 的 Topic 数组，给 amux 用户设置页渲染勾选列表。
func (p *Provider) ListTopics(ctx context.Context, topicIDs []string) ([]marketing.Topic, error) {
	return p.client.listTopicsByIDs(ctx, topicIDs)
}

// GetSubscriptions 实现 marketing.Provider 接口：查指定 contact 当前的全局退订状态
// + topic 订阅状态。每次用户进设置页都现拉，保证看到的总是最新真实状态。
func (p *Provider) GetSubscriptions(ctx context.Context, email string) (*marketing.Subscriptions, error) {
	contact, err := p.client.getContact(ctx, email)
	if err != nil {
		return nil, err
	}
	topics, err := p.client.getContactTopics(ctx, email)
	if err != nil {
		return nil, err
	}
	subs := &marketing.Subscriptions{
		Topics: make([]marketing.TopicSubscription, 0, len(topics)),
	}
	if contact != nil {
		subs.GlobalUnsubscribed = contact.Unsubscribed
	}
	for _, t := range topics {
		subs.Topics = append(subs.Topics, marketing.TopicSubscription{
			TopicID:    t.Id,
			Subscribed: t.Subscription == "opt_in",
		})
	}
	return subs, nil
}

// UpdateSubscriptions 实现 marketing.Provider 接口：把用户在 amux 的勾选写回 Resend。
//
// 三步（顺序关键）：
//  1. ensureContactExists 兜底建 contact（如果 worker 还没处理付费事件，contact 可能
//     根本不存在；不先建则 PATCH 会 404 → 被 classifyErr ignoreNotFound 静默吞掉 →
//     用户感知"保存成功"但实际什么都没写入）。POST 已存在错会被静默通过。
//  2. setUnsubscribed PATCH 全局退订状态
//  3. setContactTopics 批量更新 topic 订阅
func (p *Provider) UpdateSubscriptions(ctx context.Context, email, displayName string, subs marketing.Subscriptions) error {
	if err := p.client.ensureContactExists(ctx, email, displayName); err != nil {
		return err
	}
	if err := p.client.setUnsubscribed(ctx, email, subs.GlobalUnsubscribed); err != nil {
		return err
	}
	return p.client.setContactTopics(ctx, email, subs.Topics)
}

// 编译期断言：Provider 实现 marketing.Provider interface。
// 接口变更时这里会先报错，比业务 caller 报错更早暴露问题。
var _ marketing.Provider = (*Provider)(nil)

// segmentsFor 返回目标 segment（要加进去）和另一个 segment（要移除）。
func (p *Provider) segmentsFor(t marketing.Tier) (target, other string) {
	if t == marketing.TierVIP {
		return p.vipSegment, p.defaultSegment
	}
	// 默认走 TierDefault
	return p.defaultSegment, p.vipSegment
}

// Ping 用一次轻量调用验证 API key 是否有效。供"测试令牌"端点调用。
// 返回 nil 表示 key 有效；非 nil error 是给前端展示的原始错误。
func (p *Provider) Ping(ctx context.Context) error {
	return p.client.ping(ctx)
}
