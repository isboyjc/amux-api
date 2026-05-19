package marketing

import (
	"context"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/events"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// Resolve 把一个事件翻译为"目标平台应当处于的状态"（Intent）。
//
// 设计原则：
//
//  1. **状态从 DB 当前态算，不从事件 payload 拼**。
//     这保证事件到达顺序无关、重放幂等、新增触发事件无需改 Resolve。
//     例如"首次充值 + 自动升级 VIP"会产生 2 个事件 (topup.succeeded + group.changed)，
//     无论 worker 哪一个先处理，Resolve 都查 DB 拿到当前最终态，最终都收敛到同一 Intent。
//
//  2. **业务规则只在 tierForUser 里**。新增 tier 或调整入会条件 → 只改一个函数。
//
//  3. **事件 payload 仅用于"DB 已查不到的情况"**（如 user.deleted 时 user 行已被
//     soft delete；user.email.bound 时要清理旧邮箱）。
//
// 返回 nil 表示无需操作（如纯免费用户改名）。
func Resolve(ctx context.Context, e events.Event) (*Intent, error) {
	// 1. user.deleted 特例：DB 里查不到（硬删）或软删后查不到 email，
	//    必须用 payload 里的 email 来"按邮箱删除"。
	//    GDPR / 被遗忘权要求真删 → RemovalHardDelete。
	if e.Type == events.UserDeleted {
		var p events.UserDeletedPayload
		if err := common.Unmarshal(e.Payload, &p); err != nil {
			return nil, events.ErrPermanent
		}
		if p.Email == "" {
			return nil, nil // 没邮箱，平台里本来也没有
		}
		return &Intent{
			TargetEmail: p.Email,
			Tier:        TierNone,
			RemovalMode: RemovalHardDelete,
		}, nil
	}

	// 2. user.email.bound 特例：可能需要清理旧邮箱
	var cleanupEmail string
	if e.Type == events.UserEmailBound {
		var p events.UserEmailBoundPayload
		if err := common.Unmarshal(e.Payload, &p); err != nil {
			return nil, events.ErrPermanent
		}
		if p.OldEmail != "" && p.OldEmail != p.NewEmail {
			cleanupEmail = p.OldEmail
		}
	}

	// 3. 其余事件统一：查 DB 当前态 → 计算 tier
	user, err := model.GetUserById(e.AggregateId, false)
	if err != nil {
		// 用户可能已被删除（GetUserById 通常返回 "user not found"）
		// 这种情况，如果有旧邮箱要清理就清理，否则忽略
		if cleanupEmail != "" {
			return &Intent{TargetEmail: cleanupEmail, Tier: TierNone}, nil
		}
		return nil, nil
	}
	if user == nil || user.Email == "" {
		if cleanupEmail != "" {
			return &Intent{TargetEmail: cleanupEmail, Tier: TierNone}, nil
		}
		return nil, nil
	}

	tier, err := tierForUser(user)
	if err != nil {
		// DB 错误（如查充值汇总失败）→ 普通 error 让 worker 重试
		return nil, err
	}

	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Username
	}

	return &Intent{
		TargetEmail:  user.Email,
		DisplayName:  displayName,
		Tier:         tier,
		CleanupEmail: cleanupEmail,
	}, nil
}

// IsEligible 判断用户是否有资格在 amux 设置页管理营销订阅。
//
// 这里的"资格"和事件驱动同步的 tier 是两个独立概念：
//   - tier（tierForUser）→ amux 是否要"自动"把这个用户的状态推到 Resend
//   - eligibility（本函数）→ 用户能不能在 UI 上自助管理订阅偏好
//
// 大多数情况两者一致（vip / 付费 default 既被自动同步，也能 UI 自助管理）。
// 特例：企业 / 自定义组用户走 admin 手动维护，amux 不该自动同步（tier=TierNone），
// 但应该让他们能 UI 自助管理 —— 通过 operation_setting.MarketingExtraEligibleGroups
// 配置开放这些组的 UI 资格。
func IsEligible(u *model.User) (bool, error) {
	if u == nil || u.Email == "" {
		return false, nil
	}
	tier, err := tierForUser(u)
	if err != nil {
		return false, err
	}
	if tier != TierNone {
		return true, nil
	}
	if isInExtraEligibleGroups(u.Group) {
		return true, nil
	}
	return false, nil
}

// isInExtraEligibleGroups 检查 group 是否在 admin 配置的「额外允许自助订阅」列表里。
// 列表为空或 group 为空时返回 false。比较时去掉首尾空白，区分大小写
// （与 user.Group 在 DB 里的存储保持一致）。
func isInExtraEligibleGroups(group string) bool {
	if group == "" {
		return false
	}
	raw := operation_setting.MarketingExtraEligibleGroups
	if raw == "" {
		return false
	}
	for _, g := range strings.Split(raw, ",") {
		if strings.TrimSpace(g) == group {
			return true
		}
	}
	return false
}

// tierForUser 当前用户状态 → tier 的判定。
//
// **业务规则的唯一定义点**。修改入会条件只需改这里，所有 Provider 自动适配。
//
// 规则：
//   - vip 组：直接 TierVIP（含线下充值/admin 调额度的；不查充值记录）
//   - default 组：必须 GetUserTotalTopupAmount > 0（真实付费过）才进 TierDefault；
//     纯免费用户 / 仅有 admin 加额度的 default 用户 → TierNone（不进平台）
//   - 其他组（企业组 / admin 自定义组）→ TierNone（运营在平台侧人工管理）
//   - 兑换码不计入"付费"（GetUserTotalTopupAmount 只算 top_ups 表 status=success）
func tierForUser(u *model.User) (Tier, error) {
	switch u.Group {
	case "vip":
		return TierVIP, nil
	case "default":
		total, err := model.GetUserTotalTopupAmount(u.Id)
		if err != nil {
			return TierNone, err
		}
		if total > 0 {
			return TierDefault, nil
		}
		return TierNone, nil
	default:
		return TierNone, nil
	}
}
