package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/marketing"
	"github.com/QuantumNous/new-api/service/marketing/providers/resend"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// TestResendTokenRequest 测试令牌请求体。
// 可以传入新令牌测试（"试一下能不能用再保存"场景），也可省略用当前已保存的令牌测。
type TestResendTokenRequest struct {
	APIKey string `json:"api_key"`
}

// TestResendToken 接收一个 Resend API Key，调一次轻量 List Contacts 验证是否有效。
// 不持久化、不切换 Provider；纯粹给后台 UI"测试令牌"按钮用。
//
// 返回：
//   - 200 + {success:true}            令牌有效
//   - 200 + {success:false, message}  令牌无效（401/403 等）或网络/服务异常
//
// 设计取舍：不分类 4xx vs 5xx，把原始错误消息透传给前端，让 admin 自己看消息排错。
func TestResendToken(c *gin.Context) {
	var req TestResendTokenRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		// 没传或解析失败时回退到当前保存的令牌
		req.APIKey = ""
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		apiKey = operation_setting.ResendAPIKey
	}
	if apiKey == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "未提供 API Key 且当前未配置",
		})
		return
	}

	provider, err := resend.New(resend.Config{APIKey: apiKey})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 短超时，避免被慢 API 拖死前端请求
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	if err := provider.Ping(ctx); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "令牌验证失败：" + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "令牌有效",
	})
}

// BackfillMarketing 一次性手工触发"历史付费用户回填"。
//
// 行为：
//   - Provider 未注入（未开启或未配置）→ 400 拒绝
//   - 已有任务在跑 → 200 success=false 提示"已有任务在跑"
//   - 否则：在后台 goroutine 启动；HTTP 立即返回"已开始"
//
// 进度查询走 GET /api/option/backfill_marketing/status。
// 进程级互斥（service/marketing.IsBackfillRunning），多实例不互斥但 Sync 幂等所以无害。
func BackfillMarketing(c *gin.Context) {
	if marketing.CurrentProvider() == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "邮件营销未开启或未配置",
		})
		return
	}
	if marketing.IsBackfillRunning() {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "已有回填任务在运行，请稍后查看状态",
		})
		return
	}
	gopool.Go(func() {
		// 用独立 background context；HTTP ctx 已结束
		_, err := marketing.Backfill(context.Background(), marketing.BackfillOpts{})
		if err != nil && !errors.Is(err, marketing.ErrBackfillRunning) {
			common.SysError("backfill marketing failed to start: " + err.Error())
		}
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "回填任务已启动，请查看状态或后台日志",
	})
}

// GetBackfillMarketingStatus 返回上一次（含当前正在跑的）回填任务状态。
// 任何时候都安全调用：从未跑过返回 running=false / result=null。
func GetBackfillMarketingStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"running": marketing.IsBackfillRunning(),
			"result":  marketing.LastBackfillResult(),
		},
	})
}

// ---------- 用户端：个人订阅管理 ----------
//
// 设计见 docs/event-system-design.md 第 23 节。
//
// amux 不存订阅状态；每次请求都实时跟 Provider 双向交互。
// 这样 Resend 端的变化（用户邮件点退订等）会在用户下次进设置页时自动反映。
//
// 入口路由都在 userRoute → selfRoute（UserAuth 中间件，确保只能改自己的）。

// GetMyMarketingSubscriptions 返回当前用户的"可订阅 topic 列表 + 当前订阅状态 + 资格"。
//
// 响应结构：
//
//	{
//	  "success": true,
//	  "data": {
//	    "eligible": true,                        // 是否付费用户（非付费时下面字段可能为空）
//	    "provider_configured": true,             // 后台是否已开启并配置 marketing
//	    "available_topics": [{id, name, description}, ...],
//	    "current": { "global_unsubscribed": false, "topics": [{topic_id, subscribed}, ...] }
//	  }
//	}
func GetMyMarketingSubscriptions(c *gin.Context) {
	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil || user == nil {
		common.ApiError(c, errors.New("user not found"))
		return
	}

	provider := marketing.CurrentProvider()
	if provider == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"eligible":            false,
				"provider_configured": false,
			},
		})
		return
	}

	eligible, err := marketing.IsEligible(user)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !eligible {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"eligible":            false,
				"provider_configured": true,
			},
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	topicIDs := splitMarketingTopicIDs(operation_setting.ResendDefaultTopicIDs)
	topics, err := provider.ListTopics(ctx, topicIDs)
	if err != nil {
		common.SysError("marketing list topics failed: " + err.Error())
		topics = nil // 降级：展示空列表 + 让用户感知错误（前端可以根据 error 字段加提示）
	}

	current, err := provider.GetSubscriptions(ctx, user.Email)
	if err != nil {
		common.SysError("marketing get subscriptions failed: " + err.Error())
		current = &marketing.Subscriptions{} // 降级：当作未设置
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"eligible":            true,
			"provider_configured": true,
			"available_topics":    topics,
			"current":             current,
		},
	})
}

// UpdateMyMarketingSubscriptions 接收用户在 amux 设置页的勾选，写回 Provider。
func UpdateMyMarketingSubscriptions(c *gin.Context) {
	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil || user == nil {
		common.ApiError(c, errors.New("user not found"))
		return
	}

	provider := marketing.CurrentProvider()
	if provider == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "邮件营销未启用",
		})
		return
	}
	eligible, err := marketing.IsEligible(user)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !eligible {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "当前账户无权管理邮件订阅",
		})
		return
	}

	var req marketing.Subscriptions
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}

	// 用 background 派生而非 c.Request.Context()：客户端 HTTP 超时不应取消上游 Resend
	// 调用（取消会导致"前端报错但后端已写入"的状态分歧）。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Username
	}
	if err := provider.UpdateSubscriptions(ctx, user.Email, displayName, req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "更新失败：" + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新成功",
	})
}

// splitMarketingTopicIDs 与 main.go 的 splitTopicIDs 等价。这里复制一份避免
// controller 反向 import main 包。
func splitMarketingTopicIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
