package ali

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const (
	aliMultipartMediaContextKey = "ali_video_multipart_media"
	aliMaxInputImageBytes       = 20 * 1024 * 1024
)

type aliMultipartMedia struct {
	Field  string
	Images []string
}

func getAliTaskRequest(c *gin.Context) (relaycommon.TaskSubmitReq, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return req, err
	}
	if c.Request == nil {
		return req, nil
	}
	if !strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
		return req, nil
	}

	media, err := getAliMultipartMedia(c)
	if err != nil {
		return req, err
	}
	if len(media.Images) == 0 {
		return req, nil
	}
	if req.InputReference != "" || req.Image != "" || len(req.Images) > 0 {
		return req, errors.New("multipart image files cannot be combined with image URL fields")
	}
	if media.Field == "images" {
		req.Images = append(req.Images, media.Images...)
	} else {
		req.InputReference = media.Images[0]
	}
	return req, nil
}

func getAliMultipartMedia(c *gin.Context) (*aliMultipartMedia, error) {
	if cached, exists := c.Get(aliMultipartMediaContextKey); exists {
		if media, ok := cached.(*aliMultipartMedia); ok {
			return media, nil
		}
	}

	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, errors.Wrap(err, "parse Ali video multipart form failed")
	}
	defer form.RemoveAll()

	var selectedField string
	var selectedFiles []*multipart.FileHeader
	for _, field := range []string{"input_reference", "image", "images"} {
		files := form.File[field]
		if len(files) == 0 {
			continue
		}
		if selectedField != "" {
			return nil, errors.New("use only one multipart image field: input_reference, image, or images")
		}
		selectedField = field
		selectedFiles = files
	}
	if len(selectedFiles) == 0 {
		media := &aliMultipartMedia{}
		c.Set(aliMultipartMediaContextKey, media)
		return media, nil
	}
	maxFiles := 1
	if selectedField == "images" {
		// HappyHorse r2v 最多接受 9 张参考图；具体模型的更小上限由请求
		// 转换后的模型级校验负责收紧（例如 Wan 2.7 仍然最多 2 张）。
		maxFiles = 9
	}
	if len(selectedFiles) > maxFiles {
		return nil, fmt.Errorf("multipart field %s accepts at most %d image(s)", selectedField, maxFiles)
	}

	media := &aliMultipartMedia{Field: selectedField, Images: make([]string, 0, len(selectedFiles))}
	for _, fileHeader := range selectedFiles {
		dataURI, err := aliImageFileToDataURI(fileHeader)
		if err != nil {
			return nil, err
		}
		media.Images = append(media.Images, dataURI)
	}
	c.Set(aliMultipartMediaContextKey, media)
	return media, nil
}

func aliImageFileToDataURI(fileHeader *multipart.FileHeader) (string, error) {
	if fileHeader == nil {
		return "", errors.New("multipart image file is required")
	}
	if fileHeader.Size > aliMaxInputImageBytes {
		return "", fmt.Errorf("multipart image %s exceeds 20MB", fileHeader.Filename)
	}
	file, err := fileHeader.Open()
	if err != nil {
		return "", errors.Wrap(err, "open multipart image failed")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, aliMaxInputImageBytes+1))
	if err != nil {
		return "", errors.Wrap(err, "read multipart image failed")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("multipart image %s is empty", fileHeader.Filename)
	}
	if len(data) > aliMaxInputImageBytes {
		return "", fmt.Errorf("multipart image %s exceeds 20MB", fileHeader.Filename)
	}

	mimeType := strings.ToLower(strings.TrimSpace(fileHeader.Header.Get("Content-Type")))
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = strings.ToLower(http.DetectContentType(data))
	}
	switch mimeType {
	case "image/jpeg", "image/png", "image/bmp", "image/webp":
	default:
		return "", fmt.Errorf("unsupported multipart image type: %s", mimeType)
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
