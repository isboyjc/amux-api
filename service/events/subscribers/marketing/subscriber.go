// Package marketing 是事件总线和 service/marketing（平台无关营销层）之间的胶水。
//
// 职责仅有两件：
//  1. 在 init 注册到 events 总线，声明订阅哪些事件类型
//  2. 在 Handle 里把事件转交给 marketing.Resolve + 当前 Provider.Sync
//
// 不知道任何具体平台（Resend / MailChimp / ...）的存在，可插拔解耦完全由
// service/marketing 包负责。
package marketing

import (
	"context"

	"github.com/QuantumNous/new-api/service/events"
	"github.com/QuantumNous/new-api/service/marketing"
)

type Subscriber struct{}

func (Subscriber) Name() string { return "marketing" }

// Topics 订阅会改变"用户营销状态"的所有事件。
//
// 注意未订阅：
//   - events.UserRegistered：注册时还没付费，不入平台；首次充值才触发添加
//   - events.BillingRedemptionUsed：兑换码不计入 GetUserTotalTopupAmount，
//     用兑换码不会让用户被识别为"付费用户"
//   - events.UserProfileUpdated：免费用户改名也会发出此事件，但 Resolve 会查
//     DB 发现 TierNone 返回 nil → 自然 no-op
func (Subscriber) Topics() []string {
	return []string{
		events.BillingTopupSucceeded, // 触发首次进入 TierDefault（如果当前 group 仍是 default）
		events.UserGroupChanged,      // Default ↔ VIP 切换、被改到企业组、admin 调整等
		events.UserDeleted,           // 从平台移除
		events.UserEmailBound,        // 邮箱变更：旧邮箱删除 + 新邮箱同步当前 tier
		events.UserProfileUpdated,    // 更新姓名（仅 username/display_name 变化时事件才会发）
	}
}

// Handle 把事件交给 marketing 层处理。
//
// Provider 没注入（用户没开启功能或配置不完整）时直接返回 nil → worker 标记 done。
// 这样事件就被"消费掉"了，不会在队列里堆积；用户后期开启功能后，**只有新事件**会被
// 同步到平台。要补发历史用户，走后台的"回填历史付费用户"按钮（Phase 2.5）。
func (Subscriber) Handle(ctx context.Context, e events.Event) error {
	p := marketing.CurrentProvider()
	if p == nil {
		return nil
	}
	intent, err := marketing.Resolve(ctx, e)
	if err != nil {
		return err
	}
	if intent == nil {
		return nil
	}
	return p.Sync(ctx, *intent)
}

func init() {
	events.Register(Subscriber{})
}
