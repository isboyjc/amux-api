package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// 视频归档相关默认值。可用环境变量覆盖：
//   - VIDEO_ARCHIVE_MAX_BYTES：单个视频允许下载的最大字节数（默认 512MiB）
//   - VIDEO_ARCHIVE_DOWNLOAD_TIMEOUT_SEC：从上游下载的超时秒数（默认 180s）
const (
	defaultArchiveMaxBytes        int64 = 512 << 20 // 512 MiB
	defaultArchiveDownloadTimeout       = 180 * time.Second
)

func archiveMaxBytes() int64 {
	if v := strings.TrimSpace(os.Getenv("VIDEO_ARCHIVE_MAX_BYTES")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return defaultArchiveMaxBytes
}

func archiveDownloadTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("VIDEO_ARCHIVE_DOWNLOAD_TIMEOUT_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultArchiveDownloadTimeout
}

// archiveExt 推断归档对象的扩展名：优先看上游 URL 路径的扩展名，其次按
// Content-Type 推断，最后兜底 ".mp4"。
//
// 不直接用 mime.ExtensionsByType("video/mp4") 是因为它返回的候选顺序不稳定，
// 可能给出 ".f4v" 等非预期扩展名。
func archiveExt(upstreamURL, contentType string) string {
	if u, err := url.Parse(upstreamURL); err == nil {
		if ext := strings.ToLower(path.Ext(u.Path)); ext != "" && len(ext) <= 6 {
			return ext
		}
	}
	switch {
	case strings.Contains(contentType, "mp4"):
		return ".mp4"
	case strings.Contains(contentType, "webm"):
		return ".webm"
	case strings.Contains(contentType, "quicktime"), strings.Contains(contentType, "mov"):
		return ".mov"
	case strings.Contains(contentType, "gif"):
		return ".gif"
	}
	return ".mp4"
}

// BuildArchiveKey 拼出视频归档的对象 key：
//
//	video-archive/{userID}/{YYYYMMDD}/{taskID}{ext}
//
// 用 userID + 日期分目录，便于在 R2 控制台 / 后台按用户、日期回溯、清理、统计。
// 日期用扁平的 YYYYMMDD（不带斜杠），同一用户下所有日期一览无余、点一下即可
// 进到当天，比 YYYY/MM/DD 逐层点开更省事。
//
// **故意保持确定性**：日期取任务的 submitTime（不可变），而非 time.Now()——
// 这样同一任务重试、主节点重启后重传都落到同一个 key，幂等覆盖、绝不产生孤儿。
// taskID 全局唯一，同用户同日也不会撞 key。
//
// 例：video-archive/123/20260604/task_abc123.mp4
func BuildArchiveKey(taskID string, userID int, submitTime int64, upstreamURL, contentType string) string {
	id := strings.TrimSpace(taskID)
	if id == "" {
		id = "unknown"
	}
	uid := strconv.Itoa(userID)
	if userID <= 0 {
		uid = "anonymous"
	}
	datePart := "unknown-date"
	if submitTime > 0 {
		datePart = time.Unix(submitTime, 0).UTC().Format("20060102")
	}
	return fmt.Sprintf("video-archive/%s/%s/%s%s", uid, datePart, id, archiveExt(upstreamURL, contentType))
}

// ArchiveVideoToR2 把上游视频直链下载下来并上传到 R2，返回可公开访问的 URL。
//
// 设计要点：
//   - 走服务端下载 + 上传（不同于 PresignPut 的客户端直传）：归档是后台行为，
//     没有浏览器参与；
//   - 下载带超时 + 大小上限，避免异常大文件拖垮主进程内存；
//   - 确定性 key（见 BuildArchiveKey），重试/重启幂等；
//   - 任一环节失败返回 error，调用方据此走"降级回退到上游代理 URL"。
//
// 要求 storage 总开关已打开且凭证 + publicBaseURL 齐全，否则直接 error。
func ArchiveVideoToR2(ctx context.Context, taskID string, userID int, submitTime int64, upstreamURL string) (string, error) {
	cli, cfg, ok := getClient()
	if !ok {
		return "", errors.New("object storage is not configured")
	}
	if cfg.publicBaseURL == "" {
		return "", errors.New("storage public base URL is not configured")
	}
	if strings.TrimSpace(upstreamURL) == "" {
		return "", errors.New("empty upstream url")
	}
	if !strings.HasPrefix(upstreamURL, "http://") && !strings.HasPrefix(upstreamURL, "https://") {
		return "", fmt.Errorf("unsupported upstream url scheme: %s", upstreamURL)
	}

	// 1. 下载上游视频（带超时 + 大小上限）
	dlCtx, cancel := context.WithTimeout(ctx, archiveDownloadTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, upstreamURL, nil)
	if err != nil {
		return "", fmt.Errorf("build download request: %w", err)
	}
	httpClient := &http.Client{Timeout: archiveDownloadTimeout()}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download upstream video: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("upstream returned HTTP %d when downloading video", resp.StatusCode)
	}

	maxBytes := archiveMaxBytes()
	// 多读 1 字节用于探测是否超限
	limited := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read upstream video body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("upstream video exceeds max archive size %d bytes", maxBytes)
	}
	if len(data) == 0 {
		return "", errors.New("upstream video body is empty")
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || strings.HasPrefix(contentType, "application/octet-stream") {
		contentType = "video/mp4"
	}

	// 2. 上传到 R2（确定性 key，幂等覆盖）
	key := BuildArchiveKey(taskID, userID, submitTime, upstreamURL, contentType)
	upCtx, upCancel := context.WithTimeout(ctx, archiveDownloadTimeout())
	defer upCancel()
	_, err = cli.PutObject(upCtx, &s3.PutObjectInput{
		Bucket:        aws.String(cfg.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(int64(len(data))),
	})
	if err != nil {
		return "", fmt.Errorf("upload video to storage: %w", err)
	}

	publicURL := cfg.publicBaseURL + "/" + (&url.URL{Path: key}).EscapedPath()
	return publicURL, nil
}
