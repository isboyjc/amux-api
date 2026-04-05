package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	DesktopAuthStatusPending    = 1
	DesktopAuthStatusAuthorized = 2
	DesktopAuthStatusUsed       = 3
	DesktopAuthStatusExpired    = 4

	// Maximum number of pending sessions allowed globally to prevent abuse
	DesktopAuthMaxPendingSessions = 1000
)

type DesktopAuthSession struct {
	SessionId   string `json:"session_id" gorm:"type:varchar(64);primaryKey"`
	UserId      int    `json:"user_id"`
	AccessToken string `json:"-" gorm:"type:varchar(256)"`
	Status      int    `json:"status" gorm:"default:1;index"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint"`
	ExpiresAt   int64  `json:"expires_at" gorm:"bigint;index"`
}

func CreateDesktopAuthSession(sessionId string, expiresAt int64) error {
	// Check pending session count to prevent DB flooding
	var count int64
	DB.Model(&DesktopAuthSession{}).Where("status = ? AND expires_at > ?", DesktopAuthStatusPending, common.GetTimestamp()).Count(&count)
	if count >= DesktopAuthMaxPendingSessions {
		return gorm.ErrRecordNotFound
	}

	session := &DesktopAuthSession{
		SessionId: sessionId,
		Status:    DesktopAuthStatusPending,
		CreatedAt: common.GetTimestamp(),
		ExpiresAt: expiresAt,
	}
	return DB.Create(session).Error
}

func GetDesktopAuthSession(sessionId string) (*DesktopAuthSession, error) {
	var session DesktopAuthSession
	err := DB.Where("session_id = ?", sessionId).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func AuthorizeDesktopSession(sessionId string, userId int, accessToken string) error {
	result := DB.Model(&DesktopAuthSession{}).
		Where("session_id = ? AND status = ? AND expires_at > ?", sessionId, DesktopAuthStatusPending, common.GetTimestamp()).
		Updates(map[string]interface{}{
			"status":       DesktopAuthStatusAuthorized,
			"user_id":      userId,
			"access_token": accessToken,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ConsumeDesktopSession atomically transitions a session from authorized to used.
// It first performs the UPDATE with RowsAffected check (only one consumer wins),
// then reads the token. The access_token is cleared after read to avoid leaving
// sensitive data in the table.
func ConsumeDesktopSession(sessionId string) (*DesktopAuthSession, error) {
	// Step 1: Atomically claim the session
	result := DB.Model(&DesktopAuthSession{}).
		Where("session_id = ? AND status = ?", sessionId, DesktopAuthStatusAuthorized).
		Update("status", DesktopAuthStatusUsed)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	// Step 2: Read then clear — only the winner reaches here
	var session DesktopAuthSession
	err := DB.Where("session_id = ? AND status = ?", sessionId, DesktopAuthStatusUsed).First(&session).Error
	if err != nil {
		return nil, err
	}

	// Step 3: Clear access_token from DB — no longer needed after delivery
	DB.Model(&DesktopAuthSession{}).Where("session_id = ?", sessionId).Update("access_token", "")

	return &session, nil
}

func ExpireDesktopSession(sessionId string) error {
	// Only expire pending sessions to prevent interfering with authorized/used ones
	return DB.Model(&DesktopAuthSession{}).
		Where("session_id = ? AND status = ?", sessionId, DesktopAuthStatusPending).
		Update("status", DesktopAuthStatusExpired).Error
}

// CleanupExpiredDesktopSessions removes stale sessions from the database.
func CleanupExpiredDesktopSessions() error {
	now := common.GetTimestamp()

	// Clean expired pending sessions
	if err := DB.Where("expires_at < ? AND status = ?", now, DesktopAuthStatusPending).
		Delete(&DesktopAuthSession{}).Error; err != nil {
		return err
	}

	// Clear token and expire authorized sessions that were never consumed (> 10 min old)
	tenMinAgo := now - 600
	if err := DB.Model(&DesktopAuthSession{}).
		Where("created_at < ? AND status = ?", tenMinAgo, DesktopAuthStatusAuthorized).
		Updates(map[string]interface{}{"status": DesktopAuthStatusExpired, "access_token": ""}).Error; err != nil {
		return err
	}

	// Delete used/expired sessions older than 1 hour
	oneHourAgo := now - 3600
	return DB.Where("created_at < ? AND status IN ?", oneHourAgo, []int{DesktopAuthStatusUsed, DesktopAuthStatusExpired}).
		Delete(&DesktopAuthSession{}).Error
}
