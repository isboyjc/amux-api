package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

var completionRatioMetaOptionKeys = []string{
	"ModelPrice",
	"ModelRatio",
	"CompletionRatio",
	"CacheRatio",
	"CreateCacheRatio",
	"ImageRatio",
	"AudioRatio",
	"AudioCompletionRatio",
}

func collectModelNamesFromOptionValue(raw string, modelNames map[string]struct{}) {
	if strings.TrimSpace(raw) == "" {
		return
	}

	var parsed map[string]any
	if err := common.UnmarshalJsonStr(raw, &parsed); err != nil {
		return
	}

	for modelName := range parsed {
		modelNames[modelName] = struct{}{}
	}
}

func buildCompletionRatioMetaValue(optionValues map[string]string) string {
	modelNames := make(map[string]struct{})
	for _, key := range completionRatioMetaOptionKeys {
		collectModelNamesFromOptionValue(optionValues[key], modelNames)
	}

	meta := make(map[string]ratio_setting.CompletionRatioInfo, len(modelNames))
	for modelName := range modelNames {
		meta[modelName] = ratio_setting.GetCompletionRatioInfo(modelName)
	}

	jsonBytes, err := common.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(jsonBytes)
}

func GetOptions(c *gin.Context) {
	var options []*model.Option
	optionValues := make(map[string]string)
	common.OptionMapRWMutex.Lock()
	for k, v := range common.OptionMap {
		value := common.Interface2String(v)
		if strings.HasSuffix(k, "Token") ||
			strings.HasSuffix(k, "Secret") ||
			strings.HasSuffix(k, "Key") ||
			strings.HasSuffix(k, "secret") ||
			strings.HasSuffix(k, "api_key") {
			continue
		}
		options = append(options, &model.Option{
			Key:   k,
			Value: value,
		})
		for _, optionKey := range completionRatioMetaOptionKeys {
			if optionKey == k {
				optionValues[k] = value
				break
			}
		}
	}
	common.OptionMapRWMutex.Unlock()
	options = append(options, &model.Option{
		Key:   "CompletionRatioMeta",
		Value: buildCompletionRatioMetaValue(optionValues),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
	return
}

type OptionUpdateRequest struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

func UpdateOption(c *gin.Context) {
	var option OptionUpdateRequest
	err := common.DecodeJson(c.Request.Body, &option)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	switch option.Value.(type) {
	case bool:
		option.Value = common.Interface2String(option.Value.(bool))
	case float64:
		option.Value = common.Interface2String(option.Value.(float64))
	case int:
		option.Value = common.Interface2String(option.Value.(int))
	default:
		option.Value = fmt.Sprintf("%v", option.Value)
	}
	switch option.Key {
	case "GitHubOAuthEnabled":
		if option.Value == "true" && common.GitHubClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 GitHub OAuth，请先填入 GitHub Client Id 以及 GitHub Client Secret！",
			})
			return
		}
	case "discord.enabled":
		if option.Value == "true" && system_setting.GetDiscordSettings().ClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Discord OAuth，请先填入 Discord Client Id 以及 Discord Client Secret！",
			})
			return
		}
	case "oidc.enabled":
		if option.Value == "true" && system_setting.GetOIDCSettings().ClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 OIDC 登录，请先填入 OIDC Client Id 以及 OIDC Client Secret！",
			})
			return
		}
	case "LinuxDOOAuthEnabled":
		if option.Value == "true" && common.LinuxDOClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 LinuxDO OAuth，请先填入 LinuxDO Client Id 以及 LinuxDO Client Secret！",
			})
			return
		}
	case "EmailDomainRestrictionEnabled":
		if option.Value == "true" && len(common.EmailDomainWhitelist) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用邮箱域名限制，请先填入限制的邮箱域名！",
			})
			return
		}
	case "EmailDomainBlacklistEnabled":
		if option.Value == "true" && len(common.EmailDomainBlacklist) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用邮箱域名黑名单，请先填入要禁止的邮箱域名！",
			})
			return
		}
	case "WeChatAuthEnabled":
		if option.Value == "true" && common.WeChatServerAddress == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用微信登录，请先填入微信登录相关配置信息！",
			})
			return
		}
	case "TurnstileCheckEnabled":
		if option.Value == "true" && common.TurnstileSiteKey == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Turnstile 校验，请先填入 Turnstile 校验相关配置信息！",
			})

			return
		}
	case "TelegramOAuthEnabled":
		if option.Value == "true" && common.TelegramBotToken == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Telegram OAuth，请先填入 Telegram Bot Token！",
			})
			return
		}
	case "GroupRatio":
		err = ratio_setting.CheckGroupRatio(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "ImageRatio":
		err = ratio_setting.UpdateImageRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "图片倍率设置失败: " + err.Error(),
			})
			return
		}
	case "AudioRatio":
		err = ratio_setting.UpdateAudioRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "音频倍率设置失败: " + err.Error(),
			})
			return
		}
	case "AudioCompletionRatio":
		err = ratio_setting.UpdateAudioCompletionRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "音频补全倍率设置失败: " + err.Error(),
			})
			return
		}
	case "CreateCacheRatio":
		err = ratio_setting.UpdateCreateCacheRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "缓存创建倍率设置失败: " + err.Error(),
			})
			return
		}
	case "ModelRequestRateLimitGroup":
		err = setting.CheckModelRequestRateLimitGroup(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "AutomaticDisableStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "AutomaticRetryStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.api_info":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "ApiInfo")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.announcements":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "Announcements")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.faq":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "FAQ")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.uptime_kuma_groups":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "UptimeKumaGroups")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "storage.provider":
		// 当前只接受 r2；后续扩展时把已实现的 provider 加进来即可
		v := strings.ToLower(strings.TrimSpace(option.Value.(string)))
		if v != "" && v != "r2" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "暂不支持的对象存储提供方：" + v,
			})
			return
		}
	case "storage.r2_public_base_url":
		// 拼最终访问 URL 用，必须带 scheme；且不应以 "/" 结尾（前端拼接逻辑
		// 假设 base 不带尾斜杠）。空值视作"先清除"，后续 IsEnabled 会兜底。
		v := strings.TrimSpace(option.Value.(string))
		if v != "" {
			if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "公网访问基地址必须以 http:// 或 https:// 开头",
				})
				return
			}
		}
	case "storage.r2_endpoint":
		v := strings.TrimSpace(option.Value.(string))
		if v != "" && !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "S3 endpoint 必须以 http:// 或 https:// 开头",
			})
			return
		}
	case "announcement_bar.content":
		// 横幅文案：500 字封顶，足以塞下醒目的一句话；超过则截断会让 admin
		// 困惑，直接拒收并报错
		v := option.Value.(string)
		if len([]rune(v)) > 500 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "公告横幅文案最多 500 字",
			})
			return
		}
	case "announcement_bar.link":
		// 跳转链接：空字符串表示"纯文案、不可点"；非空必须带 scheme，避免
		// 前端 window.open 拿到相对路径或 javascript: 之类的奇怪输入
		v := strings.TrimSpace(option.Value.(string))
		if v != "" && !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "公告横幅跳转链接必须以 http:// 或 https:// 开头",
			})
			return
		}
	case "announcement_bar.bg_color",
		"announcement_bar.accent_color",
		"announcement_bar.text_color":
		// 颜色：接受 #RGB / #RRGGBB / #RRGGBBAA 三种 hex 格式；空值表示
		// 回到 CSS 默认。不接 rgb()/hsl() 等函数式表达，避免插入 CSS
		// 语法被滥用（虽然颜色字段最终走 inline style 不至于 XSS，
		// 但也没必要给一个"自由 CSS"入口）
		v := strings.TrimSpace(option.Value.(string))
		if v != "" && !isValidHexColor(v) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "颜色格式非法，请使用 #RRGGBB 或 #RRGGBBAA",
			})
			return
		}
	case "announcement_bar.version":
		// version 由后端按内容 hash 自动派生，前端只读。**禁止外部直接改**——
		// 否则可绕过 dismiss 逻辑给所有用户重复推送。
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "公告横幅版本号由系统自动维护，不可直接修改",
		})
		return
	case "sidebar_carousel.items":
		// 侧边栏轮播：JSON 数组字符串。校验长度 ≤ 5、字段格式合法
		if err := operation_setting.ValidateSidebarCarouselItems(option.Value.(string)); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "sidebar_carousel.version":
		// 同 announcement_bar.version：版本号由后端 hash 派生，前端只读
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "侧边栏轮播版本号由系统自动维护，不可直接修改",
		})
		return
	}
	err = model.UpdateOption(option.Key, option.Value.(string))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// announcement_bar.* 任一可见字段保存成功后自动重算 version 并落库；
	// 前端依据 version 判断是否需要再次给已 dismiss 的用户推送横幅。
	// 不放在前面 case 里，因为重算依赖 model.UpdateOption 已把新值写入
	// in-memory 配置（handleConfigUpdate）。
	if strings.HasPrefix(option.Key, "announcement_bar.") && option.Key != "announcement_bar.version" {
		newVer := operation_setting.ComputeAnnouncementBarVersion()
		// 写 announcement_bar.version：会再走一次 handleConfigUpdate 把
		// in-memory 字段同步刷新。该 key 上面 case 拒绝外部直改，但 model
		// 层不区分调用方，仍可写——这里就是唯一合法写入点
		_ = model.UpdateOption("announcement_bar.version", newVer)
	}

	// sidebar_carousel.* 同款 version 维护：admin 任一可见字段变更后自动 bump，
	// 让已 dismiss 的用户重新看到新内容
	if strings.HasPrefix(option.Key, "sidebar_carousel.") && option.Key != "sidebar_carousel.version" {
		newVer := operation_setting.ComputeSidebarCarouselVersion()
		_ = model.UpdateOption("sidebar_carousel.version", newVer)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

// isValidHexColor 检查是否为合法 CSS hex color：#RGB / #RGBA / #RRGGBB /
// #RRGGBBAA。简单正则即可，无须解析 rgb()/hsl()——后者本期不支持。
func isValidHexColor(s string) bool {
	if !strings.HasPrefix(s, "#") {
		return false
	}
	rest := s[1:]
	switch len(rest) {
	case 3, 4, 6, 8:
		// 有效长度
	default:
		return false
	}
	for _, r := range rest {
		switch {
		case r >= '0' && r <= '9',
			r >= 'a' && r <= 'f',
			r >= 'A' && r <= 'F':
			// ok
		default:
			return false
		}
	}
	return true
}
