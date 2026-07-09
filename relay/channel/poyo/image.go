package poyo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// 轮询节奏。图不可能秒出,先等 initialPollDelay 再开始,之后每 pollInterval 一次;
// 命中 429 指数退避到 maxPollInterval 封顶。
const (
	initialPollDelay = 5 * time.Second
	pollInterval     = 3 * time.Second
	maxPollInterval  = 15 * time.Second
)

// 轮询总时长随请求张数线性缩放。poyo 是全部出图后才一次性填充 files,
// 所以耗时随 n 增长——实测 n=2 约 51s、n=4 约 78s(含提交、下载、base64)。
//
// 一刀切的固定超时两头不讨好:对 n=1 太长(失败要等很久),对 n=15 太紧。
// 注意超时的代价由我们承担——轮询超时不给用户扣费(PostTextConsumeQuota
// 不执行),但上游任务已在跑、额度已消耗。所以宁可给足余量。
// maxPollTimeout 是轮询超时的硬上界。n 的合法性只在 ConvertImageRequest 里按
// modelCaps 校验,而两条路径能绕过它:透传模式(PassThroughRequestEnabled /
// 渠道 PassThroughBodyEnabled)不走 ConvertImageRequest;model_mapping 映射到
// modelCapsTable 之外的上游名时 capsFor 返回 false 而放行。
// 没有这个上界,一个 n=10000 的请求就能让 goroutine 和连接被占住几十小时。
// 600s 覆盖当前最大合法请求(n=15 → 510s)并留有余量。
const maxPollTimeout = 600 * time.Second

const (
	pollBaseTimeout     = 60 * time.Second
	pollPerImageTimeout = 30 * time.Second
)

// 出图后的下载阶段不在轮询 deadline 之内,自己设界。
//   - downloadPhaseTimeout: 15 张图串行下载的总预算;
//   - maxTotalImageBytes: base64 后的累计上限。单张已受
//     constant.MaxFileDownloadMB(默认 64MB)限制,但 n 最大 15,
//     没有总量上限时单请求峰值内存可近 1GB。
//     实测一张 2K 图 base64 约 3~4MB,128MB 对 n=15 有充足余量。
//
// ⚠️ downloadPhaseTimeout 只在两张图**之间**检查,拦不住单张卡死:
// service.GetImageFromUrl 不接受 ctx,底层 service.GetHttpClient() 在
// RELAY_TIMEOUT=0(默认)时没有 Timeout。缓解手段是设置 RELAY_TIMEOUT;
// 根治需要给 GetImageFromUrl 加 ctx 版本,影响全站,不在本渠道范围内。
const (
	downloadPhaseTimeout = 120 * time.Second
	maxTotalImageBytes   = 128 << 20
)

// pollTimeoutFor 返回该请求的轮询超时:公式值与管理员配置值取大,再按
// maxPollTimeout 封顶。配置(model_setting.GetSyncImagePollTimeout)因此
// 退化为"下限",想给某个慢模型加码仍可在后台配,但不会让小请求陪着一起等。
func pollTimeoutFor(modelName string, n int) time.Duration {
	if n < 1 {
		n = 1
	}
	scaled := pollBaseTimeout + time.Duration(n)*pollPerImageTimeout
	if configured := model_setting.GetSyncImagePollTimeout(modelName); configured > scaled {
		scaled = configured
	}
	if scaled > maxPollTimeout {
		return maxPollTimeout
	}
	return scaled
}

// requestedImageCount 取客户端请求的出图张数,缺省 1。
func requestedImageCount(info *relaycommon.RelayInfo) int {
	if req, ok := info.Request.(*dto.ImageRequest); ok && req.N != nil && *req.N > 0 {
		return int(*req.N)
	}
	return 1
}

// 面向客户端的通用错误文案。绝不含上游身份(poyo)、直链(storage.*)或上游响应体
// ——上游脱敏铁律:这些只允许出现在服务端日志(logger.LogError),不得进入 API 响应。
const (
	clientErrMsgFailed  = "image generation failed"
	clientErrMsgTimeout = "image generation timed out"
)

// safeErr 把完整诊断(可含上游名 / 直链 / 响应体,仅内部可见)打到服务端日志,
// 对客户端只返回脱敏后的通用错误。所有 poyo 错误出口都必须走这里,避免泄露上游。
func safeErr(c *gin.Context, logDetail, clientMsg string, statusCode int, skipRetry bool) *types.NewAPIError {
	logger.LogError(c, logDetail)
	ops := make([]types.NewAPIErrorOptions, 0, 1)
	if skipRetry {
		ops = append(ops, types.ErrOptionWithSkipRetry())
	}
	return types.NewOpenAIError(errors.New(clientMsg), types.ErrorCodeBadResponse, statusCode, ops...)
}

// poyoImageHandler 是「异步转同步」的核心:读取 submit 响应拿 task_id,请求内轮询
// 直到出图,把结果 URL 下载并统一转 base64 同步回客户端。
//
// 失败分流(见 docs/poyo-channel-integration-plan.md §8):
//   - 提交阶段失败(解析失败 / code!=200):**可重试**(不加 SkipRetry),换 key/渠道可能就好;
//   - 轮询超时 / 任务失败 / 出图后下载失败:**SkipRetry**,避免重试放大耗时与重复扣费。
func poyoImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, safeErr(c, fmt.Sprintf("poyo read submit response failed: %v", err),
			clientErrMsgFailed, http.StatusInternalServerError, false)
	}
	service.CloseResponseBodyGracefully(resp)

	var submit poyoSubmitResponse
	if err := common.Unmarshal(body, &submit); err != nil {
		// 提交阶段:可重试
		return nil, safeErr(c, fmt.Sprintf("poyo unmarshal submit response failed: %v (body: %s)", err, truncateBody(body)),
			clientErrMsgFailed, http.StatusInternalServerError, false)
	}
	if submit.Code != 200 || submit.Data.TaskID == "" {
		msg := submit.Message
		if msg == "" {
			msg = "submit failed"
		}
		// 提交阶段:可重试
		return nil, safeErr(c, fmt.Sprintf("poyo submit error (code %d): %s", submit.Code, msg),
			clientErrMsgFailed, http.StatusBadGateway, false)
	}

	taskID := submit.Data.TaskID
	files, apiErr := pollPoyoTask(c, info, taskID, requestedImageCount(info))
	if apiErr != nil {
		return nil, apiErr
	}

	// 统一转 base64:忽略 response_format=url,保持站内生图返回的一致性。
	//
	// 下载阶段独立于轮询的 deadline 之外(轮询已经结束),必须自己设界:
	// 单张受 constant.MaxFileDownloadMB 限制(默认 64MB),但 n 最大 15,
	// 不设总量上限时单个请求峰值内存可近 1GB;串行下载也可能无限拖长。
	ctx := c.Request.Context()
	downloadDeadline := time.Now().Add(downloadPhaseTimeout)
	totalBytes := 0

	imageResp := dto.ImageResponse{Created: info.StartTime.Unix()}
	for _, f := range files {
		if f.FileURL == "" {
			continue
		}
		if ctx.Err() != nil {
			return nil, canceledErr()
		}
		if time.Now().After(downloadDeadline) {
			return nil, safeErr(c, fmt.Sprintf("poyo image download exceeded %s, task_id=%s, downloaded %d/%d",
				downloadPhaseTimeout, taskID, len(imageResp.Data), len(files)),
				clientErrMsgFailed, http.StatusGatewayTimeout, true)
		}
		_, b64, derr := service.GetImageFromUrl(f.FileURL)
		if derr != nil {
			// 上游已出图但下载失败:SkipRetry(重试=重新提交+再扣费)。上游 URL 只进日志。
			return nil, safeErr(c, fmt.Sprintf("poyo image download failed, task_id=%s url=%s: %v", taskID, f.FileURL, derr),
				clientErrMsgFailed, http.StatusBadGateway, true)
		}
		totalBytes += len(b64)
		if totalBytes > maxTotalImageBytes {
			return nil, safeErr(c, fmt.Sprintf("poyo images exceed total size cap %d bytes, task_id=%s", maxTotalImageBytes, taskID),
				clientErrMsgFailed, http.StatusBadGateway, true)
		}
		imageResp.Data = append(imageResp.Data, dto.ImageData{B64Json: b64})
	}
	if len(imageResp.Data) == 0 {
		return nil, safeErr(c, fmt.Sprintf("poyo returned no image files, task_id=%s", taskID),
			clientErrMsgFailed, http.StatusBadGateway, true)
	}

	// 按**实际出图数**计费,而非请求的 n。上游可能因安全审核等原因少出图,
	// 此时按 n 收费就是超收。image_handler 只在 adaptor 没设过时才用请求的 n
	// 兜底(见 relay/image_handler.go 的 hasN 判断),这里抢先设成真实张数。
	// 仅按次计价时设置:倍率计价下 OtherRatios 会连 token 一起乘,不该叠 n。
	if info.PriceData.UsePrice {
		info.PriceData.AddOtherRatio("n", float64(len(imageResp.Data)))
	}

	jsonResp, err := common.Marshal(imageResp)
	if err != nil {
		return nil, safeErr(c, fmt.Sprintf("poyo marshal image response failed: %v", err),
			clientErrMsgFailed, http.StatusInternalServerError, true)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(jsonResp)

	return estimateUsage(info, imageResp), nil
}

// estimateUsage 生成用于**日志展示**的 token 数。上游既不返回 prompt token
// 也不返回 output token,两者都由本地计算:
//   - 输入:prompt 的实际分词数;
//   - 输出:按 size 估算的单图 token × 实际出图张数(见 token_estimate.go)。
//
// ⚠️ 只在按次计价(UsePrice)时给出估算值。倍率计价分支会拿 token 乘倍率算钱
// (service/text_quota.go 的 !UsePrice 分支),阶梯计费的 TryTieredSettle 同理——
// 一张 2K 图的估算输出就有 5476 token,模型一旦被误配成倍率计价,账单会放大
// 几千倍。此时退回 1/0:日志难看,但绝不会因为一个展示用的数字去多收用户的钱。
func estimateUsage(info *relaycommon.RelayInfo, imageResp dto.ImageResponse) *dto.Usage {
	if !info.PriceData.UsePrice {
		return &dto.Usage{PromptTokens: 1, TotalTokens: 1}
	}

	imageReq, _ := info.Request.(*dto.ImageRequest)

	promptTokens := 0
	sizeParam := ""
	if imageReq != nil {
		promptTokens = service.CountTextToken(imageReq.Prompt, info.OriginModelName)
		sizeParam = imageReq.Size
	}

	width, height := parseImageSize(sizeParam)
	completionTokens := estimateImageTokens(width, height) * len(imageResp.Data)

	// summary.TotalTokens == 0 会让 PostTextConsumeQuota 判定「上游没返回计费信息」
	// 而跳过扣费(text_quota.go:301)。空 prompt + 估算为 0 时兜底,避免漏扣。
	if promptTokens+completionTokens == 0 {
		promptTokens = 1
	}

	return &dto.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
}

// pollPoyoTask 在超时窗内轮询 poyo 任务状态,返回结果文件列表或分类错误。
// 超时窗随 n 缩放,见 pollTimeoutFor。
func pollPoyoTask(c *gin.Context, info *relaycommon.RelayInfo, taskID string, n int) ([]poyoFile, *types.NewAPIError) {
	ctx := c.Request.Context()
	timeout := pollTimeoutFor(info.OriginModelName, n)
	deadline := time.Now().Add(timeout)

	client, err := pollClient(info)
	if err != nil {
		return nil, safeErr(c, fmt.Sprintf("poyo build poll client failed: %v", err),
			clientErrMsgFailed, http.StatusInternalServerError, false)
	}

	base := info.ChannelBaseUrl
	if base == "" {
		base = constant.ChannelBaseURLs[constant.ChannelTypePoyo]
	}
	statusURL := strings.TrimRight(base, "/") + fmt.Sprintf(statusPathFmt, taskID)

	interval := pollInterval
	var lastCode int
	var lastBody string

	// 首轮延迟(封顶到超时,避免 timeout 很短时白等)。
	if !sleepCtx(ctx, minDuration(initialPollDelay, timeout)) {
		return nil, canceledErr()
	}

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
		if err != nil {
			return nil, safeErr(c, fmt.Sprintf("poyo build poll request failed: %v", err),
				clientErrMsgFailed, http.StatusInternalServerError, true)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+info.ApiKey)

		httpResp, err := client.Do(req)
		if err != nil {
			// 网络抖动:退避后在超时窗内重试。
			lastCode, lastBody = 0, err.Error()
			if !sleepCtx(ctx, interval) {
				return nil, canceledErr()
			}
			continue
		}
		statusBody, _ := io.ReadAll(httpResp.Body)
		code := httpResp.StatusCode
		_ = httpResp.Body.Close()
		lastCode, lastBody = code, truncateBody(statusBody)

		if common.DebugEnabled {
			logger.LogInfo(c, fmt.Sprintf("poyo poll task_id=%s http=%d body=%s", taskID, code, lastBody))
		}

		if code == http.StatusTooManyRequests {
			// 限速:指数退避,绝不当任务失败。
			interval = backoff(interval)
			if !sleepCtx(ctx, interval) {
				return nil, canceledErr()
			}
			continue
		}

		var st poyoStatusResponse
		if err := common.Unmarshal(statusBody, &st); err != nil {
			if !sleepCtx(ctx, interval) {
				return nil, canceledErr()
			}
			continue
		}

		// 完成判定以「是否已有结果文件」为准,不死依赖 status 字符串
		// (聚合器完成态命名可能是 finished/success/completed 等,或结构略有出入)。
		if firstFileURL(st.Data.Files) != "" {
			return st.Data.Files, nil
		}
		if st.Data.Status == statusFailed {
			reason := st.Data.ErrorMessage
			if reason == "" {
				reason = st.Message
			}
			// 上游失败原因只进日志,客户端只看通用文案。
			return nil, safeErr(c, fmt.Sprintf("poyo task failed, task_id=%s: %s", taskID, reason),
				clientErrMsgFailed, http.StatusBadGateway, true)
		}
		// not_started / running / finished-无文件 / 未知:继续等。
		if !sleepCtx(ctx, interval) {
			return nil, canceledErr()
		}
	}

	// 超时:SkipRetry。完整诊断(HTTP 码 + 上游最后一次响应体)只打服务端日志,
	// 用于定位「状态字段/结构不符」——出图上游已完成却判不出完成时看这里;
	// 客户端只看脱敏后的通用超时文案,绝不泄露上游响应。
	return nil, safeErr(c,
		fmt.Sprintf("poyo task poll timeout after %s, task_id=%s (last http=%d, last body: %s)", timeout, taskID, lastCode, lastBody),
		clientErrMsgTimeout, http.StatusGatewayTimeout, true)
}

// firstFileURL 返回第一个非空 file_url。
func firstFileURL(files []poyoFile) string {
	for _, f := range files {
		if strings.TrimSpace(f.FileURL) != "" {
			return f.FileURL
		}
	}
	return ""
}

func pollClient(info *relaycommon.RelayInfo) (*http.Client, error) {
	if info.ChannelSetting.Proxy != "" {
		return service.NewProxyHttpClient(info.ChannelSetting.Proxy)
	}
	return service.GetHttpClient(), nil
}

func backoff(d time.Duration) time.Duration {
	d *= 2
	if d > maxPollInterval {
		return maxPollInterval
	}
	return d
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// sleepCtx 等待 d,期间若请求上下文取消(客户端断开)立即返回 false。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func canceledErr() *types.NewAPIError {
	return types.NewError(errors.New("client canceled during poll"),
		types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
}

func truncateBody(b []byte) string {
	const max = 512
	if len(b) > max {
		return string(b[:max])
	}
	return string(b)
}
