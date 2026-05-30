package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/service/emailtpl"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

func TestStatus(c *gin.Context) {
	err := model.PingDB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "数据库连接失败",
		})
		return
	}
	// 获取HTTP统计信息
	httpStats := middleware.GetStats()
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Server is running",
		"http_stats": httpStats,
	})
	return
}

const publicTokenStatsRedisKey = "public:total_tokens"

var cachedTotalTokens atomic.Int64

// SyncPublicTokenStats 后台定时从 DB 查询总 Token 数并写入缓存（Redis 或内存）
// 在 main.go 中以 goroutine 启动
func SyncPublicTokenStats(intervalSeconds int) {
	// 启动时立即同步一次
	refreshPublicTokenStats()
	for {
		time.Sleep(time.Duration(intervalSeconds) * time.Second)
		refreshPublicTokenStats()
	}
}

func refreshPublicTokenStats() {
	total, err := model.GetPublicTokenStats()
	if err != nil {
		common.SysError("failed to refresh public token stats: " + err.Error())
		return
	}
	// 写入内存（兜底 / 无 Redis 场景）
	cachedTotalTokens.Store(total)
	// 写入 Redis（多实例共享）
	if common.RedisEnabled {
		_ = common.RedisSet(publicTokenStatsRedisKey, strconv.FormatInt(total, 10), 10*time.Minute)
	}
}

func GetPublicTokenStats(c *gin.Context) {
	var total int64
	// 优先从 Redis 读
	if common.RedisEnabled {
		val, err := common.RedisGet(publicTokenStatsRedisKey)
		if err == nil {
			total, _ = strconv.ParseInt(val, 10, 64)
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    gin.H{"total_tokens": total},
			})
			return
		}
	}
	// 降级到内存缓存
	total = cachedTotalTokens.Load()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"total_tokens": total},
	})
}

func GetStatus(c *gin.Context) {

	cs := console_setting.GetConsoleSetting()
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()

	passkeySetting := system_setting.GetPasskeySettings()
	legalSetting := system_setting.GetLegalSettings()

	data := gin.H{
		"version":                     common.Version,
		"start_time":                  common.StartTime,
		"email_verification":          common.EmailVerificationEnabled,
		"github_oauth":                common.GitHubOAuthEnabled,
		"github_client_id":            common.GitHubClientId,
		"discord_oauth":               system_setting.GetDiscordSettings().Enabled,
		"discord_client_id":           system_setting.GetDiscordSettings().ClientId,
		"linuxdo_oauth":               common.LinuxDOOAuthEnabled,
		"linuxdo_client_id":           common.LinuxDOClientId,
		"linuxdo_minimum_trust_level": common.LinuxDOMinimumTrustLevel,
		"telegram_oauth":              common.TelegramOAuthEnabled,
		"telegram_bot_name":           common.TelegramBotName,
		"system_name":                 common.SystemName,
		"logo":                        common.Logo,
		"footer_html":                 common.Footer,
		"wechat_qrcode":               common.WeChatAccountQRCodeImageURL,
		"wechat_login":                common.WeChatAuthEnabled,
		"server_address":              system_setting.ServerAddress,
		"turnstile_check":             common.TurnstileCheckEnabled,
		"turnstile_site_key":          common.TurnstileSiteKey,
		"top_up_link":                 common.TopUpLink,
		"docs_link":                   operation_setting.GetGeneralSetting().DocsLink,
		"quota_per_unit":              common.QuotaPerUnit,
		// 兼容旧前端：保留 display_in_currency，同时提供新的 quota_display_type
		"display_in_currency":           operation_setting.IsCurrencyDisplay(),
		"quota_display_type":            operation_setting.GetQuotaDisplayType(),
		"custom_currency_symbol":        operation_setting.GetGeneralSetting().CustomCurrencySymbol,
		"custom_currency_exchange_rate": operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate,
		"enable_batch_update":           common.BatchUpdateEnabled,
		"enable_drawing":                common.DrawingEnabled,
		"enable_task":                   common.TaskEnabled,
		"enable_data_export":            common.DataExportEnabled,
		"data_export_default_time":      common.DataExportDefaultTime,
		"default_collapse_sidebar":      common.DefaultCollapseSidebar,
		"mj_notify_enabled":             setting.MjNotifyEnabled,
		"chats":                         setting.Chats,
		"demo_site_enabled":             operation_setting.DemoSiteEnabled,
		"self_use_mode_enabled":         operation_setting.SelfUseModeEnabled,
		"register_enabled":              common.RegisterEnabled,
		"password_register_enabled":     common.PasswordRegisterEnabled,
		"default_use_auto_group":        setting.DefaultUseAutoGroup,

		"usd_exchange_rate": operation_setting.USDExchangeRate,
		"price":             operation_setting.Price,
		"stripe_unit_price": setting.StripeUnitPrice,
		"stripe_currency":   setting.StripeCurrency,
		"stripe_currency_symbol": func() string {
			if setting.StripeCurrency == "USD" {
				return "$"
			}
			return "¥"
		}(),
		"enable_stripe_topup": setting.StripeApiSecret != "" && setting.StripeWebhookSecret != "",

		// 面板启用开关
		"api_info_enabled":      cs.ApiInfoEnabled,
		"uptime_kuma_enabled":   cs.UptimeKumaEnabled,
		"announcements_enabled": cs.AnnouncementsEnabled,
		"faq_enabled":           cs.FAQEnabled,
		"support_enabled":       cs.SupportEnabled,

		// 模块管理配置
		"HeaderNavModules":    common.OptionMap["HeaderNavModules"],
		"SidebarModulesAdmin": common.OptionMap["SidebarModulesAdmin"],

		"oidc_enabled":                system_setting.GetOIDCSettings().Enabled,
		"oidc_client_id":              system_setting.GetOIDCSettings().ClientId,
		"oidc_authorization_endpoint": system_setting.GetOIDCSettings().AuthorizationEndpoint,
		"passkey_login":               passkeySetting.Enabled,
		"passkey_display_name":        passkeySetting.RPDisplayName,
		"passkey_rp_id":               passkeySetting.RPID,
		"passkey_origins":             passkeySetting.Origins,
		"passkey_allow_insecure":      passkeySetting.AllowInsecureOrigin,
		"passkey_user_verification":   passkeySetting.UserVerification,
		"passkey_attachment":          passkeySetting.AttachmentPreference,
		"setup":                       constant.Setup,
		"user_agreement_enabled":      legalSetting.UserAgreement != "",
		"privacy_policy_enabled":      legalSetting.PrivacyPolicy != "",
		"checkin_enabled":             operation_setting.GetCheckinSetting().Enabled,
		// 工单系统总开关。前端用这个字段决定是否渲染"我的工单"/"工单管理"侧边栏
		// 入口以及头部红点按钮。后端接口本身也会按 enabled 拒绝建单/回复。
		"ticket_enabled":              operation_setting.GetTicketSetting().Enabled,
		"AffShowInvitees":             common.OptionMap["AffShowInvitees"],
		"AffRebateRatio":              common.OptionMap["AffRebateRatio"],

		// 对象存储「显示侧优化」给前端用：
		//   - storage_public_base_url：判断哪些 URL 来自我们桶、可以套 cdn-cgi
		//   - storage_image_transform_enabled：admin 是否打开 CF Image Resizing 开关
		// 不暴露任何凭证；纯展示用配置
		"storage_public_base_url":          system_setting.GetStorageSettings().R2PublicBaseURL,
		"storage_image_transform_enabled":  system_setting.GetStorageSettings().ImageTransformEnabled,

		"_qn": "new-api",
	}

	// 公告横幅：登录前后都要看得到，所以放 status 走匿名可访问通道。
	// version 用于前端 dismiss 比对——内容改变时 version 跟着变，已点过
	// X 的用户也会重新看到。仅在 enabled 时下发完整字段，关闭时只给一个
	// enabled=false 让前端早退，不带任何文案/链接，减少前端干扰
	if ab := operation_setting.GetAnnouncementBarSetting(); ab.Enabled {
		data["announcement_bar"] = gin.H{
			"enabled":         true,
			"content":         ab.Content,
			"link":            ab.Link,
			"open_in_new_tab": ab.OpenInNewTab,
			"bg_color":        ab.BgColor,
			"accent_color":    ab.AccentColor,
			"text_color":      ab.TextColor,
			"version":         ab.Version,
		}
	} else {
		data["announcement_bar"] = gin.H{"enabled": false}
	}

	// 侧边栏底部宣传位轮播：仅 console 路由下渲染。enabled 且 items 至少 1 条
	// 才下发完整字段；否则下发 enabled=false 让前端早退。items 在这里以
	// 已解析数组的形式发出（前端拿来即用），不暴露原始字符串
	if sc := operation_setting.GetSidebarCarouselSetting(); sc.Enabled {
		items := operation_setting.GetSidebarCarouselItems()
		if len(items) > 0 {
			data["sidebar_carousel"] = gin.H{
				"enabled": true,
				"items":   items,
				"version": sc.Version,
			}
		} else {
			data["sidebar_carousel"] = gin.H{"enabled": false}
		}
	} else {
		data["sidebar_carousel"] = gin.H{"enabled": false}
	}

	// 根据启用状态注入可选内容
	if cs.ApiInfoEnabled {
		data["api_info"] = console_setting.GetApiInfo()
	}
	if cs.AnnouncementsEnabled {
		data["announcements"] = console_setting.GetAnnouncements()
	}
	if cs.FAQEnabled {
		data["faq"] = console_setting.GetFAQ()
	}
	if cs.SupportEnabled {
		data["support"] = console_setting.GetSupport()
	}

	// Add enabled custom OAuth providers
	customProviders := oauth.GetEnabledCustomProviders()
	if len(customProviders) > 0 {
		type CustomOAuthInfo struct {
			Id                    int    `json:"id"`
			Name                  string `json:"name"`
			Slug                  string `json:"slug"`
			Icon                  string `json:"icon"`
			ClientId              string `json:"client_id"`
			AuthorizationEndpoint string `json:"authorization_endpoint"`
			Scopes                string `json:"scopes"`
		}
		providersInfo := make([]CustomOAuthInfo, 0, len(customProviders))
		for _, p := range customProviders {
			config := p.GetConfig()
			providersInfo = append(providersInfo, CustomOAuthInfo{
				Id:                    config.Id,
				Name:                  config.Name,
				Slug:                  config.Slug,
				Icon:                  config.Icon,
				ClientId:              config.ClientId,
				AuthorizationEndpoint: config.AuthorizationEndpoint,
				Scopes:                config.Scopes,
			})
		}
		data["custom_oauth_providers"] = providersInfo
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
	return
}

func GetNotice(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["Notice"],
	})
	return
}

func GetAbout(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["About"],
	})
	return
}

func GetUserAgreement(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    system_setting.GetLegalSettings().UserAgreement,
	})
	return
}

func GetPrivacyPolicy(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    system_setting.GetLegalSettings().PrivacyPolicy,
	})
	return
}

func GetMidjourney(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["Midjourney"],
	})
	return
}

func GetHomePageContent(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["HomePageContent"],
	})
	return
}

func SendEmailVerification(c *gin.Context) {
	email := c.Query("email")
	if err := common.Validate.Var(email, "required,email"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的邮箱地址",
		})
		return
	}
	localPart := parts[0]
	domainPart := parts[1]
	if common.EmailDomainBlacklistEnabled {
		for _, domain := range common.EmailDomainBlacklist {
			if domain != "" && domainPart == domain {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": i18n.T(c, i18n.MsgSettingEmailNotSupported),
				})
				return
			}
		}
	}
	if common.EmailDomainRestrictionEnabled {
		allowed := false
		for _, domain := range common.EmailDomainWhitelist {
			if domainPart == domain {
				allowed = true
				break
			}
		}
		if !allowed {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "The administrator has enabled the email domain name whitelist, and your email address is not allowed due to special symbols or it's not in the whitelist.",
			})
			return
		}
	}
	if common.EmailAliasRestrictionEnabled {
		containsSpecialSymbols := strings.Contains(localPart, "+") || strings.Contains(localPart, ".")
		if containsSpecialSymbols {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "管理员已启用邮箱地址别名限制，您的邮箱地址由于包含特殊符号而被拒绝。",
			})
			return
		}
	}

	if model.IsEmailAlreadyTaken(email) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "邮箱地址已被占用",
		})
		return
	}
	code := common.GenerateVerificationCode(6)
	common.RegisterVerificationCodeWithKey(email, code, common.EmailVerificationPurpose)
	subject := fmt.Sprintf("%s 邮箱验证码", common.SystemName)
	// 验证码邮件：用 Highlight 大字号居中展示 6 位码，等宽字体 + 字间距，
	// 用户一眼就能读、方便手抄；Footnote 放安全提示。
	content := emailtpl.Render(emailtpl.Content{
		Tone:     emailtpl.ToneInfo,
		Eyebrow:  "账户安全",
		Headline: "验证您的邮箱",
		Intro: fmt.Sprintf(
			"您好，您正在进行 %s 的邮箱验证。请使用下方的验证码完成验证。",
			emailtpl.HtmlEscape(common.SystemName)),
		Highlight: emailtpl.Highlight{
			Value: code,
			Hint:  fmt.Sprintf("验证码 %d 分钟内有效", common.VerificationValidMinutes),
		},
		Footnote: "如果不是本人操作，请忽略此邮件。任何人都不会向您索取此验证码，请勿向第三方泄露。",
	})
	err := common.SendEmail(subject, email, content)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func SendPasswordResetEmail(c *gin.Context) {
	email := c.Query("email")
	if err := common.Validate.Var(email, "required,email"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if model.IsEmailAlreadyTaken(email) {
		code := common.GenerateVerificationCode(0)
		common.RegisterVerificationCodeWithKey(email, code, common.PasswordResetPurpose)
		link := fmt.Sprintf("%s/user/reset?email=%s&token=%s", system_setting.ServerAddress, email, code)
		subject := fmt.Sprintf("%s 密码重置", common.SystemName)
		// 密码重置邮件：CTA 按钮指向重置链接；Rows 兜底显示一份纯链接，
		// 给按钮点不动的客户端（极少数老邮件客户端） / 用户想拷贝到其它
		// 浏览器打开时用。Footnote 提示有效期 + 安全免责。
		content := emailtpl.Render(emailtpl.Content{
			Tone:     emailtpl.ToneInfo,
			Eyebrow:  "账户安全",
			Headline: "重置您的密码",
			Intro: fmt.Sprintf(
				"您好，您正在进行 %s 的密码重置。点击下方按钮即可设置新密码。",
				emailtpl.HtmlEscape(common.SystemName)),
			CTAHref:  link,
			CTALabel: "重置密码",
			Rows: []emailtpl.Row{
				{
					Label: "或复制链接",
					Value: fmt.Sprintf(
						`<a href="%s" style="color:#475569;text-decoration:none;word-break:break-all;">%s</a>`,
						link, emailtpl.HtmlEscape(link)),
				},
			},
			Footnote: fmt.Sprintf(
				"重置链接 %d 分钟内有效。如果不是本人操作，请忽略此邮件并考虑修改账号密码。",
				common.VerificationValidMinutes),
		})
		err := common.SendEmail(subject, email, content)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("failed to send password reset email to %s: %s", email, err.Error()))
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

type PasswordResetRequest struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

func ResetPassword(c *gin.Context) {
	var req PasswordResetRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	if req.Email == "" || req.Token == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if !common.VerifyCodeWithKey(req.Email, req.Token, common.PasswordResetPurpose) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "重置链接非法或已过期",
		})
		return
	}
	password := common.GenerateVerificationCode(12)
	err = model.ResetUserPasswordByEmail(req.Email, password)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.DeleteKey(req.Email, common.PasswordResetPurpose)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    password,
	})
	return
}
