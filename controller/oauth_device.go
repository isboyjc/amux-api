/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

package controller

import (
	"regexp"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// =============================================================================
// OAuth Device Authorization Grant（简化版，参考 RFC 8628）
//
// 路由（router/api-router.go 注册）：
//   POST /api/oauth/device/authorize   公开，外部应用创建 session
//   GET  /api/oauth/device/info        公开，授权页查询 session 状态 + client 元信息
//   POST /api/oauth/device/confirm     UserAuth，用户在浏览器同意/拒绝
//   GET  /api/oauth/device/check       公开，外部应用轮询拿 token
//
// client_id 处理：
//   - 调用方未传 client_id 时兜底到内置 "amux-desktop"
//   - 已传则必须能在 oauth_clients 表里找到 active 记录，否则拒绝
// =============================================================================

// 内置 client_id：未显式传 client_id 的请求都会落到这条记录上；外部应用应在
// admin 后台注册并显式传自己的 client_id。
const builtinClientID = "amux-desktop"

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isValidUUID(s string) bool {
	return uuidRegex.MatchString(s)
}

type oauthDeviceAuthorizeReq struct {
	SessionId string `json:"session_id" binding:"required"`
	ClientId  string `json:"client_id"` // 可选；未传时兜底到 amux-desktop
}

type oauthDeviceConfirmReq struct {
	SessionId string `json:"session_id" binding:"required"`
	Action    string `json:"action" binding:"required"`
}

// OAuthDeviceAuthorize 创建 device session（外部应用调用）
//
// 请求：{ "session_id": "<UUID v4>", "client_id": "<your-app>" }
func OAuthDeviceAuthorize(c *gin.Context) {
	var req oauthDeviceAuthorizeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if !isValidUUID(req.SessionId) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	clientId := strings.TrimSpace(req.ClientId)
	if clientId == "" {
		clientId = builtinClientID
	}
	clientRec, err := model.GetOAuthClientByClientID(clientId)
	if err != nil || !clientRec.IsActive() {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	if existing, _ := model.GetDesktopAuthSession(req.SessionId); existing != nil {
		common.ApiErrorI18n(c, i18n.MsgDesktopAuthSessionExists)
		return
	}
	expiresAt := common.GetTimestamp() + 5*60
	if err := model.CreateDesktopAuthSession(req.SessionId, expiresAt); err != nil {
		common.ApiErrorI18n(c, i18n.MsgRetryLater)
		return
	}
	// client_id 暂存到 controller 层 map（5 分钟内即用即弃）；后续若给
	// desktop_auth_session 加 client_id 列，可改为持久化。
	stashClientId(req.SessionId, clientId)

	common.ApiSuccess(c, nil)
}

// OAuthDeviceInfo 授权页查询 session 状态 + client 元信息（用于渲染 logo / 名字）
func OAuthDeviceInfo(c *gin.Context) {
	sessionId := c.Query("session_id")
	if !isValidUUID(sessionId) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	session, err := model.GetDesktopAuthSession(sessionId)
	if err != nil ||
		session.Status != model.DesktopAuthStatusPending ||
		session.ExpiresAt <= common.GetTimestamp() {
		common.ApiErrorI18n(c, i18n.MsgDesktopAuthSessionInvalid)
		return
	}

	clientId := loadClientId(sessionId)
	if clientId == "" {
		clientId = builtinClientID
	}

	clientInfo := defaultClientInfoFor(clientId)
	if clientRec, err := model.GetOAuthClientByClientID(clientId); err == nil && clientRec != nil {
		clientInfo = gin.H{
			"client_id":     clientRec.ClientId,
			"name":          clientRec.Name,
			"description":   clientRec.Description,
			"logo_url":      clientRec.LogoURL,
			"homepage_url":  clientRec.HomepageURL,
			"contact_email": clientRec.ContactEmail,
			"verified":      clientRec.Verified,
		}
	}

	common.ApiSuccess(c, gin.H{
		"status":    "pending",
		"client_id": clientId,
		"client":    clientInfo,
	})
}

// OAuthDeviceConfirm 用户在浏览器同意/拒绝（UserAuth 路径）
func OAuthDeviceConfirm(c *gin.Context) {
	var req oauthDeviceConfirmReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if !isValidUUID(req.SessionId) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if req.Action != "approve" && req.Action != "reject" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	if req.Action == "reject" {
		_ = model.ExpireDesktopSession(req.SessionId)
		common.ApiSuccess(c, nil)
		return
	}

	// 同意授权：每次签发一把**独立**的 OAT（amux_api_oat_）写入 user_access_tokens。
	// 不再共享 user.access_token，互不影响——撤销/泄露隔离都是单条粒度。
	userId := c.GetInt("id")

	clientId := loadClientId(req.SessionId)
	if clientId == "" {
		clientId = builtinClientID
	}
	tokenName := "OAuth Authorization"
	if clientRec, err := model.GetOAuthClientByClientID(clientId); err == nil && clientRec != nil {
		tokenName = clientRec.Name
	}

	rec := &model.UserAccessToken{
		UserId: userId,
		Name:   tokenName,
		Source: model.UserAccessTokenSourceDeviceFlow,
		SourceMeta: encodeSourceMeta(map[string]any{
			"client_id":  clientId,
			"session_id": req.SessionId,
			"ip":         c.ClientIP(),
			"user_agent": c.Request.UserAgent(),
		}),
		Scopes: "full",
		// ExpiresAt: nil → 由空闲 90 天过期机制兜底
	}
	plaintext, err := model.CreateUserAccessToken(rec, model.UserAccessTokenPrefixOAT)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgRetryLater)
		return
	}

	// 把 plaintext 写入 session（client 端一次性消费）
	if err := model.AuthorizeDesktopSession(req.SessionId, userId, plaintext); err != nil {
		// 写入失败时把刚签的 OAT 撤掉，避免悬挂记录
		_ = model.RevokeUserAccessToken(rec.Id, "session-authorize-failed")
		common.ApiErrorI18n(c, i18n.MsgDesktopAuthSessionInvalid)
		return
	}

	common.ApiSuccess(c, nil)
}

// OAuthDeviceCheck 外部应用轮询拿 token（一次性消费）
func OAuthDeviceCheck(c *gin.Context) {
	sessionId := c.Query("session_id")
	if !isValidUUID(sessionId) {
		c.JSON(200, gin.H{"status": "expired"})
		return
	}

	session, err := model.GetDesktopAuthSession(sessionId)
	if err != nil {
		c.JSON(200, gin.H{"status": "expired"})
		return
	}

	switch session.Status {
	case model.DesktopAuthStatusPending:
		if session.ExpiresAt <= common.GetTimestamp() {
			c.JSON(200, gin.H{"status": "expired"})
			return
		}
		c.JSON(200, gin.H{"status": "pending"})

	case model.DesktopAuthStatusAuthorized:
		// 原子性消费 session —— 同一 token 只能被取一次
		consumed, err := model.ConsumeDesktopSession(sessionId)
		if err != nil {
			c.JSON(200, gin.H{"status": "expired"})
			return
		}
		c.JSON(200, gin.H{
			"status":       "authorized",
			"user_id":      consumed.UserId,
			"access_token": consumed.AccessToken,
		})

	default:
		// used or expired
		c.JSON(200, gin.H{"status": "expired"})
	}
}

// encodeSourceMeta 把 device flow 的来源元信息编成 JSON 字符串入库。
// 失败时返回空串——审计不可用比阻塞授权更糟，所以错误吞掉只记日志。
func encodeSourceMeta(m map[string]any) string {
	b, err := common.Marshal(m)
	if err != nil {
		common.SysLog("encode source_meta failed: " + err.Error())
		return ""
	}
	return string(b)
}

// =============================================================================
// 临时 client_id 缓存（仅 controller 层；进程重启即丢，session TTL 仅 5 分钟，影响有限）
// 单进程读写有锁；后续若给 desktop_auth_session 加 client_id 列，这块可移除。
// =============================================================================

var (
	clientIdStashMu sync.Mutex
	clientIdStash   = map[string]string{}
)

func stashClientId(sessionId, clientId string) {
	clientIdStashMu.Lock()
	defer clientIdStashMu.Unlock()
	clientIdStash[sessionId] = clientId
	// 简单清理：当 stash 大小超过阈值时清扫一遍过期 session
	if len(clientIdStash) > 2048 {
		go janitorClientIdStash()
	}
}

func loadClientId(sessionId string) string {
	clientIdStashMu.Lock()
	defer clientIdStashMu.Unlock()
	return clientIdStash[sessionId]
}

func janitorClientIdStash() {
	clientIdStashMu.Lock()
	defer clientIdStashMu.Unlock()
	for sid := range clientIdStash {
		if sess, err := model.GetDesktopAuthSession(sid); err != nil ||
			sess == nil ||
			sess.ExpiresAt <= common.GetTimestamp() ||
			sess.Status != model.DesktopAuthStatusPending {
			delete(clientIdStash, sid)
		}
	}
}

// defaultClientInfoFor 在 oauth_clients 表查不到记录时的占位文案，授权页仍能渲染。
func defaultClientInfoFor(clientId string) gin.H {
	if clientId == builtinClientID {
		return gin.H{
			"client_id":   clientId,
			"name":        "Amux Desktop Client",
			"description": "",
			"logo_url":    "https://cdn.amux.ai/logo/icon.png",
			"verified":    true,
		}
	}
	return gin.H{
		"client_id":   clientId,
		"name":        clientId,
		"description": "",
		"logo_url":    "",
		"verified":    false,
	}
}
