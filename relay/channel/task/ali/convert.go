package ali

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

type aliVideoModelKind int

const (
	aliVideoModelLegacy aliVideoModelKind = iota
	aliVideoModelHappyHorseT2V
	aliVideoModelHappyHorseI2V
	aliVideoModelHappyHorseR2V
	aliVideoModelHappyHorseEdit
	aliVideoModelWan27
)

var (
	size480p  = []string{"832*480", "480*832", "624*624"}
	size720p  = []string{"1280*720", "720*1280", "960*960", "1088*832", "832*1088"}
	size1080p = []string{"1920*1080", "1080*1920", "1440*1440", "1632*1248", "1248*1632"}
)

const (
	happyHorseVideoEditMinSourceSeconds   = 3
	happyHorseVideoEditMaxSourceSeconds   = 60
	happyHorseVideoEditMaxBillableSeconds = happyHorseVideoEditMaxSourceSeconds * 2
	happyHorseVideoEditMaxSourceBytes     = 100 * 1024 * 1024
	happyHorseVideoEditDurationContextKey = "ali_happyhorse_video_edit_source_duration"
)

var happyHorseSourceDurationProbe = service.ProbeRemoteVideoDuration

func ptr[T any](value T) *T {
	return &value
}

func getAliVideoModelKind(modelName string) aliVideoModelKind {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.HasPrefix(modelName, "happyhorse-") && strings.Contains(modelName, "-video-edit"):
		return aliVideoModelHappyHorseEdit
	case strings.HasPrefix(modelName, "happyhorse-") && strings.Contains(modelName, "-r2v"):
		return aliVideoModelHappyHorseR2V
	case strings.HasPrefix(modelName, "happyhorse-") && strings.Contains(modelName, "-i2v"):
		return aliVideoModelHappyHorseI2V
	case strings.HasPrefix(modelName, "happyhorse-") && strings.Contains(modelName, "-t2v"):
		return aliVideoModelHappyHorseT2V
	case strings.HasPrefix(modelName, "wan2.7-i2v"):
		return aliVideoModelWan27
	default:
		return aliVideoModelLegacy
	}
}

func isHappyHorseModelKind(kind aliVideoModelKind) bool {
	return kind >= aliVideoModelHappyHorseT2V && kind <= aliVideoModelHappyHorseEdit
}

func actionForAliVideoModelKind(kind aliVideoModelKind) string {
	if kind == aliVideoModelHappyHorseT2V {
		return constant.TaskActionTextGenerate
	}
	if kind == aliVideoModelHappyHorseI2V || kind == aliVideoModelHappyHorseR2V ||
		kind == aliVideoModelHappyHorseEdit || kind == aliVideoModelWan27 {
		return constant.TaskActionGenerate
	}
	return ""
}

func normalizeAliBaseURL(baseURL string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", errors.New("ali video base URL is empty")
	}
	if strings.Contains(strings.ToLower(baseURL), "{workspaceid}") {
		return "", errors.New("replace WorkspaceId placeholder in ali video base URL")
	}
	// 阿里控制台给出的 Workspace API 地址可能以 /api 或 /api/v1 结尾，
	// 而下面的提交与查询逻辑会统一补完整的 /api/v1/... 路径。两种后缀都
	// 归一到站点根地址，避免形成 /api/api/v1/... 导致上游 404。
	lowerBaseURL := strings.ToLower(baseURL)
	for _, suffix := range []string{"/api/v1", "/api"} {
		if strings.HasSuffix(lowerBaseURL, suffix) {
			baseURL = baseURL[:len(baseURL)-len(suffix)]
			break
		}
	}
	return strings.TrimRight(baseURL, "/"), nil
}

func sizeToResolution(size string) (string, error) {
	size = strings.TrimSpace(size)
	if lo.Contains(size480p, size) {
		return "480P", nil
	}
	if lo.Contains(size720p, size) {
		return "720P", nil
	}
	if lo.Contains(size1080p, size) {
		return "1080P", nil
	}
	return "", fmt.Errorf("invalid size: %s", size)
}

func normalizeResolution(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if strings.Contains(value, "*") {
		return sizeToResolution(value)
	}
	if value != "" && !strings.HasSuffix(value, "P") {
		value += "P"
	}
	return value, nil
}

func getUpstreamModel(info *relaycommon.RelayInfo, fallback string) string {
	if info != nil && info.ChannelMeta != nil && info.IsModelMapped && strings.TrimSpace(info.UpstreamModelName) != "" {
		return info.UpstreamModelName
	}
	return fallback
}

func parseAliMetadata(req relaycommon.TaskSubmitReq, upstreamModel string) (*AliMetadata, error) {
	metadata := &AliMetadata{}
	if req.Metadata == nil {
		return metadata, nil
	}
	if modelValue, exists := req.Metadata["model"]; exists {
		modelName, ok := modelValue.(string)
		if !ok || (modelName != req.Model && modelName != upstreamModel) {
			return nil, errors.New("can't change model with metadata")
		}
	}
	metadataBytes, err := common.Marshal(req.Metadata)
	if err != nil {
		return nil, errors.Wrap(err, "marshal metadata failed")
	}
	if err := common.Unmarshal(metadataBytes, metadata); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	return metadata, nil
}

func mergeParameters(dst *AliVideoParameters, src *AliVideoParameters) {
	if dst == nil || src == nil {
		return
	}
	if src.Resolution != nil {
		dst.Resolution = src.Resolution
	}
	if src.Size != nil {
		dst.Size = src.Size
	}
	if src.Ratio != nil {
		dst.Ratio = src.Ratio
	}
	if src.Duration != nil {
		dst.Duration = src.Duration
	}
	if src.PromptExtend != nil {
		dst.PromptExtend = src.PromptExtend
	}
	if src.Watermark != nil {
		dst.Watermark = src.Watermark
	}
	if src.Audio != nil {
		dst.Audio = src.Audio
	}
	if src.AudioSetting != nil {
		dst.AudioSetting = src.AudioSetting
	}
	if src.Seed != nil {
		dst.Seed = src.Seed
	}
}

func applyMetadataParameters(dst *AliVideoParameters, metadata *AliMetadata) {
	if metadata == nil {
		return
	}
	mergeParameters(dst, metadata.Parameters)
	mergeParameters(dst, &AliVideoParameters{
		Resolution:   metadata.Resolution,
		Size:         metadata.Size,
		Ratio:        metadata.Ratio,
		Duration:     metadata.Duration,
		PromptExtend: metadata.PromptExtend,
		Watermark:    metadata.Watermark,
		Audio:        metadata.Audio,
		AudioSetting: metadata.AudioSetting,
		Seed:         metadata.Seed,
	})
}

func captureHappyHorseEditBillingDuration(req *AliVideoRequest) {
	if req == nil || getAliVideoModelKind(req.Model) != aliVideoModelHappyHorseEdit ||
		req.Parameters == nil || req.Parameters.Duration == nil {
		return
	}
	// Video Edit 的阿里上游协议不接受 parameters.duration。网关的通用与
	// 专有兼容入口允许把它作为整数秒的源视频计费提示，但转发前必须清除。
	if req.BillingInputVideoDuration <= 0 {
		req.BillingInputVideoDuration = float64(*req.Parameters.Duration)
	}
	req.Parameters.Duration = nil
}

func ensureVerifiedHappyHorseEditDuration(c *gin.Context, req *AliVideoRequest) error {
	if req == nil || getAliVideoModelKind(req.Model) != aliVideoModelHappyHorseEdit {
		return nil
	}
	if c != nil {
		if duration, exists := c.Get(happyHorseVideoEditDurationContextKey); exists {
			if value, ok := duration.(float64); ok && value > 0 {
				req.BillingInputVideoDuration = value
				return nil
			}
		}
	}
	if c == nil || c.Request == nil {
		return errors.New("HappyHorse video editing source video duration verification requires request context")
	}
	var sourceURL string
	for _, media := range req.Input.Media {
		if media.Type == "video" {
			sourceURL = media.URL
			break
		}
	}
	if sourceURL == "" {
		return errors.New("HappyHorse video editing source video URL is required")
	}
	duration, err := happyHorseSourceDurationProbe(
		c.Request.Context(), sourceURL, happyHorseVideoEditMaxSourceBytes,
	)
	if err != nil {
		return errors.Wrap(err, "verify HappyHorse source video duration")
	}
	if math.IsNaN(duration) || math.IsInf(duration, 0) ||
		duration < happyHorseVideoEditMinSourceSeconds || duration > happyHorseVideoEditMaxSourceSeconds {
		return fmt.Errorf(
			"HappyHorse video editing source video duration must be between %d and %d seconds",
			happyHorseVideoEditMinSourceSeconds, happyHorseVideoEditMaxSourceSeconds,
		)
	}
	req.BillingInputVideoDuration = duration
	c.Set(happyHorseVideoEditDurationContextKey, duration)
	return nil
}

func requestDuration(req relaycommon.TaskSubmitReq) (*int, error) {
	if req.Duration != nil {
		return ptr(*req.Duration), nil
	}
	if strings.TrimSpace(req.Seconds) != "" {
		seconds, err := strconv.Atoi(req.Seconds)
		if err != nil {
			return nil, errors.Wrap(err, "convert seconds to int failed")
		}
		return ptr(seconds), nil
	}
	return nil, nil
}

func (a *TaskAdaptor) convertToAliRequest(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*AliVideoRequest, error) {
	upstreamModel := getUpstreamModel(info, req.Model)
	metadata, err := parseAliMetadata(req, upstreamModel)
	if err != nil {
		return nil, err
	}

	kind := getAliVideoModelKind(upstreamModel)
	switch {
	case isHappyHorseModelKind(kind):
		return convertHappyHorseRequest(upstreamModel, req, metadata)
	case kind == aliVideoModelWan27:
		return convertWan27Request(upstreamModel, req, metadata)
	default:
		return convertLegacyAliRequest(upstreamModel, req, metadata)
	}
}

func convertHappyHorseRequest(modelName string, req relaycommon.TaskSubmitReq, metadata *AliMetadata) (*AliVideoRequest, error) {
	kind := getAliVideoModelKind(modelName)
	if !isHappyHorseModelKind(kind) {
		return nil, fmt.Errorf("unsupported HappyHorse model: %s", modelName)
	}

	duration, err := requestDuration(req)
	if err != nil {
		return nil, err
	}
	if kind != aliVideoModelHappyHorseEdit && duration == nil {
		duration = ptr(5)
	}
	resolution := "1080P"
	if req.Size != "" {
		resolution, err = normalizeResolution(req.Size)
		if err != nil {
			return nil, err
		}
	}

	aliReq := &AliVideoRequest{
		Model: modelName,
		Input: AliVideoInput{Prompt: req.Prompt},
		Parameters: &AliVideoParameters{
			Resolution: ptr(resolution),
			Watermark:  ptr(true),
		},
	}
	if kind == aliVideoModelHappyHorseT2V || kind == aliVideoModelHappyHorseR2V {
		aliReq.Parameters.Ratio = ptr("16:9")
	}
	if duration != nil {
		aliReq.Parameters.Duration = duration
	}
	applyMetadataParameters(aliReq.Parameters, metadata)
	captureHappyHorseEditBillingDuration(aliReq)
	if err := appendHappyHorseMedia(aliReq, kind, req, metadata); err != nil {
		return nil, err
	}
	if metadata != nil {
		if metadata.Input != nil {
			aliReq.Input.NegativePrompt = firstNonEmpty(metadata.Input.NegativePrompt, aliReq.Input.NegativePrompt)
			aliReq.Input.Template = firstNonEmpty(metadata.Input.Template, aliReq.Input.Template)
		}
		aliReq.Input.NegativePrompt = firstNonEmpty(metadata.NegativePrompt, aliReq.Input.NegativePrompt)
		aliReq.Input.Template = firstNonEmpty(metadata.Template, aliReq.Input.Template)
	}
	if err := validateAliVideoRequest(aliReq); err != nil {
		return nil, err
	}
	return aliReq, nil
}

func appendHappyHorseMedia(aliReq *AliVideoRequest, kind aliVideoModelKind, req relaycommon.TaskSubmitReq, metadata *AliMetadata) error {
	if aliReq == nil {
		return errors.New("HappyHorse request is required")
	}
	defaultImageType := "reference_image"
	if kind == aliVideoModelHappyHorseI2V {
		defaultImageType = "first_frame"
	}
	appendMedia := func(mediaType, url string) {
		if strings.TrimSpace(url) == "" {
			return
		}
		aliReq.Input.Media = append(aliReq.Input.Media, AliVideoMedia{Type: mediaType, URL: url})
	}

	requestImages := req.Images
	if req.InputReference != "" {
		requestImages = []string{req.InputReference}
	} else if req.Image != "" {
		requestImages = []string{req.Image}
	}
	for _, imageURL := range requestImages {
		appendMedia(defaultImageType, imageURL)
	}
	if metadata == nil {
		return nil
	}

	appendLegacyInput := func(input *AliVideoInput) {
		if input == nil {
			return
		}
		aliReq.Input.Media = append(aliReq.Input.Media, input.Media...)
		appendMedia(defaultImageType, input.ImgURL)
		appendMedia("first_frame", input.FirstFrameURL)
		appendMedia("last_frame", input.LastFrameURL)
		appendMedia("audio", input.AudioURL)
	}
	appendLegacyInput(metadata.Input)
	aliReq.Input.Media = append(aliReq.Input.Media, metadata.Media...)
	appendMedia(defaultImageType, metadata.ImgURL)
	appendMedia("first_frame", metadata.FirstFrameURL)
	appendMedia("last_frame", metadata.LastFrameURL)
	appendMedia("audio", metadata.AudioURL)

	for _, content := range metadata.Content {
		media, err := happyHorseMediaFromContent(content)
		if err != nil {
			return err
		}
		aliReq.Input.Media = append(aliReq.Input.Media, media)
	}
	return nil
}

func happyHorseMediaFromContent(content AliContentItem) (AliVideoMedia, error) {
	role := strings.ToLower(strings.TrimSpace(content.Role))
	switch strings.ToLower(strings.TrimSpace(content.Type)) {
	case "image_url":
		if content.ImageURL == nil || strings.TrimSpace(content.ImageURL.URL) == "" {
			return AliVideoMedia{}, errors.New("HappyHorse image content URL is required")
		}
		if role == "" {
			role = "reference_image"
		}
		if role != "first_frame" && role != "reference_image" {
			return AliVideoMedia{}, fmt.Errorf("unsupported HappyHorse image role: %s", content.Role)
		}
		return AliVideoMedia{Type: role, URL: content.ImageURL.URL}, nil
	case "video_url":
		if content.VideoURL == nil || strings.TrimSpace(content.VideoURL.URL) == "" {
			return AliVideoMedia{}, errors.New("HappyHorse video content URL is required")
		}
		if role != "" && role != "reference_video" && role != "video" {
			return AliVideoMedia{}, fmt.Errorf("unsupported HappyHorse video role: %s", content.Role)
		}
		return AliVideoMedia{
			Type:            "video",
			URL:             content.VideoURL.URL,
			BillingDuration: float64(content.Duration),
		}, nil
	case "audio_url":
		return AliVideoMedia{}, errors.New("HappyHorse does not accept audio content")
	default:
		return AliVideoMedia{}, fmt.Errorf("unsupported HappyHorse content type: %s", content.Type)
	}
}

func convertWan27Request(modelName string, req relaycommon.TaskSubmitReq, metadata *AliMetadata) (*AliVideoRequest, error) {
	duration, err := requestDuration(req)
	if err != nil {
		return nil, err
	}
	if duration == nil {
		duration = ptr(5)
	}
	resolution := "1080P"
	if req.Size != "" {
		resolution, err = normalizeResolution(req.Size)
		if err != nil {
			return nil, err
		}
	}

	aliReq := &AliVideoRequest{
		Model: modelName,
		Input: AliVideoInput{Prompt: req.Prompt},
		Parameters: &AliVideoParameters{
			Resolution:   ptr(resolution),
			Duration:     duration,
			PromptExtend: ptr(true),
			Watermark:    ptr(false),
		},
	}
	applyMetadataParameters(aliReq.Parameters, metadata)

	mediaByType := make(map[string]AliVideoMedia)
	referenceImages := make([]string, 0, 2)
	addMedia := func(mediaType, url string) error {
		mediaType = strings.ToLower(strings.TrimSpace(mediaType))
		url = strings.TrimSpace(url)
		if mediaType == "" || url == "" {
			return errors.New("wan2.7 media type and url are required")
		}
		if !lo.Contains([]string{"first_frame", "last_frame", "driving_audio", "first_clip"}, mediaType) {
			return fmt.Errorf("unsupported wan2.7 media type: %s", mediaType)
		}
		if _, exists := mediaByType[mediaType]; exists {
			return fmt.Errorf("wan2.7 media type %s can appear at most once", mediaType)
		}
		mediaByType[mediaType] = AliVideoMedia{Type: mediaType, URL: url}
		return nil
	}

	if req.InputReference != "" {
		referenceImages = append(referenceImages, req.InputReference)
	} else if req.Image != "" {
		referenceImages = append(referenceImages, req.Image)
	} else if len(req.Images) > 0 {
		referenceImages = append(referenceImages, req.Images...)
	}
	if metadata != nil {
		if metadata.Input != nil {
			aliReq.Input.NegativePrompt = metadata.Input.NegativePrompt
			aliReq.Input.Template = metadata.Input.Template
			for _, media := range metadata.Input.Media {
				if err := addMedia(media.Type, media.URL); err != nil {
					return nil, err
				}
			}
			for mediaType, url := range map[string]string{
				"first_frame":   firstNonEmpty(metadata.Input.FirstFrameURL, metadata.Input.ImgURL),
				"last_frame":    metadata.Input.LastFrameURL,
				"driving_audio": metadata.Input.AudioURL,
			} {
				if url != "" {
					if err := addMedia(mediaType, url); err != nil {
						return nil, err
					}
				}
			}
		}
		aliReq.Input.NegativePrompt = firstNonEmpty(metadata.NegativePrompt, aliReq.Input.NegativePrompt)
		aliReq.Input.Template = firstNonEmpty(metadata.Template, aliReq.Input.Template)
		for _, media := range metadata.Media {
			if err := addMedia(media.Type, media.URL); err != nil {
				return nil, err
			}
		}
		for mediaType, url := range map[string]string{
			"first_frame":   firstNonEmpty(metadata.FirstFrameURL, metadata.ImgURL),
			"last_frame":    metadata.LastFrameURL,
			"driving_audio": metadata.AudioURL,
		} {
			if url != "" {
				if err := addMedia(mediaType, url); err != nil {
					return nil, err
				}
			}
		}
		for _, content := range metadata.Content {
			role := strings.ToLower(strings.TrimSpace(content.Role))
			if strings.EqualFold(content.Type, "image_url") && (role == "" || role == "reference_image") {
				if content.ImageURL == nil || strings.TrimSpace(content.ImageURL.URL) == "" {
					return nil, errors.New("wan2.7 reference image url is required")
				}
				referenceImages = append(referenceImages, content.ImageURL.URL)
				continue
			}
			mediaType, url, err := wan27MediaFromContent(content)
			if err != nil {
				return nil, err
			}
			if err := addMedia(mediaType, url); err != nil {
				return nil, err
			}
		}
	}
	if len(referenceImages) > 2 {
		return nil, errors.New("wan2.7 accepts at most two reference images")
	}
	if len(referenceImages) > 0 {
		if _, hasFirstClip := mediaByType["first_clip"]; hasFirstClip {
			if len(referenceImages) != 1 {
				return nil, errors.New("wan2.7 first_clip accepts at most one last-frame image")
			}
			if err := addMedia("last_frame", referenceImages[0]); err != nil {
				return nil, err
			}
		} else {
			if err := addMedia("first_frame", referenceImages[0]); err != nil {
				return nil, err
			}
			if len(referenceImages) == 2 {
				if err := addMedia("last_frame", referenceImages[1]); err != nil {
					return nil, err
				}
			}
		}
	}

	for _, mediaType := range []string{"first_frame", "first_clip", "last_frame", "driving_audio"} {
		if media, exists := mediaByType[mediaType]; exists {
			aliReq.Input.Media = append(aliReq.Input.Media, media)
		}
	}
	if err := validateAliVideoRequest(aliReq); err != nil {
		return nil, err
	}
	return aliReq, nil
}

func wan27MediaFromContent(content AliContentItem) (string, string, error) {
	role := strings.ToLower(strings.TrimSpace(content.Role))
	contentType := strings.ToLower(strings.TrimSpace(content.Type))
	var url string
	switch contentType {
	case "image_url":
		if content.ImageURL != nil {
			url = content.ImageURL.URL
		}
		if role == "" || role == "reference_image" {
			return "", "", errors.New("reference_image must be resolved with the complete wan2.7 content list")
		}
		if role != "first_frame" && role != "last_frame" {
			return "", "", fmt.Errorf("wan2.7 image content does not support role: %s", content.Role)
		}
	case "video_url":
		if content.VideoURL != nil {
			url = content.VideoURL.URL
		}
		if role == "" || role == "reference_video" {
			role = "first_clip"
		}
		if role != "first_clip" {
			return "", "", fmt.Errorf("wan2.7 video content does not support role: %s", content.Role)
		}
	case "audio_url":
		if content.AudioURL != nil {
			url = content.AudioURL.URL
		}
		if role == "" || role == "reference_audio" {
			role = "driving_audio"
		}
		if role != "driving_audio" {
			return "", "", fmt.Errorf("wan2.7 audio content does not support role: %s", content.Role)
		}
	default:
		return "", "", fmt.Errorf("unsupported wan2.7 content type: %s", content.Type)
	}
	if !lo.Contains([]string{"first_frame", "last_frame", "driving_audio", "first_clip"}, role) {
		return "", "", fmt.Errorf("unsupported wan2.7 media role: %s", content.Role)
	}
	return role, url, nil
}

func convertLegacyAliRequest(modelName string, req relaycommon.TaskSubmitReq, metadata *AliMetadata) (*AliVideoRequest, error) {
	duration, err := requestDuration(req)
	if err != nil {
		return nil, err
	}
	if duration == nil {
		duration = ptr(5)
	}
	aliReq := &AliVideoRequest{
		Model: modelName,
		Input: AliVideoInput{
			Prompt: req.Prompt,
			ImgURL: req.InputReference,
		},
		Parameters: &AliVideoParameters{
			Duration:     duration,
			PromptExtend: ptr(true),
			Watermark:    ptr(false),
		},
	}

	if req.Size != "" {
		if strings.Contains(modelName, "t2v") && !strings.Contains(req.Size, "*") {
			return nil, fmt.Errorf("invalid size: %s, example: %s", req.Size, "1920*1080")
		}
		if strings.Contains(req.Size, "*") {
			aliReq.Parameters.Size = ptr(req.Size)
		} else {
			resolution, err := normalizeResolution(req.Size)
			if err != nil {
				return nil, err
			}
			aliReq.Parameters.Resolution = ptr(resolution)
		}
	} else if strings.Contains(modelName, "t2v") {
		if strings.HasPrefix(modelName, "wan2.5") || strings.HasPrefix(modelName, "wan2.2") {
			aliReq.Parameters.Size = ptr("1920*1080")
		} else {
			aliReq.Parameters.Size = ptr("1280*720")
		}
	} else {
		resolution := "720P"
		if strings.HasPrefix(modelName, "wan2.6") || strings.HasPrefix(modelName, "wan2.5") || strings.HasPrefix(modelName, "wan2.2-i2v-plus") {
			resolution = "1080P"
		}
		aliReq.Parameters.Resolution = ptr(resolution)
	}

	if metadata != nil {
		if metadata.Input != nil {
			aliReq.Input.AudioURL = firstNonEmpty(metadata.Input.AudioURL, aliReq.Input.AudioURL)
			aliReq.Input.ImgURL = firstNonEmpty(metadata.Input.ImgURL, aliReq.Input.ImgURL)
			aliReq.Input.FirstFrameURL = firstNonEmpty(metadata.Input.FirstFrameURL, aliReq.Input.FirstFrameURL)
			aliReq.Input.LastFrameURL = firstNonEmpty(metadata.Input.LastFrameURL, aliReq.Input.LastFrameURL)
			aliReq.Input.NegativePrompt = firstNonEmpty(metadata.Input.NegativePrompt, aliReq.Input.NegativePrompt)
			aliReq.Input.Template = firstNonEmpty(metadata.Input.Template, aliReq.Input.Template)
		}
		aliReq.Input.AudioURL = firstNonEmpty(metadata.AudioURL, aliReq.Input.AudioURL)
		aliReq.Input.ImgURL = firstNonEmpty(metadata.ImgURL, aliReq.Input.ImgURL)
		aliReq.Input.FirstFrameURL = firstNonEmpty(metadata.FirstFrameURL, aliReq.Input.FirstFrameURL)
		aliReq.Input.LastFrameURL = firstNonEmpty(metadata.LastFrameURL, aliReq.Input.LastFrameURL)
		aliReq.Input.NegativePrompt = firstNonEmpty(metadata.NegativePrompt, aliReq.Input.NegativePrompt)
		aliReq.Input.Template = firstNonEmpty(metadata.Template, aliReq.Input.Template)
		applyMetadataParameters(aliReq.Parameters, metadata)
	}
	return aliReq, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func validateAliVideoRequest(req *AliVideoRequest) error {
	if req == nil {
		return errors.New("ali video request is required")
	}
	if strings.TrimSpace(req.Model) == "" {
		return errors.New("model is required")
	}
	if req.Parameters == nil {
		req.Parameters = &AliVideoParameters{}
	}
	if req.Parameters.Seed != nil && (*req.Parameters.Seed < 0 || *req.Parameters.Seed > 2147483647) {
		return errors.New("seed must be between 0 and 2147483647")
	}

	kind := getAliVideoModelKind(req.Model)
	switch {
	case isHappyHorseModelKind(kind):
		return validateHappyHorseRequest(req)
	case kind == aliVideoModelWan27:
		return validateWan27Request(req)
	default:
		return nil
	}
}

func validateHappyHorseRequest(req *AliVideoRequest) error {
	kind := getAliVideoModelKind(req.Model)
	if req.Input.NegativePrompt != "" || req.Input.Template != "" {
		return errors.New("HappyHorse does not support negative_prompt or template")
	}
	if req.Parameters.Size != nil || req.Parameters.PromptExtend != nil || req.Parameters.Audio != nil {
		return errors.New("HappyHorse does not support size, prompt_extend or audio parameters")
	}
	if req.Input.ImgURL != "" || req.Input.FirstFrameURL != "" || req.Input.LastFrameURL != "" || req.Input.AudioURL != "" {
		return errors.New("HappyHorse requires media input through input.media")
	}
	if req.Parameters.Resolution != nil {
		resolution, err := normalizeResolution(*req.Parameters.Resolution)
		if err != nil {
			return err
		}
		allowedResolutions := []string{"480P", "720P", "1080P"}
		if kind == aliVideoModelHappyHorseEdit {
			allowedResolutions = []string{"720P", "1080P"}
		}
		if !lo.Contains(allowedResolutions, resolution) {
			return fmt.Errorf("invalid HappyHorse resolution: %s", *req.Parameters.Resolution)
		}
		req.Parameters.Resolution = ptr(resolution)
	}
	for i := range req.Input.Media {
		req.Input.Media[i].Type = strings.ToLower(strings.TrimSpace(req.Input.Media[i].Type))
		req.Input.Media[i].URL = strings.TrimSpace(req.Input.Media[i].URL)
		if req.Input.Media[i].URL == "" {
			return fmt.Errorf("HappyHorse media URL is required for %s", req.Input.Media[i].Type)
		}
	}

	switch kind {
	case aliVideoModelHappyHorseT2V:
		if strings.TrimSpace(req.Input.Prompt) == "" {
			return errors.New("prompt is required for HappyHorse text-to-video")
		}
		if len(req.Input.Media) > 0 {
			return errors.New("HappyHorse text-to-video does not accept media input")
		}
		return validateHappyHorseGenerationParameters(req, true)
	case aliVideoModelHappyHorseI2V:
		if len(req.Input.Media) != 1 || req.Input.Media[0].Type != "first_frame" {
			return errors.New("HappyHorse image-to-video requires exactly one first_frame")
		}
		return validateHappyHorseGenerationParameters(req, false)
	case aliVideoModelHappyHorseR2V:
		if strings.TrimSpace(req.Input.Prompt) == "" {
			return errors.New("prompt is required for HappyHorse reference-to-video")
		}
		if len(req.Input.Media) < 1 || len(req.Input.Media) > 9 {
			return errors.New("HappyHorse reference-to-video requires 1 to 9 reference_image items")
		}
		for _, media := range req.Input.Media {
			if media.Type != "reference_image" {
				return errors.New("HappyHorse reference-to-video only accepts reference_image items")
			}
		}
		return validateHappyHorseGenerationParameters(req, true)
	case aliVideoModelHappyHorseEdit:
		return validateHappyHorseEditRequest(req)
	default:
		return fmt.Errorf("unsupported HappyHorse model: %s", req.Model)
	}
}

func validateHappyHorseGenerationParameters(req *AliVideoRequest, allowRatio bool) error {
	if req.Parameters.AudioSetting != nil {
		return errors.New("HappyHorse generation models do not support audio_setting")
	}
	if !allowRatio && req.Parameters.Ratio != nil {
		return errors.New("HappyHorse image-to-video does not support ratio")
	}
	if req.Parameters.Ratio != nil && !lo.Contains([]string{"16:9", "9:16", "1:1", "4:3", "3:4", "4:5", "5:4", "9:21", "21:9"}, *req.Parameters.Ratio) {
		return fmt.Errorf("invalid HappyHorse ratio: %s", *req.Parameters.Ratio)
	}
	if req.Parameters.Duration != nil && (*req.Parameters.Duration < 3 || *req.Parameters.Duration > 15) {
		return errors.New("HappyHorse duration must be between 3 and 15 seconds")
	}
	return nil
}

func validateHappyHorseEditRequest(req *AliVideoRequest) error {
	if strings.TrimSpace(req.Input.Prompt) == "" {
		return errors.New("prompt is required for HappyHorse video editing")
	}
	if req.Parameters.Ratio != nil || req.Parameters.Duration != nil {
		return errors.New("HappyHorse video editing does not support ratio or duration parameters")
	}
	if req.Parameters.AudioSetting != nil && !lo.Contains([]string{"auto", "origin"}, *req.Parameters.AudioSetting) {
		return fmt.Errorf("invalid HappyHorse audio_setting: %s", *req.Parameters.AudioSetting)
	}
	videoCount := 0
	referenceImageCount := 0
	var sourceVideo AliVideoMedia
	referenceImages := make([]AliVideoMedia, 0, len(req.Input.Media))
	for _, media := range req.Input.Media {
		switch media.Type {
		case "video":
			videoCount++
			sourceVideo = media
		case "reference_image":
			referenceImageCount++
			referenceImages = append(referenceImages, media)
		default:
			return fmt.Errorf("HappyHorse video editing does not support media type: %s", media.Type)
		}
	}
	if videoCount != 1 {
		return errors.New("HappyHorse video editing requires exactly one video")
	}
	if referenceImageCount > 5 {
		return errors.New("HappyHorse video editing accepts at most 5 reference_image items")
	}
	sourceDuration := sourceVideo.BillingDuration
	if sourceDuration <= 0 {
		sourceDuration = req.BillingInputVideoDuration
	}
	if math.IsNaN(sourceDuration) || math.IsInf(sourceDuration, 0) {
		return errors.New("HappyHorse video editing source video duration must be finite")
	}
	// 客户端 duration 只作为兼容提示，不能在远程媒体探测前据此拒绝请求；
	// 真实的 3–60 秒约束由 ensureVerifiedHappyHorseEditDuration 校验。
	if sourceDuration > 0 {
		req.BillingInputVideoDuration = sourceDuration
		sourceVideo.BillingDuration = sourceDuration
	}
	// 官方示例始终把待编辑视频放在 media[0]，统一重排可避免操练场按
	// schema 槽位遍历时先收集参考图而改变上游媒体语义。
	req.Input.Media = append([]AliVideoMedia{sourceVideo}, referenceImages...)
	return nil
}

func validateWan27Request(req *AliVideoRequest) error {
	if req.Parameters.Size != nil || req.Parameters.Ratio != nil || req.Parameters.Audio != nil {
		return errors.New("wan2.7 does not support size, ratio or audio parameters")
	}
	if req.Input.Template != "" {
		return errors.New("wan2.7 does not support template")
	}
	if req.Parameters.Resolution != nil {
		resolution, err := normalizeResolution(*req.Parameters.Resolution)
		if err != nil {
			return err
		}
		if !lo.Contains([]string{"720P", "1080P"}, resolution) {
			return fmt.Errorf("invalid wan2.7 resolution: %s", *req.Parameters.Resolution)
		}
		req.Parameters.Resolution = ptr(resolution)
	}
	if req.Parameters.Duration != nil && (*req.Parameters.Duration < 2 || *req.Parameters.Duration > 15) {
		return errors.New("wan2.7 duration must be between 2 and 15 seconds")
	}

	mediaTypes := make(map[string]bool, len(req.Input.Media))
	for i := range req.Input.Media {
		media := &req.Input.Media[i]
		media.Type = strings.ToLower(strings.TrimSpace(media.Type))
		media.URL = strings.TrimSpace(media.URL)
		if !lo.Contains([]string{"first_frame", "last_frame", "driving_audio", "first_clip"}, media.Type) {
			return fmt.Errorf("unsupported wan2.7 media type: %s", media.Type)
		}
		if media.URL == "" {
			return fmt.Errorf("wan2.7 media url is required for %s", media.Type)
		}
		if mediaTypes[media.Type] {
			return fmt.Errorf("wan2.7 media type %s can appear at most once", media.Type)
		}
		mediaTypes[media.Type] = true
	}

	hasFirstFrame := mediaTypes["first_frame"]
	hasLastFrame := mediaTypes["last_frame"]
	hasAudio := mediaTypes["driving_audio"]
	hasFirstClip := mediaTypes["first_clip"]
	if hasFirstClip {
		if hasFirstFrame || hasAudio {
			return errors.New("wan2.7 first_clip only supports optional last_frame")
		}
		return nil
	}
	if !hasFirstFrame {
		return errors.New("wan2.7 requires first_frame or first_clip")
	}
	if hasLastFrame || hasAudio || len(mediaTypes) == 1 {
		return nil
	}
	return errors.New("invalid wan2.7 media combination")
}

func (a *TaskAdaptor) validateOfficialRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	req, err := a.getOfficialRequest(c, nil)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "InvalidParameter", http.StatusBadRequest)
	}
	if err := validateAliVideoRequest(req); err != nil {
		return service.TaskErrorWrapperLocal(err, "InvalidParameter", http.StatusBadRequest)
	}
	if action := actionForAliVideoModelKind(getAliVideoModelKind(req.Model)); action != "" {
		info.Action = action
	} else {
		info.Action = constant.TaskActionGenerate
	}
	return nil
}

func (a *TaskAdaptor) getOfficialRequest(c *gin.Context, info *relaycommon.RelayInfo) (*AliVideoRequest, error) {
	var aliReq AliVideoRequest
	if original, exists := c.Get("ali_video_original_request"); exists {
		bodyBytes, err := common.Marshal(original)
		if err != nil {
			return nil, err
		}
		if err := common.Unmarshal(bodyBytes, &aliReq); err != nil {
			return nil, err
		}
	} else if err := common.UnmarshalBodyReusable(c, &aliReq); err != nil {
		return nil, err
	}
	if info != nil {
		aliReq.Model = getUpstreamModel(info, aliReq.Model)
	}
	captureHappyHorseEditBillingDuration(&aliReq)
	if err := validateAliVideoRequest(&aliReq); err != nil {
		return nil, err
	}
	return &aliReq, nil
}

func ProcessAliOtherRatios(aliReq *AliVideoRequest) (map[string]float64, error) {
	otherRatios := make(map[string]float64)
	aliRatios := map[string]map[string]float64{
		"wan2.6-i2v":         {"720P": 1, "1080P": 1 / 0.6},
		"wan2.5-t2v-preview": {"480P": 1, "720P": 2, "1080P": 1 / 0.3},
		"wan2.2-t2v-plus":    {"480P": 1, "1080P": 0.7 / 0.14},
		"wan2.5-i2v-preview": {"480P": 1, "720P": 2, "1080P": 1 / 0.3},
		"wan2.2-i2v-plus":    {"480P": 1, "1080P": 0.7 / 0.14},
		"wan2.2-kf2v-flash":  {"480P": 1, "720P": 2, "1080P": 4.8},
		"wan2.2-i2v-flash":   {"480P": 1, "720P": 2},
		"wan2.2-s2v":         {"480P": 1, "720P": 0.9 / 0.5},
	}
	if aliReq == nil || aliReq.Parameters == nil {
		return otherRatios, nil
	}
	var resolution string
	if aliReq.Parameters.Size != nil && *aliReq.Parameters.Size != "" {
		toResolution, err := sizeToResolution(*aliReq.Parameters.Size)
		if err != nil {
			return nil, err
		}
		resolution = toResolution
	} else if aliReq.Parameters.Resolution != nil {
		resolution = strings.ToUpper(*aliReq.Parameters.Resolution)
		if !strings.HasSuffix(resolution, "P") {
			resolution += "P"
		}
	}
	if otherRatio, ok := aliRatios[aliReq.Model]; ok {
		if ratio, ok := otherRatio[resolution]; ok {
			otherRatios[fmt.Sprintf("resolution-%s", resolution)] = ratio
		}
	}
	return otherRatios, nil
}

func effectiveDuration(aliReq *AliVideoRequest) float64 {
	if aliReq != nil && getAliVideoModelKind(aliReq.Model) == aliVideoModelHappyHorseEdit {
		// Video Edit 的 usage.duration 是输入与输出视频时长之和，输出与源视频
		// 等长。因此预扣必须是本次源视频时长的 2 倍；120 秒只可能出现在
		// 60 秒源视频上，不能作为所有请求的固定预扣值。
		sourceDuration := aliReq.BillingInputVideoDuration
		if sourceDuration <= 0 && aliReq.Parameters != nil && aliReq.Parameters.Duration != nil {
			sourceDuration = float64(*aliReq.Parameters.Duration)
		}
		return math.Min(
			sourceDuration*2,
			float64(happyHorseVideoEditMaxBillableSeconds),
		)
	}
	if aliReq != nil && aliReq.Parameters != nil && aliReq.Parameters.Duration != nil {
		return float64(*aliReq.Parameters.Duration)
	}
	return 5
}
