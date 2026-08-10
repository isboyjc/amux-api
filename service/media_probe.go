package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	remoteVideoDurationProbeTimeout        = 2 * time.Minute
	maxConcurrentRemoteVideoDurationProbes = 4
)

var remoteVideoDurationProbeSlots = make(chan struct{}, maxConcurrentRemoteVideoDurationProbes)

// ProbeRemoteVideoDuration 下载受 SSRF 策略保护的远程 MP4/MOV，并从容器
// mvhd 元数据读取实际时长。下载写入临时文件而不是常驻内存，并用 maxBytes
// 限制单请求磁盘与网络占用。
func ProbeRemoteVideoDuration(ctx context.Context, sourceURL string, maxBytes int64) (float64, error) {
	if maxBytes <= 0 {
		return 0, fmt.Errorf("invalid video probe size limit: %d", maxBytes)
	}
	parsedURL, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil {
		return 0, fmt.Errorf("parse video URL: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(parsedURL.Path))

	probeCtx, cancel := context.WithTimeout(ctx, remoteVideoDurationProbeTimeout)
	defer cancel()
	select {
	case remoteVideoDurationProbeSlots <- struct{}{}:
		defer func() { <-remoteVideoDurationProbeSlots }()
	case <-probeCtx.Done():
		return 0, fmt.Errorf("wait for video duration probe slot: %w", probeCtx.Err())
	}
	resp, err := DoDownloadRequestWithContext(probeCtx, parsedURL.String(), "HappyHorse video duration probe")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download video for duration probe: HTTP %d", resp.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if ext != ".mp4" && ext != ".mov" &&
		contentType != "video/mp4" && contentType != "video/quicktime" && contentType != "application/mp4" {
		return 0, fmt.Errorf("unsupported video format for duration probe: extension=%s content-type=%s", ext, contentType)
	}
	if resp.ContentLength > maxBytes {
		return 0, fmt.Errorf("video exceeds duration probe size limit: %d bytes", maxBytes)
	}

	tmp, err := os.CreateTemp("", "new-api-video-duration-*.mp4")
	if err != nil {
		return 0, fmt.Errorf("create video duration temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	written, err := io.Copy(tmp, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return 0, fmt.Errorf("download video for duration probe: %w", err)
	}
	if written > maxBytes {
		return 0, fmt.Errorf("video exceeds duration probe size limit: %d bytes", maxBytes)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("seek video duration temp file: %w", err)
	}

	// QuickTime MOV 与 MP4 都使用 ISO BMFF 容器；复用同一个 mvhd 解析器。
	duration, err := probeISOBaseMediaDuration(probeCtx, tmp)
	if err != nil {
		return 0, fmt.Errorf("probe video duration: %w", err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("probe video duration returned invalid value: %v", duration)
	}
	return duration, nil
}

func probeISOBaseMediaDuration(ctx context.Context, source io.ReadSeeker) (float64, error) {
	return common.GetAudioDuration(ctx, source, ".mp4")
}
