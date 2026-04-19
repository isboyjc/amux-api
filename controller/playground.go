package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/dto"
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

// playgroundTaskPreamble 是 PlaygroundTask / PlaygroundTaskFetch 的公共前置：
// 把 UserAuth 当前的登录用户当作一次调用的身份装入临时 Token，和 Playground()
// 的做法保持一致。所有"任务类"（视频生成等异步路径）都走这条。
func playgroundTaskPreamble(c *gin.Context) *dto.TaskError {
	if c.GetBool("use_access_token") {
		return &dto.TaskError{
			Code:       "access_denied",
			Message:    "暂不支持使用 access token",
			StatusCode: http.StatusForbidden,
			LocalError: true,
		}
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		return &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
			LocalError: true,
		}
	}

	userId := c.GetInt("id")
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		return &dto.TaskError{
			Code:       "query_user_cache_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
			LocalError: true,
		}
	}
	userCache.WriteContext(c)

	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("playground-%s", relayInfo.UsingGroup),
		Group:  relayInfo.UsingGroup,
	}
	_ = middleware.SetupContextForToken(c, tempToken)
	return nil
}

// PlaygroundTask 处理 /pg/video/generations POST —— 提交视频生成任务。
// 这里只负责把 UserAuth 上下文装成临时 Token、写入 userCache，之后把流程
// 交给通用的 RelayTask 处理（走同一条"选渠道 → 预扣费 → 适配器转上游"路径）。
func PlaygroundTask(c *gin.Context) {
	if taskErr := playgroundTaskPreamble(c); taskErr != nil {
		c.JSON(taskErr.StatusCode, taskErr)
		return
	}
	RelayTask(c)
}

// PlaygroundTaskFetch 处理 /pg/video/generations/:task_id GET —— 查询任务状态。
// 响应格式与 /v1/videos/:task_id 一致（OpenAIVideo），前端可直接消费。
func PlaygroundTaskFetch(c *gin.Context) {
	if taskErr := playgroundTaskPreamble(c); taskErr != nil {
		c.JSON(taskErr.StatusCode, taskErr)
		return
	}
	RelayTaskFetch(c)
}
