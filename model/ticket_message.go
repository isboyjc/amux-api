package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// TicketMessage 单条工单消息。一旦写入不再修改（v1 全员不可编辑/删除），
// 详情页通过 (ticket_id, created_at) 复合索引按时间顺序拉取。
type TicketMessage struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	TicketId    int    `json:"ticket_id" gorm:"index:idx_tm_ticket_created,priority:1;not null"`
	SenderId    int    `json:"sender_id" gorm:"index;not null"`            // 0 = 系统消息
	SenderRole  int    `json:"sender_role" gorm:"not null"`                // 0 user / 1 admin / 2 system
	Content     string `json:"content" gorm:"type:text;not null"`          // markdown，前端渲染前必须 sanitize
	Attachments string `json:"attachments" gorm:"type:text"`               // JSON []TicketAttachment
	IsInternal  bool   `json:"is_internal" gorm:"not null;default:false"`  // v2 启用，v1 永远 false
	CreatedAt   int64  `json:"created_at" gorm:"bigint;index:idx_tm_ticket_created,priority:2"`
}

func (TicketMessage) TableName() string { return "ticket_messages" }

// CreateTicketMessage 在传入事务内插入消息。tx 为 nil 时使用 DB 全局连接。
func CreateTicketMessage(tx *gorm.DB, m *TicketMessage) error {
	if m.TicketId <= 0 {
		return errors.New("invalid ticket_id")
	}
	if m.CreatedAt == 0 {
		m.CreatedAt = common.GetTimestamp()
	}
	if tx == nil {
		tx = DB
	}
	return tx.Create(m).Error
}

// ListTicketMessages 拉取一张工单的消息流。
// 用户视角调用方需要在上层过滤掉 IsInternal=true 的条目。
// limit<=0 时不分页（小工单可一次性拉完）。
func ListTicketMessages(ticketId int, limit, offset int) ([]*TicketMessage, error) {
	if ticketId <= 0 {
		return nil, errors.New("invalid ticket_id")
	}
	tx := DB.Where("ticket_id = ?", ticketId).Order("created_at ASC, id ASC")
	if limit > 0 {
		tx = tx.Limit(limit).Offset(offset)
	}
	var list []*TicketMessage
	err := tx.Find(&list).Error
	return list, err
}

// CountTicketMessages 用于详情页"是否还有更多"判断。
func CountTicketMessages(ticketId int) (int64, error) {
	var n int64
	err := DB.Model(&TicketMessage{}).Where("ticket_id = ?", ticketId).Count(&n).Error
	return n, err
}

// CountUserRepliesSince 用于回复频次限流。
func CountUserRepliesSince(userId int, since int64) (int64, error) {
	var n int64
	err := DB.Model(&TicketMessage{}).
		Where("sender_id = ? AND sender_role = ? AND created_at >= ?",
			userId, TicketSenderRoleUser, since).
		Count(&n).Error
	return n, err
}
