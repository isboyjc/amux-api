package poyo

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// Adaptor 是 poyo 聚合渠道的同步图片适配器。poyo 上游是异步的
// (submit + poll),这里在一次请求内部把它伪装成站内统一的「同步返回 base64」:
// DoRequest 提交任务,DoResponse 请求内轮询直到出图,下载结果转 base64 回客户端。
// 详见 image.go 与 docs/poyo-channel-integration-plan.md。
type Adaptor struct{}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	base := info.ChannelBaseUrl
	if base == "" {
		base = constant.ChannelBaseURLs[constant.ChannelTypePoyo]
	}
	return strings.TrimRight(base, "/") + submitPath, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	req.Set("Content-Type", "application/json")
	req.Set("Accept", "application/json")
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// pickUpstreamModel 按「请求是否带图输入」在 poyo 的基础模型名与 -edit 变体间选择。
// 对外一个模型 id 同时兼文生/图生;-edit 是 poyo 的内部实现细节,不对外暴露。
// 注意:base 传入的应是 model_mapping 之后的上游名(见 ConvertImageRequest)。
func pickUpstreamModel(base string, hasImage bool) string {
	if !hasImage || strings.HasSuffix(base, "-edit") {
		return base
	}
	return base + "-edit"
}

// collectImageURLsFromRequest 从 OpenAI image / images 字段里取出 http(s) 参考图 URL。
// poyo 的 image_urls 只吃 URL;裸 base64 / 上传文件不在此托管——参考图的 R2 托管由
// 通用 presign 上传(/api/upload/presign)在调用方 / 操练场侧完成,到这里应已是 URL。
func collectImageURLsFromRequest(request *dto.ImageRequest) []string {
	var urls []string
	appendURLs := func(raw []byte) {
		if len(raw) == 0 {
			return
		}
		var single string
		if err := common.Unmarshal(raw, &single); err == nil {
			if isHTTPURL(single) {
				urls = append(urls, strings.TrimSpace(single))
			}
			return
		}
		var arr []string
		if err := common.Unmarshal(raw, &arr); err == nil {
			for _, s := range arr {
				if isHTTPURL(s) {
					urls = append(urls, strings.TrimSpace(s))
				}
			}
		}
	}
	appendURLs(request.Image)
	appendURLs(request.Images)
	return urls
}

func isHTTPURL(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, errors.New("prompt is required")
	}

	input := poyoInput{
		Prompt: request.Prompt,
		Size:   request.Size,
	}
	if request.N != nil {
		// 先挡住 uint→int 的溢出:*request.N 是 uint,直接 int() 转换时超过
		// MaxInt32 的值会变成负数,反而绕过下面的上限校验并把负数发给上游。
		if *request.N > math.MaxInt32 {
			return nil, errors.New("n is out of range")
		}
		input.N = int(*request.N)
	}

	// request.Model 此时已是 model_mapping 之后的上游名(image_handler 先跑 ModelMappedHelper)。
	// 若仍是对外 id,说明渠道漏配了 model_mapping——见 isUnmappedModel 的说明。
	if isUnmappedModel(request.Model) {
		return nil, fmt.Errorf("channel model_mapping is not configured for model %q", request.Model)
	}

	// n 超上限本地先拦,免得白跑一次提交,也免得上游报错经脱敏后
	// 变成无从下手的 "image generation failed"。
	if caps, ok := capsFor(request.Model); ok && caps.maxN > 0 && input.N > caps.maxN {
		return nil, fmt.Errorf("n must not exceed %d for this model", caps.maxN)
	}

	// extra_body 是「调用方 → 本 adapter」的私有参数通道,用来透传 poyo input
	// 的非标准字段(enable_safety_checker / image_urls 等)。不覆盖上面
	// 已按 OpenAI 标准字段填好的 prompt/size/n。
	if len(request.ExtraBody) > 0 {
		var extra poyoInput
		if err := common.Unmarshal(request.ExtraBody, &extra); err == nil {
			if extra.EnableSafetyChecker != nil {
				input.EnableSafetyChecker = extra.EnableSafetyChecker
			}
			if len(extra.ImageURLs) > 0 {
				input.ImageURLs = append(input.ImageURLs, extra.ImageURLs...)
			}
			if input.Size == "" && extra.Size != "" {
				input.Size = extra.Size
			}
		}
	}

	// 图生图:poyo 的 image_urls 只吃 URL。参考图的 R2 托管由通用 presign 上传
	// (/api/upload/presign,复用给操练场与 API key 用户)在调用方侧完成,到这里
	// 应已是 URL。adaptor 只透传请求里已经是 http(s) URL 的参考图,不做任何托管。
	input.ImageURLs = append(input.ImageURLs, collectImageURLsFromRequest(&request)...)
	if len(input.ImageURLs) > maxInputImages {
		input.ImageURLs = input.ImageURLs[:maxInputImages]
	}

	// 有任意参考图 → 走 poyo 的 -edit 变体;对外仍是同一个模型 id,文生/图生统一。
	hasImage := len(input.ImageURLs) > 0
	if info.RelayMode == relayconstant.RelayModeImagesEdits && !hasImage {
		// 上游无关的通用文案:图生图需要参考图 URL(可先经 /api/upload/presign 上传)。
		return nil, errors.New("image edit requires a reference image URL")
	}

	return poyoSubmitRequest{
		Model: pickUpstreamModel(request.Model, hasImage),
		Input: input,
	}, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	return poyoImageHandler(c, resp, info)
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

// ── 以下能力 poyo 图片渠道不支持 ─────────────────────────────────

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("not implemented")
}
