package ali

import (
	"fmt"
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
	aliVideoModelHappyHorse
	aliVideoModelWan27
)

var (
	size480p  = []string{"832*480", "480*832", "624*624"}
	size720p  = []string{"1280*720", "720*1280", "960*960", "1088*832", "832*1088"}
	size1080p = []string{"1920*1080", "1080*1920", "1440*1440", "1632*1248", "1248*1632"}
)

func ptr[T any](value T) *T {
	return &value
}

func getAliVideoModelKind(modelName string) aliVideoModelKind {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.HasPrefix(modelName, "happyhorse-"):
		return aliVideoModelHappyHorse
	case strings.HasPrefix(modelName, "wan2.7-i2v"):
		return aliVideoModelWan27
	default:
		return aliVideoModelLegacy
	}
}

func normalizeAliBaseURL(baseURL string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", errors.New("ali video base URL is empty")
	}
	if strings.Contains(strings.ToLower(baseURL), "{workspaceid}") {
		return "", errors.New("replace WorkspaceId placeholder in ali video base URL")
	}
	baseURL = strings.TrimSuffix(baseURL, "/api/v1")
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
		Seed:         metadata.Seed,
	})
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

	switch getAliVideoModelKind(upstreamModel) {
	case aliVideoModelHappyHorse:
		return convertHappyHorseRequest(upstreamModel, req, metadata)
	case aliVideoModelWan27:
		return convertWan27Request(upstreamModel, req, metadata)
	default:
		return convertLegacyAliRequest(upstreamModel, req, metadata)
	}
}

func convertHappyHorseRequest(modelName string, req relaycommon.TaskSubmitReq, metadata *AliMetadata) (*AliVideoRequest, error) {
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
			Resolution: ptr(resolution),
			Ratio:      ptr("16:9"),
			Duration:   duration,
			Watermark:  ptr(true),
		},
	}
	if req.InputReference != "" || req.Image != "" || len(req.Images) > 0 {
		aliReq.Input.ImgURL = firstNonEmpty(req.InputReference, req.Image)
		if aliReq.Input.ImgURL == "" && len(req.Images) > 0 {
			aliReq.Input.ImgURL = req.Images[0]
		}
	}
	applyMetadataParameters(aliReq.Parameters, metadata)
	if metadata != nil {
		if len(metadata.Content) > 0 {
			return nil, errors.New("HappyHorse text-to-video does not accept media content")
		}
		if metadata.Input != nil {
			aliReq.Input.Media = append(aliReq.Input.Media, metadata.Input.Media...)
			aliReq.Input.ImgURL = firstNonEmpty(metadata.Input.ImgURL, aliReq.Input.ImgURL)
			aliReq.Input.FirstFrameURL = firstNonEmpty(metadata.Input.FirstFrameURL, aliReq.Input.FirstFrameURL)
			aliReq.Input.LastFrameURL = firstNonEmpty(metadata.Input.LastFrameURL, aliReq.Input.LastFrameURL)
			aliReq.Input.AudioURL = firstNonEmpty(metadata.Input.AudioURL, aliReq.Input.AudioURL)
			aliReq.Input.NegativePrompt = firstNonEmpty(metadata.Input.NegativePrompt, aliReq.Input.NegativePrompt)
			aliReq.Input.Template = firstNonEmpty(metadata.Input.Template, aliReq.Input.Template)
		}
		aliReq.Input.Media = append(aliReq.Input.Media, metadata.Media...)
		aliReq.Input.ImgURL = firstNonEmpty(metadata.ImgURL, aliReq.Input.ImgURL, req.InputReference)
		aliReq.Input.FirstFrameURL = firstNonEmpty(metadata.FirstFrameURL, aliReq.Input.FirstFrameURL)
		aliReq.Input.LastFrameURL = firstNonEmpty(metadata.LastFrameURL, aliReq.Input.LastFrameURL)
		aliReq.Input.AudioURL = firstNonEmpty(metadata.AudioURL, aliReq.Input.AudioURL)
		aliReq.Input.NegativePrompt = firstNonEmpty(metadata.NegativePrompt, aliReq.Input.NegativePrompt)
		aliReq.Input.Template = firstNonEmpty(metadata.Template, aliReq.Input.Template)
	}
	if err := validateAliVideoRequest(aliReq); err != nil {
		return nil, err
	}
	return aliReq, nil
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

	switch getAliVideoModelKind(req.Model) {
	case aliVideoModelHappyHorse:
		return validateHappyHorseRequest(req)
	case aliVideoModelWan27:
		return validateWan27Request(req)
	default:
		return nil
	}
}

func validateHappyHorseRequest(req *AliVideoRequest) error {
	if strings.TrimSpace(req.Input.Prompt) == "" {
		return errors.New("prompt is required for HappyHorse")
	}
	if len(req.Input.Media) > 0 || req.Input.ImgURL != "" || req.Input.FirstFrameURL != "" || req.Input.LastFrameURL != "" || req.Input.AudioURL != "" {
		return errors.New("HappyHorse text-to-video does not accept media input")
	}
	if req.Input.NegativePrompt != "" || req.Input.Template != "" {
		return errors.New("HappyHorse does not support negative_prompt or template")
	}
	if req.Parameters.Size != nil || req.Parameters.PromptExtend != nil || req.Parameters.Audio != nil {
		return errors.New("HappyHorse does not support size, prompt_extend or audio parameters")
	}
	if req.Parameters.Resolution != nil {
		resolution, err := normalizeResolution(*req.Parameters.Resolution)
		if err != nil {
			return err
		}
		if !lo.Contains([]string{"480P", "720P", "1080P"}, resolution) {
			return fmt.Errorf("invalid HappyHorse resolution: %s", *req.Parameters.Resolution)
		}
		req.Parameters.Resolution = ptr(resolution)
	}
	if req.Parameters.Ratio != nil && !lo.Contains([]string{"16:9", "9:16", "1:1", "4:3", "3:4", "4:5", "5:4", "9:21", "21:9"}, *req.Parameters.Ratio) {
		return fmt.Errorf("invalid HappyHorse ratio: %s", *req.Parameters.Ratio)
	}
	if req.Parameters.Duration != nil && (*req.Parameters.Duration < 3 || *req.Parameters.Duration > 15) {
		return errors.New("HappyHorse duration must be between 3 and 15 seconds")
	}
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
	if getAliVideoModelKind(req.Model) == aliVideoModelHappyHorse {
		info.Action = constant.TaskActionTextGenerate
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

func effectiveDuration(aliReq *AliVideoRequest) int {
	if aliReq != nil && aliReq.Parameters != nil && aliReq.Parameters.Duration != nil {
		return *aliReq.Parameters.Duration
	}
	return 5
}
