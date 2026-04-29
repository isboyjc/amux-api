/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

package model

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// ========= 常量 / 形态 =========

const (
	// 前缀。两种区分仅作审计/管理筛选，校验路径完全一致。
	UserAccessTokenPrefixPAT = "amux_api_pat_" // 用户手工创建的 Personal Access Token
	UserAccessTokenPrefixOAT = "amux_api_oat_" // OAuth Device Flow 签发的 Token

	// 主体随机段长度（base62），熵 ≈ 40 * log2(62) ≈ 238 bits
	UserAccessTokenRandomLen = 40

	// 持久化的 prefix 段长度：完整前缀 + 主体前 3 字符
	// （PAT 13 + 3 = 16；OAT 同长）
	UserAccessTokenPrefixStoreLen = 16

	// 校验时的最小完整 token 长度。比 PrefixStoreLen 多 10：保证主体随机段
	// 至少有 10 个字符可参与 SHA256 比对，挡掉用 "amux_api_pat_" 这类纯前缀串
	// 也吃 DB 查询的探测攻击。生成路径用的随机段长是 UserAccessTokenRandomLen=40。
	UserAccessTokenMinValidLen = UserAccessTokenPrefixStoreLen + 10

	// 状态
	UserAccessTokenStatusActive  = 1
	UserAccessTokenStatusRevoked = 2
	UserAccessTokenStatusExpired = 3

	// 来源
	UserAccessTokenSourceManual     = "manual"      // 用户手动创建
	UserAccessTokenSourceDeviceFlow = "device-flow" // OAuth Device Flow 自动签发
	UserAccessTokenSourceLegacy     = "legacy"      // 从 users.access_token 一次性迁移过来
	UserAccessTokenSourceAdmin      = "admin"       // 管理员代为创建（如客户支持场景）

	// 撤销原因
	UserAccessTokenRevokeUser         = "user"          // 用户主动撤销
	UserAccessTokenRevokeRotate       = "rotate"        // 旋转替换
	UserAccessTokenRevokeIdleExpired  = "idle-expired"  // 空闲过期
	UserAccessTokenRevokeAdmin        = "admin"         // 管理员强制撤销
	UserAccessTokenRevokePasswordReset = "password-reset" // 重置密码自动撤销

	// 默认空闲过期阈值（90 天，对齐 GitHub PAT 默认）
	UserAccessTokenIdleExpireSeconds = int64(90 * 24 * 3600)

	// last_used_at 节流：DB 当前值距 now < 此值则不更新，省 UPDATE
	UserAccessTokenLastUsedThrottleSec = int64(60)

	// 每用户 active token 上限
	UserAccessTokenMaxPerUser = 1000

	// 默认过期时间（用户创建时未指定且未选「永不」时）
	UserAccessTokenDefaultExpireDays = 90
)

// ========= 结构体 =========

// UserAccessToken 是新版统一的 access token 表。
// 替代旧 users.access_token 单字段（旧字段保留只读，过渡期后删除）。
type UserAccessToken struct {
	Id            int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId        int    `json:"user_id" gorm:"not null;index:idx_uat_user_status,priority:1"`
	Name          string `json:"name" gorm:"type:varchar(64);not null"`
	Description   string `json:"description" gorm:"type:varchar(255)"`
	TokenPrefix   string `json:"token_prefix" gorm:"type:varchar(32);not null;index:idx_uat_prefix_status,priority:1"`
	TokenHash     string `json:"-" gorm:"type:varchar(64);not null"`
	Source        string `json:"source" gorm:"type:varchar(32);not null;default:'manual'"`
	SourceMeta    string `json:"source_meta,omitempty" gorm:"type:text"`
	Scopes        string `json:"scopes" gorm:"type:varchar(255);default:'full'"`
	Status        int    `json:"status" gorm:"type:int;default:1;index:idx_uat_user_status,priority:2;index:idx_uat_prefix_status,priority:2;index:idx_uat_status_expires,priority:1"`
	ExpiresAt     *int64 `json:"expires_at" gorm:"index:idx_uat_status_expires,priority:2"`
	LastUsedAt    *int64 `json:"last_used_at"`
	LastUsedIP    string `json:"last_used_ip" gorm:"type:varchar(64)"`
	CreatedAt     int64  `json:"created_at" gorm:"not null"`
	RevokedAt     *int64 `json:"revoked_at"`
	RevokeReason  string `json:"revoke_reason,omitempty" gorm:"type:varchar(64)"`
}

func (UserAccessToken) TableName() string {
	return "user_access_tokens"
}

// IsActive 综合 status + expires_at 判断当前是否真有效（不更新 DB，只读）。
func (t *UserAccessToken) IsActive(now int64) bool {
	if t.Status != UserAccessTokenStatusActive {
		return false
	}
	if t.ExpiresAt != nil && *t.ExpiresAt > 0 && *t.ExpiresAt <= now {
		return false
	}
	return true
}

// ========= Token 生成 / 哈希 =========

const base62Charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// GenerateAccessTokenString 生成一个完整 plaintext token、其存储 prefix 段、其 SHA256 hex。
//   - prefix: UserAccessTokenPrefixPAT / UserAccessTokenPrefixOAT
//   - plaintext: 完整 token，仅创建瞬间返回给调用方，不入库
//   - storedPrefix: 入库 token_prefix 字段（用于查询索引）
//   - hash: 入库 token_hash 字段（SHA256 hex）
func GenerateAccessTokenString(prefix string) (plaintext, storedPrefix, hash string, err error) {
	body, err := randomBase62(UserAccessTokenRandomLen)
	if err != nil {
		return "", "", "", err
	}
	plaintext = prefix + body
	storedPrefix = plaintext[:UserAccessTokenPrefixStoreLen]
	hash = HashAccessToken(plaintext)
	return plaintext, storedPrefix, hash, nil
}

// HashAccessToken 计算 token 的 SHA256 hex。校验路径直接用同样的算法再算一次比对。
// 用 SHA256 而不是 BCrypt：token 本身已是 238 bits 高熵随机串，无需慢哈希；
// middleware 在请求热路径上每次都跑慢哈希成本太高。
func HashAccessToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func randomBase62(n int) (string, error) {
	out := make([]byte, n)
	// 一次取够（拒绝采样：每个字节 < 62*4=248 时按 mod 62 用，否则丢弃）
	// 期望需求字节 ≈ n * 256/248，多取 25% 余量。
	need := n + n/4 + 1
	buf := make([]byte, need)
	idx := 0
	for idx < n {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("crypto rand: %w", err)
		}
		for _, b := range buf {
			if int(b) < 248 {
				out[idx] = base62Charset[int(b)%62]
				idx++
				if idx >= n {
					break
				}
			}
		}
	}
	return string(out), nil
}

// ConstantTimeMatchHash 用 constant-time 比对，防 timing attack。
func ConstantTimeMatchHash(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ========= 校验路径（middleware 调） =========

// ValidateUserAccessToken 是新版统一校验入口：
//   - 1) 前缀分流：amux_api_pat_ / amux_api_oat_ 走新表；其它（含旧 32 字符）走旧字段兜底
//   - 2) SHA256 + DB 双索引查 (token_prefix, status=1)
//   - 3) 内存中 constant-time 比对 token_hash（防 timing attack）
//   - 4) 校验 expires_at
//   - 5) 异步节流更新 last_used_at + last_used_ip
//
// 返回 (user, tokenRecord, err)。tokenRecord 为新表记录或 nil（走的是 legacy 兜底）。
func ValidateUserAccessToken(plaintext, clientIP string) (*User, *UserAccessToken, error) {
	if plaintext == "" {
		return nil, nil, nil
	}
	plaintext = strings.TrimPrefix(plaintext, "Bearer ")
	plaintext = strings.TrimSpace(plaintext)

	// 旧 32 字符 token：走兜底（中间件层会调旧 ValidateAccessToken）。
	if !strings.HasPrefix(plaintext, UserAccessTokenPrefixPAT) &&
		!strings.HasPrefix(plaintext, UserAccessTokenPrefixOAT) {
		return nil, nil, nil
	}

	// 长度门禁，防止短 token 也吃 DB
	if len(plaintext) < UserAccessTokenMinValidLen {
		return nil, nil, nil
	}
	storedPrefix := plaintext[:UserAccessTokenPrefixStoreLen]

	var candidates []UserAccessToken
	if err := DB.Where("token_prefix = ? AND status = ?", storedPrefix, UserAccessTokenStatusActive).
		Find(&candidates).Error; err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	if len(candidates) == 0 {
		return nil, nil, nil
	}

	hash := HashAccessToken(plaintext)
	now := common.GetTimestamp()
	var matched *UserAccessToken
	for i := range candidates {
		// constant-time 防 timing attack（即便 candidates 多于 1 也对每条比一遍）
		if ConstantTimeMatchHash(candidates[i].TokenHash, hash) && candidates[i].IsActive(now) {
			matched = &candidates[i]
			// 不 break：继续比剩余项以维持时间常数（影响极小，几条 hash 比较）
		}
	}
	if matched == nil {
		return nil, nil, nil
	}

	// 加载用户
	user := &User{}
	if err := DB.Where("id = ?", matched.UserId).First(user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}

	// 异步节流更新 last_used_at
	touchLastUsedAsync(matched.Id, matched.LastUsedAt, clientIP, now)

	return user, matched, nil
}

// touchLastUsedAsync 节流：与上次差 < 60s 不更新；用 gopool 派发避免阻塞请求。
func touchLastUsedAsync(id int, prev *int64, ip string, now int64) {
	if prev != nil && now-*prev < UserAccessTokenLastUsedThrottleSec {
		return
	}
	go func() {
		updates := map[string]interface{}{"last_used_at": now}
		if ip != "" {
			updates["last_used_ip"] = ip
		}
		DB.Model(&UserAccessToken{}).Where("id = ?", id).Updates(updates)
	}()
}

// ========= CRUD =========

// CountActiveUserAccessTokens 计数某用户有效 token 数（用于上限判断）。
func CountActiveUserAccessTokens(userId int) (int64, error) {
	var n int64
	err := DB.Model(&UserAccessToken{}).
		Where("user_id = ? AND status = ?", userId, UserAccessTokenStatusActive).
		Count(&n).Error
	return n, err
}

// CreateUserAccessToken 创建并返回 (plaintext, record)。plaintext 仅本次返回。
//
// 调用方填充：UserId / Name / Description / Source / SourceMeta / Scopes / ExpiresAt
// 内部自动填充：TokenPrefix / TokenHash / Status=Active / CreatedAt
func CreateUserAccessToken(t *UserAccessToken, prefix string) (plaintext string, err error) {
	if t.Source == "" {
		t.Source = UserAccessTokenSourceManual
	}
	if t.Scopes == "" {
		t.Scopes = "full"
	}
	if prefix == "" {
		prefix = UserAccessTokenPrefixPAT
	}
	plaintext, storedPrefix, hash, err := GenerateAccessTokenString(prefix)
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	t.TokenPrefix = storedPrefix
	t.TokenHash = hash
	t.Status = UserAccessTokenStatusActive
	t.CreatedAt = common.GetTimestamp()
	if err := DB.Create(t).Error; err != nil {
		return "", fmt.Errorf("create access token: %w", err)
	}
	return plaintext, nil
}

// ListUserAccessTokensByUser 列某用户的 token（按状态可选筛选）。
// status=0 表示不筛。
func ListUserAccessTokensByUser(userId, status int) ([]UserAccessToken, error) {
	var out []UserAccessToken
	q := DB.Where("user_id = ?", userId).Order("created_at DESC")
	if status > 0 {
		q = q.Where("status = ?", status)
	}
	err := q.Find(&out).Error
	return out, err
}

// GetUserAccessTokenByID 取单条（用于 update / revoke / rotate 前的所有权校验）。
func GetUserAccessTokenByID(id int) (*UserAccessToken, error) {
	var t UserAccessToken
	if err := DB.Where("id = ?", id).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateUserAccessTokenMeta 仅允许更新 name / description / expires_at（不改 token 本体）。
func UpdateUserAccessTokenMeta(id int, name, description string, expiresAt *int64) error {
	updates := map[string]interface{}{}
	if name != "" {
		updates["name"] = name
	}
	updates["description"] = description
	updates["expires_at"] = expiresAt // 允许 nil 显式设为永不过期
	if len(updates) == 0 {
		return nil
	}
	return DB.Model(&UserAccessToken{}).Where("id = ?", id).Updates(updates).Error
}

// RevokeUserAccessToken 标记单个 token 为 revoked。
// 允许从 active / expired 转入；已 revoked 的不再变更（保留首次撤销时间）。
func RevokeUserAccessToken(id int, reason string) error {
	now := common.GetTimestamp()
	return DB.Model(&UserAccessToken{}).
		Where("id = ? AND status IN ?", id, []int{
			UserAccessTokenStatusActive,
			UserAccessTokenStatusExpired,
		}).
		Updates(map[string]interface{}{
			"status":        UserAccessTokenStatusRevoked,
			"revoked_at":    now,
			"revoke_reason": reason,
		}).Error
}

// RevokeAllUserAccessTokens 把某用户所有 active token 标 revoked。
// 用于密码重置 / 管理员强制下线场景。可选按 source 筛选（如只撤 legacy）。
func RevokeAllUserAccessTokens(userId int, reason string, sourceFilter string) error {
	now := common.GetTimestamp()
	q := DB.Model(&UserAccessToken{}).
		Where("user_id = ? AND status = ?", userId, UserAccessTokenStatusActive)
	if sourceFilter != "" {
		q = q.Where("source = ?", sourceFilter)
	}
	return q.Updates(map[string]interface{}{
		"status":        UserAccessTokenStatusRevoked,
		"revoked_at":    now,
		"revoke_reason": reason,
	}).Error
}

// RotateUserAccessToken 撤销旧 + 新建新（保留 name/description/scopes/expires_at/source/source_meta）。
// 返回新记录 + plaintext。
func RotateUserAccessToken(id int) (*UserAccessToken, string, error) {
	old, err := GetUserAccessTokenByID(id)
	if err != nil {
		return nil, "", err
	}
	if old.Status != UserAccessTokenStatusActive {
		return nil, "", fmt.Errorf("token not active")
	}
	prefix := UserAccessTokenPrefixPAT
	if strings.HasPrefix(old.TokenPrefix, UserAccessTokenPrefixOAT) {
		prefix = UserAccessTokenPrefixOAT
	}
	now := common.GetTimestamp()

	var plaintext string
	err = DB.Transaction(func(tx *gorm.DB) error {
		// 撤销旧
		if err := tx.Model(&UserAccessToken{}).
			Where("id = ? AND status = ?", old.Id, UserAccessTokenStatusActive).
			Updates(map[string]interface{}{
				"status":        UserAccessTokenStatusRevoked,
				"revoked_at":    now,
				"revoke_reason": UserAccessTokenRevokeRotate,
			}).Error; err != nil {
			return err
		}
		// 新建新
		fresh := &UserAccessToken{
			UserId:      old.UserId,
			Name:        old.Name,
			Description: old.Description,
			Source:      old.Source,
			SourceMeta:  old.SourceMeta,
			Scopes:      old.Scopes,
			ExpiresAt:   old.ExpiresAt,
		}
		pt, _, hash, gErr := GenerateAccessTokenString(prefix)
		if gErr != nil {
			return gErr
		}
		fresh.TokenPrefix = pt[:UserAccessTokenPrefixStoreLen]
		fresh.TokenHash = hash
		fresh.Status = UserAccessTokenStatusActive
		fresh.CreatedAt = now
		if err := tx.Create(fresh).Error; err != nil {
			return err
		}
		plaintext = pt
		old = fresh
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return old, plaintext, nil
}

// ========= 定时清理 =========

// CleanupExpiredUserAccessTokens 由调度器定期调（建议每天 1 次）：
//  1. 把硬过期（expires_at <= now）但仍 active 的标 expired
//  2. 把空闲超过 90 天（last_used_at < now - 90d 且 created_at < now - 90d）的标 expired
func CleanupExpiredUserAccessTokens() error {
	now := common.GetTimestamp()
	idleCutoff := now - UserAccessTokenIdleExpireSeconds

	// 硬过期
	if err := DB.Model(&UserAccessToken{}).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at > 0 AND expires_at <= ?",
			UserAccessTokenStatusActive, now).
		Updates(map[string]interface{}{
			"status":        UserAccessTokenStatusExpired,
			"revoked_at":    now,
			"revoke_reason": UserAccessTokenRevokeIdleExpired,
		}).Error; err != nil {
		return err
	}

	// 空闲过期：last_used_at < cutoff（或从未使用且 created_at < cutoff）
	// 三 DB 兼容写法：用两个独立 UPDATE 避免 OR 子句和不同 NULL 语义。
	if err := DB.Model(&UserAccessToken{}).
		Where("status = ? AND last_used_at IS NOT NULL AND last_used_at < ?",
			UserAccessTokenStatusActive, idleCutoff).
		Updates(map[string]interface{}{
			"status":        UserAccessTokenStatusExpired,
			"revoked_at":    now,
			"revoke_reason": UserAccessTokenRevokeIdleExpired,
		}).Error; err != nil {
		return err
	}
	if err := DB.Model(&UserAccessToken{}).
		Where("status = ? AND last_used_at IS NULL AND created_at < ?",
			UserAccessTokenStatusActive, idleCutoff).
		Updates(map[string]interface{}{
			"status":        UserAccessTokenStatusExpired,
			"revoked_at":    now,
			"revoke_reason": UserAccessTokenRevokeIdleExpired,
		}).Error; err != nil {
		return err
	}
	return nil
}

// ========= 一次性数据迁移 =========
//
// 把 users.access_token（旧 32 字符）一次性复制进新表 source=legacy。
// 字面值不变，调用方原有 token 仍可继续工作。

var migrateLegacyOnce sync.Once

func MigrateLegacyAccessTokens() error {
	var migrateErr error
	migrateLegacyOnce.Do(func() {
		migrateErr = migrateLegacyAccessTokensInner()
	})
	return migrateErr
}

func migrateLegacyAccessTokensInner() error {
	// 简单分页扫 users.access_token != ''
	const pageSize = 500
	var lastId int
	now := common.GetTimestamp()
	for {
		var users []User
		err := DB.Select("id, access_token").
			Where("id > ? AND access_token IS NOT NULL AND access_token != ''", lastId).
			Order("id ASC").
			Limit(pageSize).
			Find(&users).Error
		if err != nil {
			return err
		}
		if len(users) == 0 {
			break
		}
		for _, u := range users {
			lastId = u.Id
			if u.AccessToken == nil || *u.AccessToken == "" {
				continue
			}
			plaintext := *u.AccessToken
			storedPrefix := plaintext
			if len(plaintext) > UserAccessTokenPrefixStoreLen {
				storedPrefix = plaintext[:UserAccessTokenPrefixStoreLen]
			}
			hash := HashAccessToken(plaintext)

			// 如果已经迁移过则跳过（按 user_id + source=legacy + token_hash 唯一性）
			var existing int64
			if err := DB.Model(&UserAccessToken{}).
				Where("user_id = ? AND source = ? AND token_hash = ?",
					u.Id, UserAccessTokenSourceLegacy, hash).
				Count(&existing).Error; err != nil {
				return err
			}
			if existing > 0 {
				continue
			}

			rec := &UserAccessToken{
				UserId:      u.Id,
				Name:        "Imported",
				TokenPrefix: storedPrefix,
				TokenHash:   hash,
				Source:      UserAccessTokenSourceLegacy,
				Scopes:      "full",
				Status:      UserAccessTokenStatusActive,
				CreatedAt:   now,
			}
			if err := DB.Create(rec).Error; err != nil {
				return err
			}
		}
		if len(users) < pageSize {
			break
		}
	}
	return nil
}

// ========= 调度器外壳 =========

// StartUserAccessTokenJanitor 启动定时清理（每 24h 触发一次）。
// 由 main.go 在启动后调用，进程退出时自然结束。
func StartUserAccessTokenJanitor() {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		// 启动时先跑一次，避免长时间不重启的实例堆积
		_ = CleanupExpiredUserAccessTokens()
		for range ticker.C {
			if err := CleanupExpiredUserAccessTokens(); err != nil {
				common.SysLog("CleanupExpiredUserAccessTokens error: " + err.Error())
			}
		}
	}()
}
