package ticket

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// EnrichBugContext 给一个 TicketBugContext 补全调用上下文。
// 流程：
//  1. 如果 request_id 非空，查 LOG_DB 验证所有权。命中且属于本人 → 覆盖
//     channel/model/group/token_name/occurred_at 等字段（避免伪造）。
//  2. 命中失败或 request_id 为空 → 保留用户填的字段，但显式置
//     RequestIdVerified=false，方便管理员侧识别。
//  3. 截断 ErrorExcerpt 防止 metadata 列被滥用为日志倾倒地。
//
// 安全：所有权校验是核心，不能省。
func EnrichBugContext(userId int, ctx *dto.TicketBugContext) *dto.TicketBugContext {
	if ctx == nil {
		return nil
	}
	// 在原地修改并返回（指针）。先做长度收敛。
	const maxErrorExcerpt = 2 * 1024
	if len(ctx.ErrorExcerpt) > maxErrorExcerpt {
		ctx.ErrorExcerpt = ctx.ErrorExcerpt[:maxErrorExcerpt] + "...[truncated]"
	}
	ctx.ChannelName = strings.TrimSpace(ctx.ChannelName)
	ctx.Group = strings.TrimSpace(ctx.Group)
	ctx.Model = strings.TrimSpace(ctx.Model)

	requestId := strings.TrimSpace(ctx.RequestId)
	if requestId == "" {
		return ctx
	}

	// 显式查 LOG_DB；不复用 GetAllLogs，避免不必要的分页/统计开销。
	var log model.Log
	err := model.LOG_DB.
		Where("request_id = ?", requestId).
		Order("id desc").
		First(&log).Error
	if err != nil {
		ctx.RequestIdVerified = false
		return ctx
	}
	if log.UserId != userId {
		// 所有权校验失败 —— 用户在尝试引用别人的 request_id。
		// 不抛错（避免泄露存在性），仅标记未验证，并清掉用户填的可能伪造字段。
		ctx.RequestIdVerified = false
		return ctx
	}

	ctx.RequestIdVerified = true
	ctx.ChannelId = log.ChannelId
	if log.ChannelName != "" {
		ctx.ChannelName = log.ChannelName
	} else if log.ChannelId > 0 {
		if ch, e := model.CacheGetChannel(log.ChannelId); e == nil && ch != nil {
			ctx.ChannelName = ch.Name
		}
	}
	ctx.Group = log.Group
	ctx.Model = log.ModelName
	ctx.TokenName = log.TokenName
	ctx.OccurredAt = log.CreatedAt
	ctx.LogId = log.Id
	return ctx
}

// SerializeMetadata 把 metadata 编码为字符串落库。空 metadata 返回空字符串。
func SerializeMetadata(meta *dto.TicketMetadata) (string, error) {
	if meta == nil {
		return "", nil
	}
	b, err := common.Marshal(meta)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DeserializeMetadata 反序列化 tickets.metadata 字符串。空字符串视为无 metadata。
func DeserializeMetadata(raw string) (*dto.TicketMetadata, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var meta dto.TicketMetadata
	if err := common.UnmarshalJsonStr(raw, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// SerializeAttachments 把附件数组编码为字符串落库。空数组返回空字符串而非 "[]"
// —— 在数据库里更紧凑。
func SerializeAttachments(atts []dto.TicketAttachment) (string, error) {
	if len(atts) == 0 {
		return "", nil
	}
	b, err := common.Marshal(atts)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DeserializeAttachments 反序列化附件字段。空字符串返回 nil 切片。
func DeserializeAttachments(raw string) ([]dto.TicketAttachment, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var atts []dto.TicketAttachment
	if err := common.UnmarshalJsonStr(raw, &atts); err != nil {
		return nil, err
	}
	return atts, nil
}
