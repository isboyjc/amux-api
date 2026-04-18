package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// playgroundRelayFormat 根据 /pg/* 具体子路径决定应该用哪种 RelayFormat
// 去解析请求体。没匹配上就退到 chat（OpenAI）格式。
//
//	/pg/chat/completions     → RelayFormatOpenAI       （GeneralOpenAIRequest）
//	/pg/images/generations   → RelayFormatOpenAIImage  （ImageRequest）
//	/pg/images/edits         → RelayFormatOpenAIImage
func playgroundRelayFormat(path string) types.RelayFormat {
	if strings.HasPrefix(path, "/pg/images/") {
		return types.RelayFormatOpenAIImage
	}
	return types.RelayFormatOpenAI
}

func Playground(c *gin.Context) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	useAccessToken := c.GetBool("use_access_token")
	if useAccessToken {
		newAPIError = types.NewError(errors.New("暂不支持使用 access token"), types.ErrorCodeAccessDenied, types.ErrOptionWithSkipRetry())
		return
	}

	format := playgroundRelayFormat(c.Request.URL.Path)
	relayInfo, err := relaycommon.GenRelayInfo(c, format, nil, nil)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		return
	}

	userId := c.GetInt("id")

	// Write user context to ensure acceptUnsetRatio is available
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		return
	}
	userCache.WriteContext(c)

	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("playground-%s", relayInfo.UsingGroup),
		Group:  relayInfo.UsingGroup,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	Relay(c, format)
}
