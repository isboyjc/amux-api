package controller

import (
	"regexp"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type createDesktopAuthSessionRequest struct {
	SessionId string `json:"session_id" binding:"required"`
}

type confirmDesktopAuthRequest struct {
	SessionId string `json:"session_id" binding:"required"`
	Action    string `json:"action" binding:"required"`
}

func isValidUUID(s string) bool {
	return uuidRegex.MatchString(s)
}

// CreateDesktopAuthSession creates a new pending desktop auth session.
// POST /api/desktop/auth/session (no auth required)
func CreateDesktopAuthSession(c *gin.Context) {
	var req createDesktopAuthSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	if !isValidUUID(req.SessionId) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	// Check if session already exists
	existing, _ := model.GetDesktopAuthSession(req.SessionId)
	if existing != nil {
		common.ApiErrorI18n(c, i18n.MsgDesktopAuthSessionExists)
		return
	}

	expiresAt := common.GetTimestamp() + 5*60 // 5 minutes
	if err := model.CreateDesktopAuthSession(req.SessionId, expiresAt); err != nil {
		common.ApiErrorI18n(c, i18n.MsgRetryLater)
		return
	}

	common.ApiSuccess(c, nil)
}

// GetDesktopAuthInfo validates a desktop auth session for the frontend authorize page.
// GET /api/desktop/auth/info?session_id=xxx (no auth required)
func GetDesktopAuthInfo(c *gin.Context) {
	sessionId := c.Query("session_id")
	if !isValidUUID(sessionId) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	session, err := model.GetDesktopAuthSession(sessionId)
	if err != nil || session.Status != model.DesktopAuthStatusPending || session.ExpiresAt <= common.GetTimestamp() {
		common.ApiErrorI18n(c, i18n.MsgDesktopAuthSessionInvalid)
		return
	}

	common.ApiSuccess(c, gin.H{
		"status": "pending",
	})
}

// ConfirmDesktopAuth handles user approval or rejection of a desktop auth session.
// POST /api/desktop/auth/confirm (requires UserAuth)
func ConfirmDesktopAuth(c *gin.Context) {
	var req confirmDesktopAuthRequest
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

	// Handle reject
	if req.Action == "reject" {
		_ = model.ExpireDesktopSession(req.SessionId)
		common.ApiSuccess(c, nil)
		return
	}

	// Handle approve: get user's token first, then atomically authorize the session.
	// AuthorizeDesktopSession will check status=pending AND expires_at>now,
	// so no need for a separate pre-check SELECT (avoids TOCTOU).
	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, true)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgRetryLater)
		return
	}

	accessToken := user.GetAccessToken()
	if accessToken == "" {
		// Generate a new access token (same logic as GenerateAccessToken in user.go)
		randI := common.GetRandomInt(4)
		key, err := common.GenerateRandomKey(29 + randI)
		if err != nil {
			common.ApiErrorI18n(c, i18n.MsgGenerateFailed)
			return
		}
		user.SetAccessToken(key)

		if model.DB.Where("access_token = ?", user.AccessToken).First(user).RowsAffected != 0 {
			common.ApiErrorI18n(c, i18n.MsgUuidDuplicate)
			return
		}

		if err := user.Update(false); err != nil {
			common.ApiErrorI18n(c, i18n.MsgRetryLater)
			return
		}
		accessToken = user.GetAccessToken()
	}

	// Authorize the session
	if err := model.AuthorizeDesktopSession(req.SessionId, userId, accessToken); err != nil {
		common.ApiErrorI18n(c, i18n.MsgDesktopAuthSessionInvalid)
		return
	}

	common.ApiSuccess(c, nil)
}

// CheckDesktopAuth is polled by Desktop to check authorization status.
// GET /api/desktop/auth/check?session_id=xxx (no auth required)
func CheckDesktopAuth(c *gin.Context) {
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
		// Atomically consume the session — token can only be retrieved once
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
