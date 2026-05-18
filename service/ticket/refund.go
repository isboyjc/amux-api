package ticket

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// 退款工单（category=refund）专用：方式、原因、enrichment 与所有权校验。
// 与 BugContext 同一个 metadata JSON 列共存，互不影响。

// 退款方式枚举。
const (
	RefundMethodPlatform = "platform" // 平台在线充值订单
	RefundMethodOffline  = "offline"  // 线下/对公转账等运营手工记账
)

// 退款原因枚举。Other 时必填 ReasonOther（自由文本）。
// 新增 reason 时同步前端 constants.js + locales（key 为中文源串）。
const (
	RefundReasonWrongAmount  = "wrong_amount"  // 充错金额
	RefundReasonDuplicate    = "duplicate"     // 重复充值
	RefundReasonUnused       = "unused"        // 不再使用
	RefundReasonDissatisfied = "dissatisfied"  // 服务不满意
	RefundReasonOther        = "other"         // 其他，必填补充文字
)

var refundMethodWhitelist = map[string]struct{}{
	RefundMethodPlatform: {},
	RefundMethodOffline:  {},
}

var refundReasonWhitelist = map[string]struct{}{
	RefundReasonWrongAmount:  {},
	RefundReasonDuplicate:    {},
	RefundReasonUnused:       {},
	RefundReasonDissatisfied: {},
	RefundReasonOther:        {},
}

// 关键边界。订单条数上限是软约束 —— metadata 8KB 也会兜底闸住。
const (
	maxRefundTopUps           = 10
	maxRefundReasonOtherChars = 512
)

// 退款相关错误。controller 层映射成 i18n key。
var (
	ErrRefundContextInvalid       = errors.New("invalid refund context")
	ErrRefundOtherReasonRequired  = errors.New("refund reason 'other' requires a description")
	ErrRefundOrderRequired        = errors.New("refund of platform topup requires at least one order")
	ErrRefundOrderNotFound        = errors.New("one or more refund orders not found, or not successful, or do not belong to the user")
	ErrRefundOrderTooMany         = errors.New("too many refund orders selected")
)

// IsValidRefundMethod / IsValidRefundReason 提供给 controller 兜底（DTO 层暂不引依赖）。
func IsValidRefundMethod(m string) bool {
	_, ok := refundMethodWhitelist[m]
	return ok
}

func IsValidRefundReason(r string) bool {
	_, ok := refundReasonWhitelist[r]
	return ok
}

// EnrichRefundContext 把用户填的 RefundContext 校验、去重、并从 top_ups 表
// 覆盖回填权威字段（金额 / 完成时间 / 支付方式）。任一 trade_no 不属于用户或
// 状态不是 success，整体拒绝。
//
// 安全：所有权 + 状态双重校验是核心。不暴露存在性差异 —— 不管订单是不存在、
// 属于他人、还是 status 不为 success，统一返回 ErrRefundOrderNotFound。
func EnrichRefundContext(userId int, ctx *dto.TicketRefundContext) (*dto.TicketRefundContext, error) {
	if ctx == nil {
		return nil, ErrRefundContextInvalid
	}

	method := strings.TrimSpace(ctx.Method)
	reason := strings.TrimSpace(ctx.Reason)
	if !IsValidRefundMethod(method) || !IsValidRefundReason(reason) {
		return nil, ErrRefundContextInvalid
	}
	ctx.Method = method
	ctx.Reason = reason

	// 其他原因必填补充文字；其余原因忽略 ReasonOther（避免污染展示）。
	if reason == RefundReasonOther {
		other := strings.TrimSpace(ctx.ReasonOther)
		if other == "" {
			return nil, ErrRefundOtherReasonRequired
		}
		if utf8.RuneCountInString(other) > maxRefundReasonOtherChars {
			return nil, ErrRefundOtherReasonRequired
		}
		ctx.ReasonOther = other
	} else {
		ctx.ReasonOther = ""
	}

	switch method {
	case RefundMethodOffline:
		// 线下充值：忽略用户传的 topups，避免和 platform 混用。
		ctx.TopUps = nil
		return ctx, nil

	case RefundMethodPlatform:
		// 去重 + 收集 trade_no
		seen := make(map[string]struct{}, len(ctx.TopUps))
		tradeNos := make([]string, 0, len(ctx.TopUps))
		for _, ref := range ctx.TopUps {
			tn := strings.TrimSpace(ref.TradeNo)
			if tn == "" {
				continue
			}
			if _, dup := seen[tn]; dup {
				continue
			}
			seen[tn] = struct{}{}
			tradeNos = append(tradeNos, tn)
		}
		if len(tradeNos) == 0 {
			return nil, ErrRefundOrderRequired
		}
		if len(tradeNos) > maxRefundTopUps {
			return nil, ErrRefundOrderTooMany
		}

		// 一次 IN 查询验证所有权 + 状态
		var rows []*model.TopUp
		err := model.DB.
			Where("user_id = ? AND status = ? AND trade_no IN ?",
				userId, common.TopUpStatusSuccess, tradeNos).
			Find(&rows).Error
		if err != nil {
			return nil, err
		}
		if len(rows) != len(tradeNos) {
			return nil, ErrRefundOrderNotFound
		}
		byTradeNo := make(map[string]*model.TopUp, len(rows))
		for _, r := range rows {
			byTradeNo[r.TradeNo] = r
		}

		// 用权威字段重建 TopUps（顺序按 tradeNos 去重后顺序）
		enriched := make([]dto.TicketRefundTopupRef, 0, len(tradeNos))
		for _, tn := range tradeNos {
			r, ok := byTradeNo[tn]
			if !ok {
				return nil, ErrRefundOrderNotFound
			}
			enriched = append(enriched, dto.TicketRefundTopupRef{
				TradeNo:       r.TradeNo,
				Money:         r.Money,
				Amount:        r.Amount,
				PaymentMethod: r.PaymentMethod,
				CompletedAt:   r.CompleteTime,
			})
		}
		ctx.TopUps = enriched
		return ctx, nil
	}

	return nil, ErrRefundContextInvalid
}
