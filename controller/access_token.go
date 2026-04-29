/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ============================================================================
// 用户端 — 自助管理 PAT
//
// 路由（router/api-router.go 注册）：
//   POST   /api/user/access_tokens
//   GET    /api/user/access_tokens
//   PATCH  /api/user/access_tokens/:id
//   DELETE /api/user/access_tokens/:id
//   POST   /api/user/access_tokens/:id/rotate
//
// 调用方：用户已登录（cookie 会话）或带有效 PAT。
// ============================================================================

type createAccessTokenReq struct {
	Name          string  `json:"name" binding:"required"`
	Description   string  `json:"description"`
	ExpiresInDays *int    `json:"expires_in_days"` // nil = 服务端套用默认值；负数 / 0 = 永不过期
	Scopes        *string `json:"scopes"`          // 暂时只接受 "full"；保留字段以便未来细化
}

// CreateUserAccessToken 用户在后台手动创建一个 PAT。
//
// 行为：
//   - Name 必填，长度限制 64
//   - ExpiresInDays 不传 → 默认 90 天；<= 0 → 永不过期
//   - 校验上限：每用户最多 UserAccessTokenMaxPerUser 个 active token
//   - 响应里**仅本次**包含 plaintext_token 字段，之后任何接口拿不到
func CreateUserAccessToken(c *gin.Context) {
	var req createAccessTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 64 {
		common.ApiErrorI18n(c, i18n.MsgUatNameRequired)
		return
	}
	if len(req.Description) > 255 {
		req.Description = req.Description[:255]
	}

	userId := c.GetInt("id")

	// 上限校验
	cnt, err := model.CountActiveUserAccessTokens(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if cnt >= int64(model.UserAccessTokenMaxPerUser) {
		common.ApiErrorI18n(c, i18n.MsgUatLimitExceeded)
		return
	}

	expiresAt := computeExpiresAt(req.ExpiresInDays)

	rec := &model.UserAccessToken{
		UserId:      userId,
		Name:        req.Name,
		Description: req.Description,
		Source:      model.UserAccessTokenSourceManual,
		Scopes:      "full",
		ExpiresAt:   expiresAt,
	}
	plaintext, err := model.CreateUserAccessToken(rec, model.UserAccessTokenPrefixPAT)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"id":              rec.Id,
		"name":            rec.Name,
		"description":     rec.Description,
		"token_prefix":    rec.TokenPrefix,
		"source":          rec.Source,
		"scopes":          rec.Scopes,
		"status":          rec.Status,
		"expires_at":      rec.ExpiresAt,
		"created_at":      rec.CreatedAt,
		"plaintext_token": plaintext,
	})
}

// ListUserAccessTokens 列我所有 token。可选 ?status=1/2/3、?source=manual/device-flow/legacy
func ListUserAccessTokens(c *gin.Context) {
	userId := c.GetInt("id")
	status := atoiOrZero(c.Query("status"))

	tokens, err := model.ListUserAccessTokensByUser(userId, status)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	source := strings.TrimSpace(c.Query("source"))
	out := make([]gin.H, 0, len(tokens))
	for _, t := range tokens {
		if source != "" && t.Source != source {
			continue
		}
		out = append(out, accessTokenToView(&t))
	}
	common.ApiSuccess(c, out)
}

type updateAccessTokenReq struct {
	Name          *string `json:"name"`
	Description   *string `json:"description"`
	ExpiresInDays *int    `json:"expires_in_days"` // nil = 不改；<=0 = 改为永不过期
}

// UpdateUserAccessToken 改 name / description / expires_at（不改 token 本体）
func UpdateUserAccessToken(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	rec := mustOwnAccessToken(c, id)
	if rec == nil {
		return
	}

	var req updateAccessTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	name := rec.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 64 {
			common.ApiErrorI18n(c, i18n.MsgUatNameRequired)
			return
		}
	}
	desc := rec.Description
	if req.Description != nil {
		desc = *req.Description
		if len(desc) > 255 {
			desc = desc[:255]
		}
	}
	var expiresAt *int64
	if req.ExpiresInDays != nil {
		expiresAt = computeExpiresAt(req.ExpiresInDays)
	} else {
		expiresAt = rec.ExpiresAt
	}

	if err := model.UpdateUserAccessTokenMeta(rec.Id, name, desc, expiresAt); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// DeleteUserAccessToken 撤销
func DeleteUserAccessToken(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	rec := mustOwnAccessToken(c, id)
	if rec == nil {
		return
	}
	if err := model.RevokeUserAccessToken(rec.Id, model.UserAccessTokenRevokeUser); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// RotateUserAccessToken 旋转：撤销旧 + 签发新（同元信息）。响应**仅本次**含 plaintext_token。
func RotateUserAccessToken(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	rec := mustOwnAccessToken(c, id)
	if rec == nil {
		return
	}
	if rec.Status != model.UserAccessTokenStatusActive {
		common.ApiErrorI18n(c, i18n.MsgUatNotActive)
		return
	}

	fresh, plaintext, err := model.RotateUserAccessToken(rec.Id)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgUatRotateFailed)
		return
	}
	resp := accessTokenToView(fresh)
	resp["plaintext_token"] = plaintext
	common.ApiSuccess(c, resp)
}

// ============================================================================
// 管理员端 — 治理面板
//
// 路由：
//   GET    /api/admin/access_tokens?user_id=&status=&source=
//   DELETE /api/admin/access_tokens/:id
// ============================================================================

// AdminListAccessTokens 管理员查询任意用户 token，支持按 user_id / status / source 筛选。
func AdminListAccessTokens(c *gin.Context) {
	userIdFilter := atoiOrZero(c.Query("user_id"))
	statusFilter := atoiOrZero(c.Query("status"))
	sourceFilter := strings.TrimSpace(c.Query("source"))

	q := model.DB.Model(&model.UserAccessToken{}).Order("created_at DESC")
	if userIdFilter > 0 {
		q = q.Where("user_id = ?", userIdFilter)
	}
	if statusFilter > 0 {
		q = q.Where("status = ?", statusFilter)
	}
	if sourceFilter != "" {
		q = q.Where("source = ?", sourceFilter)
	}

	var rows []model.UserAccessToken
	if err := q.Limit(500).Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, t := range rows {
		out = append(out, accessTokenToView(&t))
	}
	common.ApiSuccess(c, out)
}

type adminRevokeReq struct {
	Reason string `json:"reason"`
}

// AdminRevokeAccessToken 管理员强制撤销某个 token（治理滥用）。
func AdminRevokeAccessToken(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	var req adminRevokeReq
	_ = c.ShouldBindJSON(&req) // body 可空
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = model.UserAccessTokenRevokeAdmin
	}
	if err := model.RevokeUserAccessToken(id, reason); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// ============================================================================
// 兼容旧端点：保留 GET /api/user/token 的语义（生成/重置）
// ============================================================================

// GenerateAccessTokenLegacy 旧 GET /api/user/token 的内部新实现。
//
// 旧行为：直接重置 users.access_token。
// 新行为：撤销该用户所有 source=legacy 的 token，再签发一个新的 source=legacy token；
//        同步把 users.access_token 字段也写一份（保留过渡期），调用方拿到的字面值
//        可在 /api/... 调用中继续工作。
//
// 与 controller/user.go:GenerateAccessToken 的关系：保留一份本函数作为后续替换；
// 第 1 阶段我们让旧函数转调本函数。
func GenerateAccessTokenLegacy(c *gin.Context) {
	userId := c.GetInt("id")

	// 撤销现存 legacy token
	if err := model.RevokeAllUserAccessTokens(userId, model.UserAccessTokenRevokeRotate, model.UserAccessTokenSourceLegacy); err != nil {
		common.ApiError(c, err)
		return
	}

	// 旧字段是 char(32)，无前缀；为了让现有客户端继续工作，这里仍生成 32 字符无前缀字符串。
	// 写入 users.access_token（兼容路径）+ user_access_tokens（new 表，source=legacy）。
	plaintext, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgGenerateFailed)
		return
	}

	// 写入新表
	rec := &model.UserAccessToken{
		UserId:      userId,
		Name:        "Imported",
		TokenPrefix: plaintext[:model.UserAccessTokenPrefixStoreLen],
		TokenHash:   model.HashAccessToken(plaintext),
		Source:      model.UserAccessTokenSourceLegacy,
		Scopes:      "full",
		Status:      model.UserAccessTokenStatusActive,
		CreatedAt:   common.GetTimestamp(),
	}
	if err := model.DB.Create(rec).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	// 同步写 users.access_token 兼容字段（过渡期）
	if err := model.DB.Model(&model.User{}).
		Where("id = ?", userId).
		Update("access_token", plaintext).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    plaintext,
	})
}

// ============================================================================
// helpers
// ============================================================================

// computeExpiresAt 把请求的 days 翻成 *int64 时间戳：
//   - nil  → 默认 UserAccessTokenDefaultExpireDays 天后
//   - <= 0 → nil（永不过期）
//   - > 0  → 现在 + days 天
func computeExpiresAt(daysPtr *int) *int64 {
	now := common.GetTimestamp()
	days := model.UserAccessTokenDefaultExpireDays
	if daysPtr != nil {
		days = *daysPtr
	}
	if days <= 0 {
		return nil
	}
	exp := now + int64(days)*86400
	return &exp
}

func atoiOrZero(s string) int {
	if s == "" {
		return 0
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return v
}

func parseIDParam(c *gin.Context) (int, bool) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return 0, false
	}
	return id, true
}

// mustOwnAccessToken 取记录 + 校验所有权。返回 nil 表示已写入 4xx 响应。
func mustOwnAccessToken(c *gin.Context, id int) *model.UserAccessToken {
	rec, err := model.GetUserAccessTokenByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgUatNotFound)
			return nil
		}
		common.ApiError(c, err)
		return nil
	}
	myId := c.GetInt("id")
	if rec.UserId != myId {
		common.ApiErrorI18n(c, i18n.MsgForbidden)
		return nil
	}
	return rec
}

func accessTokenToView(t *model.UserAccessToken) gin.H {
	view := gin.H{
		"id":            t.Id,
		"user_id":       t.UserId,
		"name":          t.Name,
		"description":   t.Description,
		"token_prefix":  t.TokenPrefix,
		"source":        t.Source,
		"scopes":        t.Scopes,
		"status":        t.Status,
		"expires_at":    t.ExpiresAt,
		"last_used_at":  t.LastUsedAt,
		"last_used_ip":  t.LastUsedIP,
		"created_at":    t.CreatedAt,
		"revoked_at":    t.RevokedAt,
		"revoke_reason": t.RevokeReason,
	}

	// device-flow 来源的 token：解析 source_meta，关联 oauth_clients 拿应用元信息；
	// 只外露 client_app（含 logo / 名字 / 认证标）和 authorized_ip，
	// session_id / user_agent 等审计信息不下发到前端，避免泄露。
	if t.Source == model.UserAccessTokenSourceDeviceFlow && t.SourceMeta != "" {
		var meta map[string]any
		if err := common.UnmarshalJsonStr(t.SourceMeta, &meta); err == nil {
			if ipVal, ok := meta["ip"].(string); ok && ipVal != "" {
				view["authorized_ip"] = ipVal
			}
			if cidVal, ok := meta["client_id"].(string); ok && cidVal != "" {
				if rec, err := model.GetOAuthClientByClientID(cidVal); err == nil && rec != nil {
					view["client_app"] = gin.H{
						"client_id": rec.ClientId,
						"name":      rec.Name,
						"logo_url":  rec.LogoURL,
						"verified":  rec.Verified,
					}
				} else {
					// 应用记录被硬删的边角场景：至少把 client_id 透回，前端能渲染
					view["client_app"] = gin.H{"client_id": cidVal, "name": cidVal}
				}
			}
		}
	}

	return view
}
