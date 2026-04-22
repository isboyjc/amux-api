package gemini

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if len(request.Contents) > 0 {
		for i, content := range request.Contents {
			if i == 0 {
				if request.Contents[0].Role == "" {
					request.Contents[0].Role = "user"
				}
			}
			for _, part := range content.Parts {
				if part.FileData != nil {
					if part.FileData.MimeType == "" && strings.Contains(part.FileData.FileUri, "www.youtube.com") {
						part.FileData.MimeType = "video/webm"
					}
				}
			}
		}
	}
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := openai.Adaptor{}
	oaiReq, err := adaptor.ConvertClaudeRequest(c, info, req)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, oaiReq.(*dto.GeneralOpenAIRequest))
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	// /v1/images/edits（含操练场 /pg/images/edits）：multipart 形态，除
	// 文本 prompt 外还会携带一至多张参考图。Gemini Nano Banana 家族上游
	// 的 :generateContent 本身就支持在 contents.parts 里混合 text 和
	// inlineData，所以这里把每张上传图转 base64 拼进 Parts 即可，响应
	// 路径仍由 GeminiNanoBananaImageHandler 统一处理。
	if info.RelayMode == constant.RelayModeImagesEdits &&
		model_setting.IsGeminiModelSupportImagine(info.UpstreamModelName) &&
		!strings.HasPrefix(info.UpstreamModelName, "imagen") {
		parts, err := collectGeminiImageParts(c, request.Prompt)
		if err != nil {
			return nil, err
		}
		cfg := dto.GeminiChatGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
		}
		applyGeminiImageExtra(&cfg, parseGeminiImageExtra(request.ExtraBody))
		return &dto.GeminiChatRequest{
			Contents: []dto.GeminiChatContent{
				{Role: "user", Parts: parts},
			},
			GenerationConfig: cfg,
		}, nil
	}

	// Nano Banana 家族（gemini-*-flash-image / gemini-*-pro-image 等）走
	// generateContent 端点而非 :predict。调用方（操练场或外部）通过
	// /v1/images/generations 打过来时，在此直接转成 chat 请求并开启
	// responseModalities=["TEXT","IMAGE"]，回应会在 DoResponse 里被
	// GeminiNanoBananaImageHandler 解析成 OpenAI 图片响应。
	if model_setting.IsGeminiModelSupportImagine(info.UpstreamModelName) &&
		!strings.HasPrefix(info.UpstreamModelName, "imagen") {
		cfg := dto.GeminiChatGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
		}
		// 从 request.ExtraBody 里读取操练场 / 外部调用方想要透传的私有参数
		// （aspect_ratio / image_size / seed / thinking_level / person_generation），
		// 合并到 generationConfig。未携带时无副作用，对公 API 契约不变。
		applyGeminiImageExtra(&cfg, parseGeminiImageExtra(request.ExtraBody))
		return &dto.GeminiChatRequest{
			Contents: []dto.GeminiChatContent{
				{
					Role: "user",
					Parts: []dto.GeminiPart{
						{Text: request.Prompt},
					},
				},
			},
			GenerationConfig: cfg,
		}, nil
	}

	if !strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return nil, errors.New("not supported model for image generation, only imagen and gemini image models are supported")
	}

	// convert size to aspect ratio but allow user to specify aspect ratio
	aspectRatio := "1:1" // default aspect ratio
	size := strings.TrimSpace(request.Size)
	if size != "" {
		if strings.Contains(size, ":") {
			aspectRatio = size
		} else {
			switch size {
			case "256x256", "512x512", "1024x1024":
				aspectRatio = "1:1"
			case "1536x1024":
				aspectRatio = "3:2"
			case "1024x1536":
				aspectRatio = "2:3"
			case "1024x1792":
				aspectRatio = "9:16"
			case "1792x1024":
				aspectRatio = "16:9"
			}
		}
	}

	// build gemini imagen request
	geminiRequest := dto.GeminiImageRequest{
		Instances: []dto.GeminiImageInstance{
			{
				Prompt: request.Prompt,
			},
		},
		Parameters: dto.GeminiImageParameters{
			SampleCount:      int(lo.FromPtrOr(request.N, uint(1))),
			AspectRatio:      aspectRatio,
			PersonGeneration: "allow_adult", // default allow adult
		},
	}

	// Set imageSize when quality parameter is specified
	// Map quality parameter to imageSize (only supported by Standard and Ultra models)
	// quality values: auto, high, medium, low (for gpt-image-1), hd, standard (for dall-e-3)
	// imageSize values: 1K (default), 2K
	// https://ai.google.dev/gemini-api/docs/imagen
	// https://platform.openai.com/docs/api-reference/images/create
	if request.Quality != "" {
		imageSize := "1K" // default
		switch request.Quality {
		case "hd", "high":
			imageSize = "2K"
		case "2K":
			imageSize = "2K"
		case "standard", "medium", "low", "auto", "1K":
			imageSize = "1K"
		default:
			// unknown quality value, default to 1K
			imageSize = "1K"
		}
		geminiRequest.Parameters.ImageSize = imageSize
	}

	return geminiRequest, nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {

}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {

	if model_setting.GetGeminiSettings().ThinkingAdapterEnabled &&
		!model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
		// 新增逻辑：处理 -thinking-<budget> 格式
		if strings.Contains(info.UpstreamModelName, "-thinking-") {
			parts := strings.Split(info.UpstreamModelName, "-thinking-")
			info.UpstreamModelName = parts[0]
		} else if strings.HasSuffix(info.UpstreamModelName, "-thinking") { // 旧的适配
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-thinking")
		} else if strings.HasSuffix(info.UpstreamModelName, "-nothinking") {
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-nothinking")
		} else if baseModel, level, ok := reasoning.TrimEffortSuffix(info.UpstreamModelName); ok && level != "" {
			info.UpstreamModelName = baseModel
		}
	}

	version := model_setting.GetGeminiVersionSetting(info.UpstreamModelName)

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return fmt.Sprintf("%s/%s/models/%s:predict", info.ChannelBaseUrl, version, info.UpstreamModelName), nil
	}

	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		action := "embedContent"
		if info.IsGeminiBatchEmbedding {
			action = "batchEmbedContents"
		}
		return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, info.UpstreamModelName, action), nil
	}

	action := "generateContent"
	if info.IsStream {
		action = "streamGenerateContent?alt=sse"
		if info.RelayMode == constant.RelayModeGemini {
			info.DisablePing = true
		}
	}
	return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, info.UpstreamModelName, action), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-goog-api-key", info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	geminiRequest, err := CovertOpenAI2Gemini(c, *request, info)
	if err != nil {
		return nil, err
	}

	return geminiRequest, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	if request.Input == nil {
		return nil, errors.New("input is required")
	}

	inputs := request.ParseInput()
	if len(inputs) == 0 {
		return nil, errors.New("input is empty")
	}
	// We always build a batch-style payload with `requests`, so ensure we call the
	// batch endpoint upstream to avoid payload/endpoint mismatches.
	info.IsGeminiBatchEmbedding = true
	// process all inputs
	geminiRequests := make([]map[string]interface{}, 0, len(inputs))
	for _, input := range inputs {
		geminiRequest := map[string]interface{}{
			"model": fmt.Sprintf("models/%s", info.UpstreamModelName),
			"content": dto.GeminiChatContent{
				Parts: []dto.GeminiPart{
					{
						Text: input,
					},
				},
			},
		}

		// set specific parameters for different models
		// https://ai.google.dev/api/embeddings?hl=zh-cn#method:-models.embedcontent
		switch info.UpstreamModelName {
		case "text-embedding-004", "gemini-embedding-exp-03-07", "gemini-embedding-001":
			// Only newer models introduced after 2024 support OutputDimensionality
			dimensions := lo.FromPtrOr(request.Dimensions, 0)
			if dimensions > 0 {
				geminiRequest["outputDimensionality"] = dimensions
			}
		}
		geminiRequests = append(geminiRequests, geminiRequest)
	}

	return map[string]interface{}{
		"requests": geminiRequests,
	}, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == constant.RelayModeGemini {
		if strings.Contains(info.RequestURLPath, ":embedContent") ||
			strings.Contains(info.RequestURLPath, ":batchEmbedContents") {
			return NativeGeminiEmbeddingHandler(c, resp, info)
		}
		if info.IsStream {
			return GeminiTextGenerationStreamHandler(c, info, resp)
		} else {
			return GeminiTextGenerationHandler(c, info, resp)
		}
	}

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return GeminiImageHandler(c, info, resp)
	}

	// Nano Banana 走 generateContent 返回 inlineData；需要把 chat 响应
	// 转成 OpenAI 图片响应（data[].b64_json）。Generations 和 Edits 两种
	// RelayMode 下上游都走同一条 :generateContent 端点，解析逻辑一致。
	if (info.RelayMode == constant.RelayModeImagesGenerations ||
		info.RelayMode == constant.RelayModeImagesEdits) &&
		model_setting.IsGeminiModelSupportImagine(info.UpstreamModelName) {
		return GeminiNanoBananaImageHandler(c, info, resp)
	}

	// check if the model is an embedding model
	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		return GeminiEmbeddingHandler(c, info, resp)
	}

	if info.IsStream {
		return GeminiChatStreamHandler(c, info, resp)
	} else {
		return GeminiChatHandler(c, info, resp)
	}

}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

// collectGeminiImageParts 从 multipart/form-data 里收集所有图像文件转成
// Gemini 的 GeminiPart 列表，前置一个 text part 承载 prompt。
//
// 和 OpenAI :edits 不同，Gemini 的 :generateContent 没有"主图 / 蒙版"的
// 位置语义——所有 inlineData part 都等价，含义由上下文（prompt 文字）
// 决定。因此这里不对 schema 里的 key 做区分，把 image / image[] /
// style_reference 等**所有图像文件字段**按字段名字典序依次加入，保证
// 渲染顺序稳定（同一字段内的多个文件保留客户端顺序）。
func collectGeminiImageParts(c *gin.Context, prompt string) ([]dto.GeminiPart, error) {
	parts := []dto.GeminiPart{{Text: prompt}}

	mf := c.Request.MultipartForm
	if mf == nil {
		if _, err := c.MultipartForm(); err != nil {
			return nil, fmt.Errorf("failed to parse multipart form: %w", err)
		}
		mf = c.Request.MultipartForm
	}
	if mf == nil || len(mf.File) == 0 {
		return parts, nil
	}

	keys := make([]string, 0, len(mf.File))
	for k := range mf.File {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		for _, fh := range mf.File[key] {
			f, err := fh.Open()
			if err != nil {
				return nil, fmt.Errorf("open image %q: %w", fh.Filename, err)
			}
			data, readErr := io.ReadAll(f)
			_ = f.Close()
			if readErr != nil {
				return nil, fmt.Errorf("read image %q: %w", fh.Filename, readErr)
			}
			parts = append(parts, dto.GeminiPart{
				InlineData: &dto.GeminiInlineData{
					MimeType: geminiDetectImageMime(fh, data),
					Data:     base64.StdEncoding.EncodeToString(data),
				},
			})
		}
	}
	return parts, nil
}

// geminiDetectImageMime 优先信任客户端声明的 Content-Type（浏览器 File API
// 会填），其次按文件后缀猜，最后退化到 http.DetectContentType 嗅探字节。
// Content-Type 可能带 "; charset=..." 后缀，这里只取 media type 部分。
func geminiDetectImageMime(fh *multipart.FileHeader, data []byte) string {
	if ct := fh.Header.Get("Content-Type"); ct != "" {
		if semi := strings.IndexByte(ct, ';'); semi >= 0 {
			ct = strings.TrimSpace(ct[:semi])
		}
		if strings.HasPrefix(ct, "image/") {
			return ct
		}
	}
	switch strings.ToLower(filepath.Ext(fh.Filename)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	}
	sniff := http.DetectContentType(data)
	if semi := strings.IndexByte(sniff, ';'); semi >= 0 {
		sniff = strings.TrimSpace(sniff[:semi])
	}
	if strings.HasPrefix(sniff, "image/") {
		return sniff
	}
	return "image/png"
}
