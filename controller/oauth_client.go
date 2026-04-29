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
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// =============================================================================
// 管理员 OAuth Client 管理（router 层挂 /api/admin/oauth/clients）
//   GET    /api/admin/oauth/clients
//   POST   /api/admin/oauth/clients
//   PATCH  /api/admin/oauth/clients/:id
//   DELETE /api/admin/oauth/clients/:id        （软禁用）
//   POST   /api/admin/oauth/clients/:id/rotate （轮换 secret）
// =============================================================================

type createOAuthClientReq struct {
	ClientId      string `json:"client_id"`     // 留空则按 name 自动生成
	Name          string `json:"name" binding:"required"`
	Description   string `json:"description"`
	LogoURL       string `json:"logo_url"`
	HomepageURL   string `json:"homepage_url"`
	ContactEmail  string `json:"contact_email"`
	AllowedScopes string `json:"allowed_scopes"`
	Verified      bool   `json:"verified"`
}

func AdminCreateOAuthClient(c *gin.Context) {
	var req createOAuthClientReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		common.ApiErrorI18n(c, i18n.MsgUatNameRequired)
		return
	}

	rec := &model.OAuthClient{
		ClientId:      req.ClientId,
		Name:          req.Name,
		Description:   req.Description,
		LogoURL:       req.LogoURL,
		HomepageURL:   req.HomepageURL,
		ContactEmail:  req.ContactEmail,
		AllowedScopes: req.AllowedScopes,
		Verified:      req.Verified,
		CreatedBy:     c.GetInt("id"),
	}
	secret, err := model.CreateOAuthClient(rec)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	view := oauthClientToView(rec)
	view["client_secret"] = secret // **仅本次返回**
	common.ApiSuccess(c, view)
}

func AdminListOAuthClients(c *gin.Context) {
	status := atoiOrZero(c.Query("status"))
	rows, err := model.ListOAuthClients(status)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, oauthClientToView(&r))
	}
	common.ApiSuccess(c, out)
}

type updateOAuthClientReq struct {
	Name          *string `json:"name"`
	Description   *string `json:"description"`
	LogoURL       *string `json:"logo_url"`
	HomepageURL   *string `json:"homepage_url"`
	ContactEmail  *string `json:"contact_email"`
	AllowedScopes *string `json:"allowed_scopes"`
	Verified      *bool   `json:"verified"`
	Status        *int    `json:"status"`
}

func AdminUpdateOAuthClient(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	var req updateOAuthClientReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	patch := map[string]interface{}{}
	if req.Name != nil {
		v := strings.TrimSpace(*req.Name)
		if v == "" {
			common.ApiErrorI18n(c, i18n.MsgUatNameRequired)
			return
		}
		patch["name"] = v
	}
	if req.Description != nil {
		patch["description"] = *req.Description
	}
	if req.LogoURL != nil {
		patch["logo_url"] = *req.LogoURL
	}
	if req.HomepageURL != nil {
		patch["homepage_url"] = *req.HomepageURL
	}
	if req.ContactEmail != nil {
		patch["contact_email"] = *req.ContactEmail
	}
	if req.AllowedScopes != nil {
		patch["allowed_scopes"] = *req.AllowedScopes
	}
	if req.Verified != nil {
		patch["verified"] = *req.Verified
	}
	if req.Status != nil {
		// 仅允许 1 / 2 两个值
		if *req.Status != model.OAuthClientStatusActive &&
			*req.Status != model.OAuthClientStatusDisabled {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		patch["status"] = *req.Status
	}
	if err := model.UpdateOAuthClient(id, patch); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// AdminDeleteOAuthClient 默认软禁用（status=2）。强制硬删走 ?hard=1。
func AdminDeleteOAuthClient(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	rec, err := model.GetOAuthClientByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgNotFound)
			return
		}
		common.ApiError(c, err)
		return
	}
	// 内置 client 不可删
	if rec.ClientId == model.OAuthClientAmuxDesktopId {
		common.ApiErrorI18n(c, i18n.MsgForbidden)
		return
	}
	if c.Query("hard") == "1" {
		if err := model.DeleteOAuthClient(id); err != nil {
			common.ApiError(c, err)
			return
		}
	} else {
		if err := model.DisableOAuthClient(id); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	common.ApiSuccess(c, nil)
}

func AdminRotateOAuthClientSecret(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	secret, err := model.RotateOAuthClientSecret(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"client_secret": secret})
}

func oauthClientToView(c *model.OAuthClient) gin.H {
	return gin.H{
		"id":             c.Id,
		"client_id":      c.ClientId,
		"name":           c.Name,
		"description":    c.Description,
		"logo_url":       c.LogoURL,
		"homepage_url":   c.HomepageURL,
		"contact_email":  c.ContactEmail,
		"allowed_scopes": c.AllowedScopes,
		"verified":       c.Verified,
		"status":         c.Status,
		"created_by":     c.CreatedBy,
		"created_at":     c.CreatedAt,
		"updated_at":     c.UpdatedAt,
	}
}
