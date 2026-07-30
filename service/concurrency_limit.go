package service

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// ResolveUserMaxConcurrency 计算某用户「账户级并发上限」的实际生效值。
//
// 三态优先级（与 dto.UserSetting.MaxConcurrency 的注释一致）：
//
//	用户单独配置了 → 用该值（含显式 0 = 不限制，优先级高于全局默认）
//	未单独配置     → 用全局默认 DefaultUserMaxConcurrency
//	全局默认也是 0 → 0，即不限制
//
// 这是唯一的判定入口：中间件、令牌校验、/api/user/self 展示都走它，
// 避免多处各写一遍 ?? 逻辑导致口径不一致。
//
// 性能：纯内存计算，一次指针判空 + 一次全局字段读取，无锁无分配。
// 该函数在每个中继请求上都会被调用。
func ResolveUserMaxConcurrency(setting dto.UserSetting) int {
	if setting.MaxConcurrency != nil {
		if *setting.MaxConcurrency < 0 {
			return 0
		}
		return *setting.MaxConcurrency
	}
	limit := operation_setting.GetDefaultUserMaxConcurrency()
	if limit < 0 {
		return 0
	}
	return limit
}

// ClampTokenMaxConcurrency 把令牌级并发夹紧到账户级实际值。
//
// 静默夹紧而不报错：典型场景是用户先给令牌设了 50，管理员之后把该用户账户级调到
// 10 —— 此时令牌应按 10 生效，而不是让用户在下次编辑任何字段（哪怕只是改个名字）
// 时收到一句「不能超过 10」的报错。
//
// userLimit <= 0（账户级不限制）时，令牌级原样保留。
func ClampTokenMaxConcurrency(tokenLimit, userLimit int) int {
	if tokenLimit < 0 {
		return 0
	}
	if userLimit > 0 && tokenLimit > userLimit {
		return userLimit
	}
	return tokenLimit
}
