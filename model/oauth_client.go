/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// OAuthClient 第三方应用注册记录。仅管理员可 CRUD（router 层已用 AdminAuth）。
//   - client_id：公开标识，外部应用调 OAuth Device Flow 时携带
//   - client_secret：当前 Device Flow 不强制，但保留字段以便未来 Authorization
//     Code Flow / Confidential Client 场景启用；secret 不直接返回，仅创建/轮换
//     时一次性返回明文，DB 存储 SHA256 哈希
//   - status：1 active / 2 disabled。禁用后该 client_id 下所有 device flow
//     创建被拒，已签发的 OAT 仍能用——撤销由管理员单独触发
const (
	OAuthClientStatusActive   = 1
	OAuthClientStatusDisabled = 2

	// 内置 client：未显式传 client_id 的 device flow 请求都会兜底到此条记录。
	// 启动时由 EnsureBuiltinOAuthClient 幂等创建；旧安装中存在 client_id 为
	// "legacy-desktop" 的旧记录，启动时会一次性改名为 OAuthClientAmuxDesktopId。
	OAuthClientAmuxDesktopId = "amux-desktop"

	// 旧 client_id；仅在 EnsureBuiltinOAuthClient 升级路径上引用一次。
	oauthClientLegacyDesktopIdDeprecated = "legacy-desktop"
)

type OAuthClient struct {
	Id             int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ClientId       string `json:"client_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	ClientSecretHash string `json:"-" gorm:"type:varchar(64)"`
	Name           string `json:"name" gorm:"type:varchar(128);not null"`
	Description    string `json:"description" gorm:"type:text"`
	LogoURL        string `json:"logo_url" gorm:"type:varchar(500)"`
	HomepageURL    string `json:"homepage_url" gorm:"type:varchar(500)"`
	ContactEmail   string `json:"contact_email" gorm:"type:varchar(128)"`
	AllowedScopes  string `json:"allowed_scopes" gorm:"type:varchar(255);default:'full'"`
	Verified       bool   `json:"verified" gorm:"default:false"`
	Status         int    `json:"status" gorm:"type:int;default:1;index"`
	CreatedBy      int    `json:"created_by" gorm:"not null"`
	CreatedAt      int64  `json:"created_at" gorm:"not null"`
	UpdatedAt      int64  `json:"updated_at" gorm:"not null"`
}

func (OAuthClient) TableName() string {
	return "oauth_clients"
}

// IsActive 判断 client 当前是否可用于授权（不仅看 status，也避免空 client_id）。
func (c *OAuthClient) IsActive() bool {
	return c != nil && c.ClientId != "" && c.Status == OAuthClientStatusActive
}

// ============= CRUD =============

func GetOAuthClientByClientID(clientId string) (*OAuthClient, error) {
	clientId = strings.TrimSpace(clientId)
	if clientId == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var c OAuthClient
	if err := DB.Where("client_id = ?", clientId).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func GetOAuthClientByID(id int) (*OAuthClient, error) {
	var c OAuthClient
	if err := DB.Where("id = ?", id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func ListOAuthClients(status int) ([]OAuthClient, error) {
	var out []OAuthClient
	q := DB.Order("created_at DESC")
	if status > 0 {
		q = q.Where("status = ?", status)
	}
	err := q.Find(&out).Error
	return out, err
}

// CreateOAuthClient 创建一条记录。返回 client_secret 明文（**仅本次**返回）。
// 调用方必须填 Name；ClientId 留空时自动生成。
func CreateOAuthClient(c *OAuthClient) (clientSecret string, err error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return "", errors.New("name is required")
	}
	if strings.TrimSpace(c.ClientId) == "" {
		// 用 name 生成可读 client_id
		c.ClientId = generateClientId(c.Name)
	}
	c.ClientId = strings.TrimSpace(c.ClientId)
	if len(c.ClientId) > 64 {
		c.ClientId = c.ClientId[:64]
	}
	if c.AllowedScopes == "" {
		c.AllowedScopes = "full"
	}
	if c.Status == 0 {
		c.Status = OAuthClientStatusActive
	}

	// 生成 client_secret（明文仅返回一次；DB 存 SHA256）
	secret, err := common.GenerateRandomCharsKey(40)
	if err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	c.ClientSecretHash = HashAccessToken(secret)
	c.CreatedAt = common.GetTimestamp()
	c.UpdatedAt = c.CreatedAt
	if err := DB.Create(c).Error; err != nil {
		return "", err
	}
	return secret, nil
}

// UpdateOAuthClient 仅允许更新可见元信息字段（不改 client_id / secret）。
func UpdateOAuthClient(id int, updates map[string]interface{}) error {
	allowed := map[string]struct{}{
		"name":           {},
		"description":    {},
		"logo_url":       {},
		"homepage_url":   {},
		"contact_email":  {},
		"allowed_scopes": {},
		"verified":       {},
		"status":         {},
	}
	patch := map[string]interface{}{}
	for k, v := range updates {
		if _, ok := allowed[k]; ok {
			patch[k] = v
		}
	}
	if len(patch) == 0 {
		return nil
	}
	patch["updated_at"] = common.GetTimestamp()
	return DB.Model(&OAuthClient{}).Where("id = ?", id).Updates(patch).Error
}

// RotateOAuthClientSecret 返回新的明文 secret（旧 secret 失效）。
func RotateOAuthClientSecret(id int) (string, error) {
	secret, err := common.GenerateRandomCharsKey(40)
	if err != nil {
		return "", err
	}
	if err := DB.Model(&OAuthClient{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"client_secret_hash": HashAccessToken(secret),
			"updated_at":         common.GetTimestamp(),
		}).Error; err != nil {
		return "", err
	}
	return secret, nil
}

// DisableOAuthClient 把 client 标 disabled。已签发的 OAT 不在此撤销，
// 由管理员单独操作（避免误操作连带撤销大量 token）。
func DisableOAuthClient(id int) error {
	return DB.Model(&OAuthClient{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     OAuthClientStatusDisabled,
			"updated_at": common.GetTimestamp(),
		}).Error
}

// DeleteOAuthClient 物理删除。慎用——建议优先用 DisableOAuthClient 保留历史 grant 关联。
func DeleteOAuthClient(id int) error {
	return DB.Where("id = ?", id).Delete(&OAuthClient{}).Error
}

// ============= 启动时预置内置 client =============

// EnsureBuiltinOAuthClient 幂等保证内置 OAuth Client 存在，给未显式传 client_id
// 的 device flow 兜底。由 model.migrateDB 在 AutoMigrate 之后调用。
//
// 升级路径：旧安装中这条记录的 client_id 是 "legacy-desktop"，启动时会一次性
// 改名为 OAuthClientAmuxDesktopId（"amux-desktop"），同时把仍为默认值的
// name/description/logo_url 升级到新版文案。管理员手动改过的字段一概不动。
func EnsureBuiltinOAuthClient() error {
	const (
		builtinName        = "Amux Desktop Client"
		builtinLogoURL     = "https://cdn.amux.ai/logo/icon.png"
		builtinDescription = "Amux 桌面客户端使用的内置 OAuth 应用，也是未显式传 client_id 的请求的兜底配置。"
		oldDefaultName     = "Desktop Client"
		oldDefaultDesc     = "Built-in client for legacy desktop authorization flow."
	)

	// Step 1: 老安装可能有 client_id="legacy-desktop" 的旧记录，先按旧 ID 查到再改名。
	existing, err := GetOAuthClientByClientID(OAuthClientAmuxDesktopId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if existing == nil {
		legacyRow, lerr := GetOAuthClientByClientID(oauthClientLegacyDesktopIdDeprecated)
		if lerr != nil && !errors.Is(lerr, gorm.ErrRecordNotFound) {
			return lerr
		}
		if legacyRow != nil {
			// 把 client_id 字面值改成新值；其他字段在 step 2 里统一升级
			if rerr := DB.Model(legacyRow).
				Updates(map[string]interface{}{
					"client_id":  OAuthClientAmuxDesktopId,
					"updated_at": common.GetTimestamp(),
				}).Error; rerr != nil {
				return rerr
			}
			existing = legacyRow
			existing.ClientId = OAuthClientAmuxDesktopId
		}
	}

	// Step 2: 找到记录后，把仍是默认值的字段升级到新版文案；管理员改过的不动。
	if existing != nil {
		updates := map[string]interface{}{}
		if existing.Name == oldDefaultName {
			updates["name"] = builtinName
		}
		if existing.Description == "" || existing.Description == oldDefaultDesc {
			updates["description"] = builtinDescription
		}
		if existing.LogoURL == "" {
			updates["logo_url"] = builtinLogoURL
		}
		if len(updates) == 0 {
			return nil
		}
		updates["updated_at"] = common.GetTimestamp()
		return DB.Model(existing).Updates(updates).Error
	}

	// Step 3: 全新安装：创建一条新记录
	now := common.GetTimestamp()
	c := &OAuthClient{
		ClientId:      OAuthClientAmuxDesktopId,
		Name:          builtinName,
		Description:   builtinDescription,
		LogoURL:       builtinLogoURL,
		AllowedScopes: "full",
		Verified:      true,
		Status:        OAuthClientStatusActive,
		CreatedBy:     0, // 系统创建
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	// 内置 client 不需要 secret（device flow 不要求）；置空字符串
	c.ClientSecretHash = ""
	return DB.Create(c).Error
}

// generateClientId 把名字转成形如 "notion-ai-3jK9pQ" 的可读公开标识。
func generateClientId(name string) string {
	// 取前 24 个 ASCII 可打印字符，去空格转小写连字符
	clean := make([]rune, 0, 24)
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			clean = append(clean, r)
		} else if r == ' ' || r == '-' || r == '_' {
			if len(clean) > 0 && clean[len(clean)-1] != '-' {
				clean = append(clean, '-')
			}
		}
		if len(clean) >= 24 {
			break
		}
	}
	prefix := strings.Trim(string(clean), "-")
	if prefix == "" {
		prefix = "app"
	}
	suffix, _ := common.GenerateRandomCharsKey(6)
	return fmt.Sprintf("%s-%s", prefix, strings.ToLower(suffix))
}
