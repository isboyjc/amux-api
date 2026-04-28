package minimax

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// MiniMax 图像生成接口（t2i 与 i2i 共用 /v1/image_generation）。
// i2i 不是通用图编辑，而是"角色一致性参考"：把人物参考图放进 subject_reference，
// 上游按 prompt 重新生成保持该角色的新图。仅 image-01 / image-01-live 支持。
type MiniMaxImageRequest struct {
	Model            string             `json:"model"`
	Prompt           string             `json:"prompt"`
	AspectRatio      string             `json:"aspect_ratio,omitempty"`
	ResponseFormat   string             `json:"response_format,omitempty"`
	N                int                `json:"n,omitempty"`
	Seed             *int64             `json:"seed,omitempty"`
	PromptOptimizer  *bool              `json:"prompt_optimizer,omitempty"`
	AigcWatermark    *bool              `json:"aigc_watermark,omitempty"`
	Style            *MiniMaxStyle      `json:"style,omitempty"`
	SubjectReference []SubjectReference `json:"subject_reference,omitempty"`
}

// MiniMaxStyle 画风设置，仅 image-01-live 生效。
// 调用方 schema 把 style.style_type / style.style_weight 平铺成顶层
// style_type / style_weight，由 oaiImage2MiniMaxImageRequest 在这里组装回嵌套对象。
type MiniMaxStyle struct {
	StyleType   string   `json:"style_type,omitempty"`
	StyleWeight *float64 `json:"style_weight,omitempty"`
}

// SubjectReference 当前 MiniMax 仅支持 type=character + 单张人物图。
// 注意 image_file 是单个字符串（公网 URL 或 base64 Data URL），不是数组。
type SubjectReference struct {
	Type      string `json:"type"`
	ImageFile string `json:"image_file"`
}

type MiniMaxImageResponse struct {
	ID   string `json:"id"`
	Data struct {
		ImageURLs   []string `json:"image_urls"`
		ImageBase64 []string `json:"image_base64"`
	} `json:"data"`
	Metadata map[string]any `json:"metadata"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

func oaiImage2MiniMaxImageRequest(request dto.ImageRequest) MiniMaxImageRequest {
	responseFormat := normalizeMiniMaxResponseFormat(request.ResponseFormat)
	minimaxRequest := MiniMaxImageRequest{
		Model:          request.Model,
		Prompt:         request.Prompt,
		ResponseFormat: responseFormat,
		N:              1,
		AigcWatermark:  request.Watermark,
	}

	if request.Model == "" {
		minimaxRequest.Model = "image-01"
	}
	if request.N != nil && *request.N > 0 {
		minimaxRequest.N = int(*request.N)
	}
	if aspectRatio := aspectRatioFromImageRequest(request); aspectRatio != "" {
		minimaxRequest.AspectRatio = aspectRatio
	}
	var promptOptimizer bool
	if lookupImageExtra(request, "prompt_optimizer", &promptOptimizer) {
		minimaxRequest.PromptOptimizer = &promptOptimizer
	}
	var seed int64
	if lookupImageExtra(request, "seed", &seed) {
		minimaxRequest.Seed = &seed
	}
	if style := extractStyle(request); style != nil {
		minimaxRequest.Style = style
	}
	if sr := extractSubjectReference(request); len(sr) > 0 {
		minimaxRequest.SubjectReference = sr
	}

	return minimaxRequest
}

// lookupImageExtra 在 ImageRequest 的两个"非声明字段通道"里查 key：
//  1. 顶层未声明字段（落进 dto.ImageRequest.Extra，调用方直接平铺写时用这个）
//  2. extra_body 嵌套对象（OpenAI SDK 通过 extra_body 透传时用这个；Playground 的
//     useImageGeneration 也走这条）
//
// 任一路径解析成功即返回 true，dst 必须是可寻址指针。
func lookupImageExtra(request dto.ImageRequest, key string, dst any) bool {
	if raw, ok := request.Extra[key]; ok {
		if err := common.Unmarshal(raw, dst); err == nil {
			return true
		}
	}
	if len(request.ExtraBody) > 0 {
		var m map[string]json.RawMessage
		if err := common.Unmarshal(request.ExtraBody, &m); err == nil {
			if raw, ok := m[key]; ok {
				if err := common.Unmarshal(raw, dst); err == nil {
					return true
				}
			}
		}
	}
	return false
}

// extractStyle 把调用方平铺写的 style_type / style_weight 组装回 MiniMax 期望
// 的嵌套 style 对象。同时兼容直接整体透传 style: {...} 的写法。
//
// 仅 image-01-live 生效——image-01 收到 style 字段会被上游忽略，所以这里不做
// 模型名校验，发出去也不会出错。
func extractStyle(request dto.ImageRequest) *MiniMaxStyle {
	// 1) 整体透传 style: {style_type, style_weight}
	var nested MiniMaxStyle
	if lookupImageExtra(request, "style", &nested) &&
		(nested.StyleType != "" || nested.StyleWeight != nil) {
		return &nested
	}

	// 2) 平铺写法：从 style_type / style_weight 各自读
	out := &MiniMaxStyle{}
	hit := false
	var styleType string
	if lookupImageExtra(request, "style_type", &styleType) && styleType != "" {
		out.StyleType = styleType
		hit = true
	}
	var styleWeight float64
	if lookupImageExtra(request, "style_weight", &styleWeight) {
		out.StyleWeight = &styleWeight
		hit = true
	}
	if !hit {
		return nil
	}
	return out
}

// extractSubjectReference 兼容两种调用姿势：顶层 subject_reference（Extra）
// 与 extra_body.subject_reference（OpenAI SDK / Playground 的私有参数通道）。
//
// 同时兼容历史/民间写法 image_file 写成数组（["data:..."]）的情况——MiniMax
// 官方现在要求 string，但很多博客示例用的是数组形态，这里统一拍扁成单值。
func extractSubjectReference(request dto.ImageRequest) []SubjectReference {
	var raw []rawSubjectReference
	if !lookupImageExtra(request, "subject_reference", &raw) || len(raw) == 0 {
		return nil
	}
	out := make([]SubjectReference, 0, len(raw))
	for _, r := range raw {
		out = append(out, SubjectReference{
			Type:      r.Type,
			ImageFile: r.PickImageFile(),
		})
	}
	return out
}

// rawSubjectReference 用 RawMessage 接 image_file，再决定按 string 还是 array
// 拆出来，以便兼容两种历史写法。
type rawSubjectReference struct {
	Type      string          `json:"type"`
	ImageFile json.RawMessage `json:"image_file"`
}

// PickImageFile 优先按字符串解，失败再按数组解（取首个非空元素）。
func (r rawSubjectReference) PickImageFile() string {
	if len(r.ImageFile) == 0 {
		return ""
	}
	var s string
	if err := common.Unmarshal(r.ImageFile, &s); err == nil {
		return s
	}
	var arr []string
	if err := common.Unmarshal(r.ImageFile, &arr); err == nil {
		for _, v := range arr {
			if v != "" {
				return v
			}
		}
	}
	return ""
}

// IsImage01Family i2i 仅 image-01 系支持，调用方传入的 model 应是已经过映射的上游名。
func IsImage01Family(model string) bool {
	m := strings.ToLower(model)
	return m == "image-01" || m == "image-01-live"
}

// oaiEdit2MiniMaxImageRequest 把 OpenAI /v1/images/edits 的 multipart 请求转成 MiniMax i2i 调用。
// 取 multipart 中的 image 文件 → base64 dataURL → 填到 subject_reference[0]，
// 其它非文件字段（prompt/n/size/response_format/...）已经被上层解析进 dto.ImageRequest。
func oaiEdit2MiniMaxImageRequest(c *gin.Context, request dto.ImageRequest) (MiniMaxImageRequest, error) {
	base := oaiImage2MiniMaxImageRequest(request)

	// 已有 subject_reference（透传姿势）：直接用，不再读 multipart
	if len(base.SubjectReference) > 0 {
		return base, nil
	}

	mf := c.Request.MultipartForm
	if mf == nil {
		if _, err := c.MultipartForm(); err != nil {
			return MiniMaxImageRequest{}, errors.New("failed to parse multipart form")
		}
		mf = c.Request.MultipartForm
	}
	if mf == nil || mf.File == nil {
		return MiniMaxImageRequest{}, errors.New("image is required")
	}

	var imageFiles []*multipart.FileHeader
	if files, ok := mf.File["image"]; ok && len(files) > 0 {
		imageFiles = files
	} else if files, ok := mf.File["image[]"]; ok && len(files) > 0 {
		imageFiles = files
	} else {
		for fieldName, files := range mf.File {
			if strings.HasPrefix(fieldName, "image[") && len(files) > 0 {
				imageFiles = append(imageFiles, files...)
				break
			}
		}
	}
	if len(imageFiles) == 0 {
		return MiniMaxImageRequest{}, errors.New("image is required")
	}

	// MiniMax i2i 当前仅消费第一张参考图（character 类型只接受单张人物图）
	dataURL, err := fileHeaderToDataURL(imageFiles[0])
	if err != nil {
		return MiniMaxImageRequest{}, err
	}
	base.SubjectReference = []SubjectReference{
		{Type: "character", ImageFile: dataURL},
	}
	return base, nil
}

func fileHeaderToDataURL(fh *multipart.FileHeader) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("open image file: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("read image file: %w", err)
	}
	mimeType := fh.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)), nil
}

func aspectRatioFromImageRequest(request dto.ImageRequest) string {
	var aspectRatio string
	if lookupImageExtra(request, "aspect_ratio", &aspectRatio) && aspectRatio != "" {
		return aspectRatio
	}

	switch request.Size {
	case "1024x1024":
		return "1:1"
	case "1792x1024":
		return "16:9"
	case "1024x1792":
		return "9:16"
	case "1536x1024", "1248x832":
		return "3:2"
	case "1024x1536", "832x1248":
		return "2:3"
	case "1152x864":
		return "4:3"
	case "864x1152":
		return "3:4"
	case "1344x576":
		return "21:9"
	}

	width, height, ok := parseImageSize(request.Size)
	if !ok {
		return ""
	}
	ratio := reduceAspectRatio(width, height)
	switch ratio {
	case "1:1", "16:9", "4:3", "3:2", "2:3", "3:4", "9:16", "21:9":
		return ratio
	default:
		return ""
	}
}

func parseImageSize(size string) (int, int, bool) {
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func reduceAspectRatio(width, height int) string {
	divisor := gcd(width, height)
	return fmt.Sprintf("%d:%d", width/divisor, height/divisor)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

func normalizeMiniMaxResponseFormat(responseFormat string) string {
	switch strings.ToLower(responseFormat) {
	case "", "url":
		return "url"
	case "b64_json", "base64":
		return "base64"
	default:
		return responseFormat
	}
}

func responseMiniMax2OpenAIImage(response *MiniMaxImageResponse, info *relaycommon.RelayInfo) (*dto.ImageResponse, error) {
	imageResponse := &dto.ImageResponse{
		Created: info.StartTime.Unix(),
	}

	for _, imageURL := range response.Data.ImageURLs {
		imageResponse.Data = append(imageResponse.Data, dto.ImageData{Url: imageURL})
	}
	for _, imageBase64 := range response.Data.ImageBase64 {
		imageResponse.Data = append(imageResponse.Data, dto.ImageData{B64Json: imageBase64})
	}
	if len(response.Metadata) > 0 {
		metadata, err := common.Marshal(response.Metadata)
		if err != nil {
			return nil, err
		}
		imageResponse.Metadata = metadata
	}

	return imageResponse, nil
}

func miniMaxImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	var minimaxResponse MiniMaxImageResponse
	if err := common.Unmarshal(responseBody, &minimaxResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if minimaxResponse.BaseResp.StatusCode != 0 {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: minimaxResponse.BaseResp.StatusMsg,
			Type:    "minimax_image_error",
			Code:    fmt.Sprintf("%d", minimaxResponse.BaseResp.StatusCode),
		}, resp.StatusCode)
	}

	openAIResponse, err := responseMiniMax2OpenAIImage(&minimaxResponse, info)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	jsonResponse, err := common.Marshal(openAIResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err := c.Writer.Write(jsonResponse); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	return &dto.Usage{}, nil
}
