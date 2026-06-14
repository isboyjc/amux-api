package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/router"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/events"
	_ "github.com/QuantumNous/new-api/service/events/subscribers/logger"    // 触发 init 注册 logger 订阅者
	_ "github.com/QuantumNous/new-api/service/events/subscribers/marketing" // 触发 init 注册 marketing 订阅者
	"github.com/QuantumNous/new-api/service/marketing"
	resendprovider "github.com/QuantumNous/new-api/service/marketing/providers/resend"
	"github.com/QuantumNous/new-api/service/ticket"
	_ "github.com/QuantumNous/new-api/setting/performance_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/google/uuid"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	_ "net/http/pprof"
)

//go:embed web/dist
var buildFS embed.FS

//go:embed web/dist/index.html
var indexPage []byte

func main() {
	startTime := time.Now()

	err := InitResources()
	if err != nil {
		common.FatalLog("failed to initialize resources: " + err.Error())
		return
	}

	common.SysLog("Amux API " + common.Version + " started")
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	if common.DebugEnabled {
		common.SysLog("running in debug mode")
	}

	defer func() {
		err := model.CloseDB()
		if err != nil {
			common.FatalLog("failed to close database: " + err.Error())
		}
	}()

	if common.RedisEnabled {
		// for compatibility with old versions
		common.MemoryCacheEnabled = true
	}
	if common.MemoryCacheEnabled {
		common.SysLog("memory cache enabled")
		common.SysLog(fmt.Sprintf("sync frequency: %d seconds", common.SyncFrequency))

		// Add panic recovery and retry for InitChannelCache
		func() {
			defer func() {
				if r := recover(); r != nil {
					common.SysLog(fmt.Sprintf("InitChannelCache panic: %v, retrying once", r))
					// Retry once
					_, _, fixErr := model.FixAbility()
					if fixErr != nil {
						common.FatalLog(fmt.Sprintf("InitChannelCache failed: %s", fixErr.Error()))
					}
				}
			}()
			model.InitChannelCache()
		}()

		go model.SyncChannelCache(common.SyncFrequency)
	}

	// 热更新配置
	go model.SyncOptions(common.SyncFrequency)

	// 数据看板
	go model.UpdateQuotaData()

	// 首页公开 Token 统计（每 5 分钟刷新）
	go controller.SyncPublicTokenStats(300)

	if os.Getenv("CHANNEL_UPDATE_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_UPDATE_FREQUENCY"))
		if err != nil {
			common.FatalLog("failed to parse CHANNEL_UPDATE_FREQUENCY: " + err.Error())
		}
		go controller.AutomaticallyUpdateChannels(frequency)
	}

	go controller.AutomaticallyTestChannels()

	// Codex credential auto-refresh check every 10 minutes, refresh when expires within 1 day
	service.StartCodexCredentialAutoRefreshTask()

	// Subscription quota reset task (daily/weekly/monthly/custom)
	service.StartSubscriptionQuotaResetTask()

	// 事件总线 worker（所有实例都跑，靠 DB 乐观 claim 互斥）+ 每日清理任务（仅 master）
	if operation_setting.EventWorkerEnabled {
		workerOpts := events.WorkerOpts{
			PollInterval:  time.Duration(operation_setting.EventWorkerPollIntervalMs) * time.Millisecond,
			BatchSize:     operation_setting.EventWorkerBatchSize,
			Concurrency:   operation_setting.EventWorkerConcurrency,
			HandleTimeout: time.Duration(operation_setting.EventHandleTimeoutMs) * time.Millisecond,
			WorkerId:      uuid.New().String(),
		}
		gopool.Go(func() {
			events.StartWorker(context.Background(), workerOpts)
		})
	}
	service.StartEventCleanupTask()

	// 营销 Provider 接线：注册"配置变更回调"，配置载入完成后跑一次完成初始装配。
	// 此后 admin 在后台改 MarketingEnabled/ResendAPIKey/Segment ID 等都会经
	// model/option.go updateOptionMap → TriggerMarketingReload → 触发本回调重建 Provider。
	operation_setting.OnMarketingConfigChanged = rebuildMarketingProvider
	rebuildMarketingProvider()

	// 邀请关系羊毛风控后台 worker：消费 affiliate_risk_dirty 表，按批重算缓存。
	// 单进程一个 worker；多实例部署时各实例并行跑，由 UpsertAffiliateRiskCache 的
	// ON CONFLICT 保证幂等。开关由 operation_setting.AffiliateRiskCacheEnabled 控制，
	// 关闭时 worker 进入长 sleep，业务零影响。
	gopool.Go(func() {
		marketing.RunAffiliateRiskWorker(context.Background())
	})

	// Desktop auth session cleanup task
	service.StartDesktopAuthCleanupTask()

	// Ticket: auto-resolve inactive tickets after configured days
	ticket.StartTicketAutoResolveTask()

	// Wire task polling adaptor factory (breaks service -> relay import cycle)
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		a := relay.GetTaskAdaptor(platform)
		if a == nil {
			return nil
		}
		return a
	}

	// Channel upstream model update check task
	controller.StartChannelUpstreamModelUpdateTask()

	if common.IsMasterNode && constant.UpdateTask {
		// 视频结果 R2 归档 worker 池：把大视频的下载+上传从轮询循环里搬出来，
		// 避免阻塞轮询。仅主节点启动（轮询本身也只在主节点跑）。
		service.StartVideoArchiveWorkers(0)
		gopool.Go(func() {
			controller.UpdateMidjourneyTaskBulk()
		})
		gopool.Go(func() {
			controller.UpdateTaskBulk()
		})
	}
	if os.Getenv("BATCH_UPDATE_ENABLED") == "true" {
		common.BatchUpdateEnabled = true
		common.SysLog("batch update enabled with interval " + strconv.Itoa(common.BatchUpdateInterval) + "s")
		model.InitBatchUpdater()
	}

	if os.Getenv("ENABLE_PPROF") == "true" {
		gopool.Go(func() {
			log.Println(http.ListenAndServe("0.0.0.0:8005", nil))
		})
		go common.Monitor()
		common.SysLog("pprof enabled")
	}

	err = common.StartPyroScope()
	if err != nil {
		common.SysError(fmt.Sprintf("start pyroscope error : %v", err))
	}

	// Initialize HTTP server
	server := gin.New()
	server.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		common.SysLog(fmt.Sprintf("panic detected: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Panic detected, error: %v. Please submit a issue here: https://github.com/Calcium-Ion/new-api", err),
				"type":    "new_api_panic",
			},
		})
	}))
	// This will cause SSE not to work!!!
	//server.Use(gzip.Gzip(gzip.DefaultCompression))
	server.Use(middleware.RequestId())
	server.Use(middleware.PoweredBy())
	server.Use(middleware.I18n())
	middleware.SetUpLogger(server)
	// Initialize session store
	store := cookie.NewStore([]byte(common.SessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   2592000, // 30 days
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	})
	server.Use(sessions.Sessions("session", store))

	InjectUmamiAnalytics()
	InjectGoogleAnalytics()

	// 设置路由
	router.SetRouter(server, buildFS, indexPage)
	var port = os.Getenv("PORT")
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}

	// Log startup success message
	common.LogStartupSuccess(startTime, port)

	err = server.Run(":" + port)
	if err != nil {
		common.FatalLog("failed to start HTTP server: " + err.Error())
	}
}

// rebuildMarketingProvider 根据当前 operation_setting 重建 marketing.Provider 并注入。
// 在启动时调用一次；之后由 OnMarketingConfigChanged 钩子在配置变更时调用。
//
// 行为：
//   - MarketingEnabled=false / APIKey 为空 → 显式 SetProvider(nil)，订阅者变 no-op
//   - 构造新 Provider 成功 → SetProvider(newP) 切换
//   - 构造失败（不应发生，留为防御性兜底）→ **保留现有 Provider 不动**，仅记日志
func rebuildMarketingProvider() {
	if !operation_setting.MarketingEnabled {
		marketing.SetProvider(nil)
		return
	}
	switch operation_setting.MarketingProvider {
	case "", "resend":
		if operation_setting.ResendAPIKey == "" {
			common.SysLog("[marketing] enabled but ResendAPIKey empty; provider not built")
			marketing.SetProvider(nil)
			return
		}
		p, err := resendprovider.New(resendprovider.Config{
			APIKey:          operation_setting.ResendAPIKey,
			DefaultSegment:  operation_setting.ResendDefaultSegmentID,
			VIPSegment:      operation_setting.ResendVIPSegmentID,
			DefaultTopicIDs: splitTopicIDs(operation_setting.ResendDefaultTopicIDs),
		})
		if err != nil {
			common.SysError("[marketing] failed to build resend provider, keeping existing: " + err.Error())
			return
		}
		marketing.SetProvider(p)
		common.SysLog("[marketing] resend provider activated")
	default:
		common.SysError("[marketing] unknown provider, keeping existing: " + operation_setting.MarketingProvider)
	}
}

func splitTopicIDs(raw string) []string {
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

func InjectUmamiAnalytics() {
	analyticsInjectBuilder := &strings.Builder{}
	if os.Getenv("UMAMI_WEBSITE_ID") != "" {
		umamiSiteID := os.Getenv("UMAMI_WEBSITE_ID")
		umamiScriptURL := os.Getenv("UMAMI_SCRIPT_URL")
		if umamiScriptURL == "" {
			umamiScriptURL = "https://analytics.umami.is/script.js"
		}
		analyticsInjectBuilder.WriteString("<script defer src=\"")
		analyticsInjectBuilder.WriteString(umamiScriptURL)
		analyticsInjectBuilder.WriteString("\" data-website-id=\"")
		analyticsInjectBuilder.WriteString(umamiSiteID)
		analyticsInjectBuilder.WriteString("\"></script>")
	}
	analyticsInjectBuilder.WriteString("<!--Umami QuantumNous-->\n")
	analyticsInject := analyticsInjectBuilder.String()
	indexPage = bytes.ReplaceAll(indexPage, []byte("<!--umami-->\n"), []byte(analyticsInject))
}

func InjectGoogleAnalytics() {
	analyticsInjectBuilder := &strings.Builder{}
	if os.Getenv("GOOGLE_ANALYTICS_ID") != "" {
		gaID := os.Getenv("GOOGLE_ANALYTICS_ID")
		// Google Analytics 4 (gtag.js)
		analyticsInjectBuilder.WriteString("<script async src=\"https://www.googletagmanager.com/gtag/js?id=")
		analyticsInjectBuilder.WriteString(gaID)
		analyticsInjectBuilder.WriteString("\"></script>")
		analyticsInjectBuilder.WriteString("<script>")
		analyticsInjectBuilder.WriteString("window.dataLayer = window.dataLayer || [];")
		analyticsInjectBuilder.WriteString("function gtag(){dataLayer.push(arguments);}")
		analyticsInjectBuilder.WriteString("gtag('js', new Date());")
		analyticsInjectBuilder.WriteString("gtag('config', '")
		analyticsInjectBuilder.WriteString(gaID)
		analyticsInjectBuilder.WriteString("');")
		analyticsInjectBuilder.WriteString("</script>")
	}
	analyticsInjectBuilder.WriteString("<!--Google Analytics QuantumNous-->\n")
	analyticsInject := analyticsInjectBuilder.String()
	indexPage = bytes.ReplaceAll(indexPage, []byte("<!--Google Analytics-->\n"), []byte(analyticsInject))
}

func InitResources() error {
	// Initialize resources here if needed
	// This is a placeholder function for future resource initialization
	err := godotenv.Load(".env")
	if err != nil {
		if common.DebugEnabled {
			common.SysLog("No .env file found, using default environment variables. If needed, please create a .env file and set the relevant variables.")
		}
	}

	// 加载环境变量
	common.InitEnv()

	logger.SetupLogger()

	// Initialize model settings
	ratio_setting.InitRatioSettings()

	service.InitHttpClient()

	service.InitTokenEncoders()

	// Initialize SQL Database
	err = model.InitDB()
	if err != nil {
		common.FatalLog("failed to initialize database: " + err.Error())
		return err
	}

	// 事件总线表自迁移（与主 DB 共用一个连接，独立于 model.AutoMigrate 流程）
	events.SetDB(model.DB)
	if err := events.AutoMigrate(); err != nil {
		common.FatalLog("failed to migrate event tables: " + err.Error())
		return err
	}

	model.CheckSetup()

	// Initialize options, should after model.InitDB()
	model.InitOptionMap()

	// 清理旧的磁盘缓存文件
	common.CleanupOldCacheFiles()

	// 初始化模型
	model.GetPricing()

	// Initialize SQL Database
	err = model.InitLogDB()
	if err != nil {
		return err
	}
	// 一次性迁移：从 logs 回填 quota_data 的 token 拆分列（后台执行，不阻塞启动）
	// 必须在 InitLogDB() 之后启动，确保 LOG_DB 已初始化
	go model.MigrateQuotaDataTokenSplit()

	// 启动 access_token 定时清理：每天扫一次硬过期 + 空闲过期（默认 90 天）
	model.StartUserAccessTokenJanitor()

	// Initialize Redis
	err = common.InitRedisClient()
	if err != nil {
		return err
	}

	// 启动系统监控
	common.StartSystemMonitor()

	// Initialize i18n
	err = i18n.Init()
	if err != nil {
		common.SysError("failed to initialize i18n: " + err.Error())
		// Don't return error, i18n is not critical
	} else {
		common.SysLog("i18n initialized with languages: " + strings.Join(i18n.SupportedLanguages(), ", "))
	}
	// Register user language loader for lazy loading
	i18n.SetUserLangLoader(model.GetUserLanguage)

	// Load custom OAuth providers from database
	err = oauth.LoadCustomProviders()
	if err != nil {
		common.SysError("failed to load custom OAuth providers: " + err.Error())
		// Don't return error, custom OAuth is not critical
	}

	return nil
}
