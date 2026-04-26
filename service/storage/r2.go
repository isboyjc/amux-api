// Package storage 提供 S3 兼容对象存储（当前主要对接 Cloudflare R2）的
// 通用预签名上传 / 删除能力。设计目标：
//
//   - 配置入库：admin 面板把 R2 凭证写到 system_setting.StorageSettings，
//     运行时按需热重建 client；env 变量作为部署兜底，DB 优先于 env
//   - 上传 key 强制走 {pathPrefix}/{userID}/{YYYY/MM/DD}/{uuid}{ext} 格式，
//     按业务域 + 用户 + 日期分目录隔离，便于回溯 / 清理 / 配额统计
//   - pathPrefix 由调用方（controller）按白名单决定，禁止任何用户输入；
//     防止越权写到别的目录
//   - 公开访问 URL 由 R2_PUBLIC_BASE_URL（或对应 setting）拼接；R2 桶默认
//     私有，没配公网就直接拒，避免上层拿到不可访问的 URL
//   - 上传走客户端直传：服务端只签 URL，文件流不经过 amux-api，省带宽
//     也避免大文件占应用进程
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

// PresignResult 预签名上传回执。客户端用 method+upload_url+headers 直接 PUT
// 到 R2；上传成功后，public_url 即可访问。
//
// 客户端必须原样发送 headers 里的每个 header（Content-Type 等签名 header），
// 否则签名校验失败、R2 拒绝。Host / Content-Length 浏览器自动带，无需也不
// 应在 JS 里显式 setRequestHeader。
type PresignResult struct {
	UploadURL   string            `json:"upload_url"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers"`
	Key         string            `json:"key"`
	PublicURL   string            `json:"public_url"`
	Bucket      string            `json:"bucket"`
	ExpiresAt   int64             `json:"expires_at"`
	Size        int64             `json:"size"`
	ContentType string            `json:"content_type"`
}

// resolvedConfig 是一份 settings + env 合流后的"实际生效配置"快照。
// 每次取 client 前都重新解析；当 signature 变了说明 admin 改了配置，
// 重建 s3 client。
//
// 「能用」分两层：
//   - credsReady：凭证 + endpoint + bucket 齐了，可以建 client
//   - toggleOn：admin 面板的"启用"开关打开了
//
// 对外的 IsEnabled() 要求两者都成立；管理员"测试连接"按钮则只要求
// credsReady（让 admin 在打开开关前先验证配置）。
type resolvedConfig struct {
	credsReady      bool
	toggleOn        bool
	provider        string
	accountID       string
	accessKeyID     string
	secretAccessKey string
	bucket          string
	endpoint        string
	region          string
	publicBaseURL   string // 末尾不带 "/"
}

// signature 把决定 client 行为的关键字段拼一行；用于判断 admin 改没改。
// 不要把 publicBaseURL 算进来——它只影响"返回给前端的 URL 拼接"，不影响
// 真正的 s3 连接，没必要因为它变更而重连 endpoint。
func (c resolvedConfig) signature() string {
	return strings.Join([]string{
		c.provider,
		c.accessKeyID,
		c.secretAccessKey,
		c.bucket,
		c.endpoint,
		c.region,
	}, "|")
}

// pickNonEmpty 工具函数：DB setting 优先，env 兜底；都空给 fallback。
func pickNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// resolveConfig 把 admin 面板配置（DB）与环境变量合流。优先级：
//   1) admin 配置（system_setting.StorageSettings）
//   2) 环境变量 R2_*（部署兜底，给老方式留迁移期）
//
// 注意：这里每次调用都拉一遍——StorageSettings 是单例 struct，DB 加载完
// 会原地改字段，所以任何时候 GetStorageSettings() 都拿到最新值。
func resolveConfig() resolvedConfig {
	s := system_setting.GetStorageSettings()

	cfg := resolvedConfig{
		provider:        strings.ToLower(strings.TrimSpace(pickNonEmpty(s.Provider, "r2"))),
		accountID:       pickNonEmpty(s.R2AccountID, common.GetEnvOrDefaultString("R2_ACCOUNT_ID", "")),
		accessKeyID:     pickNonEmpty(s.R2AccessKeyID, common.GetEnvOrDefaultString("R2_ACCESS_KEY_ID", "")),
		secretAccessKey: pickNonEmpty(s.R2SecretAccessKey, common.GetEnvOrDefaultString("R2_SECRET_ACCESS_KEY", "")),
		bucket:          pickNonEmpty(s.R2Bucket, common.GetEnvOrDefaultString("R2_BUCKET", "")),
		endpoint:        pickNonEmpty(s.R2Endpoint, common.GetEnvOrDefaultString("R2_ENDPOINT", "")),
		region:          pickNonEmpty(s.R2Region, common.GetEnvOrDefaultString("R2_REGION", ""), "auto"),
		publicBaseURL:   strings.TrimRight(pickNonEmpty(s.R2PublicBaseURL, common.GetEnvOrDefaultString("R2_PUBLIC_BASE_URL", "")), "/"),
	}

	// endpoint 没显式给：按 account_id 派生 R2 标准格式
	if cfg.endpoint == "" && cfg.accountID != "" {
		cfg.endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.accountID)
	}
	cfg.credsReady = cfg.accessKeyID != "" && cfg.secretAccessKey != "" && cfg.bucket != "" && cfg.endpoint != ""
	// 总开关只看 admin 面板。env 变量没有对应项——env 部署只填凭证，
	// 是否启用统一由 admin UI 控制
	cfg.toggleOn = s.Enabled
	return cfg
}

// 进程级单例：当前缓存的 client 与对应的 signature。signature 不一致就
// 重建。RWMutex 保证并发请求下只有一个 goroutine 重建。
var (
	clientMu     sync.RWMutex
	cachedClient *s3.Client
	cachedSig    string
)

// getClient 返回当前生效 client + 当前生效配置。**业务路径用这个**——
// 同时要求凭证齐 + admin 总开关 ON。任一不满足返回 (nil, cfg, false)。
func getClient() (*s3.Client, resolvedConfig, bool) {
	return getClientInternal(true)
}

// getClientForAdminTest 仅供 admin"测试连接"用：跳过总开关，只要凭证
// 齐就给 client。让管理员能在不打开 Enabled 的情况下先验证配置——
// 否则会陷入"必须启用 → 启用前却不能测试"的死循环。
func getClientForAdminTest() (*s3.Client, resolvedConfig, bool) {
	return getClientInternal(false)
}

func getClientInternal(requireToggle bool) (*s3.Client, resolvedConfig, bool) {
	cfg := resolveConfig()
	usable := cfg.credsReady && (!requireToggle || cfg.toggleOn)
	if !usable {
		// 关键字段缺失或总开关关闭：丢弃缓存的 client，让下次配齐 / 打开后
		// 立刻生效。用 RLock 先看一眼，避免每次空配置都抢写锁。
		clientMu.RLock()
		needClear := cachedClient != nil
		clientMu.RUnlock()
		if needClear {
			clientMu.Lock()
			cachedClient = nil
			cachedSig = ""
			clientMu.Unlock()
		}
		return nil, cfg, false
	}

	sig := cfg.signature()

	clientMu.RLock()
	if cachedClient != nil && cachedSig == sig {
		c := cachedClient
		clientMu.RUnlock()
		return c, cfg, true
	}
	clientMu.RUnlock()

	clientMu.Lock()
	defer clientMu.Unlock()
	// 二次检查：可能在等锁期间被别的 goroutine 重建
	if cachedClient != nil && cachedSig == sig {
		return cachedClient, cfg, true
	}

	awsCfg := aws.Config{
		Region:      cfg.region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.accessKeyID, cfg.secretAccessKey, ""),
	}
	cli := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.endpoint)
		// path-style addressing：避开 virtual-hosted 模式下 SDK 对桶名
		// 含 "." 的 SSL 校验抱怨；R2 两种风格都支持
		o.UsePathStyle = true
	})
	cachedClient = cli
	cachedSig = sig
	return cli, cfg, true
}

// IsEnabled 上层用来判断「是否能用对象存储」。要求总开关打开 + 关键凭证齐。
// 用于决定是否暴露上传接口。
func IsEnabled() bool {
	_, _, ok := getClient()
	return ok
}

// CredsReady 凭证是否齐（不看总开关）。admin 面板用来判断"测试连接"按钮
// 是否可用——开关没打开但凭证齐了，也可以测。
func CredsReady() bool {
	cfg := resolveConfig()
	return cfg.credsReady && cfg.publicBaseURL != ""
}

// Bucket 暴露当前桶名（用于日志 / 调试）；未启用时返回空字符串。
func Bucket() string {
	_, cfg, ok := getClient()
	if !ok {
		return ""
	}
	return cfg.bucket
}

// Provider 当前生效的存储提供方名（"r2" / 未来 "s3" / "minio" …）。
// 未启用时返回空字符串。前端可据此切换 UI。
func Provider() string {
	_, cfg, ok := getClient()
	if !ok {
		return ""
	}
	return cfg.provider
}

// sanitizePathPrefix 把外部传入的 pathPrefix 收敛到 [a-z0-9-_/] 集合。
// pathPrefix 由 controller 控制（不来自终端用户），但仍做防御性过滤：
//
//   - 静默丢弃 "."（杜绝 "../" 路径穿越）和其他特殊字符
//   - 多余的 "/" 折叠为单个；首尾 "/" 去掉
//   - 全部 trim 完后还是空 → 退回到 "misc"，避免 key 退化成 "/userID/..."
func sanitizePathPrefix(p string) string {
	p = strings.ToLower(strings.Trim(strings.TrimSpace(p), "/"))
	if p == "" {
		return "misc"
	}
	var b strings.Builder
	b.Grow(len(p))
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-' || r == '_' || r == '/':
			b.WriteRune(r)
		default:
			// 丢弃，含 "."（防 ../）/ 空格 / Unicode 等
		}
	}
	out := strings.Trim(b.String(), "/")
	for strings.Contains(out, "//") {
		out = strings.ReplaceAll(out, "//", "/")
	}
	if out == "" {
		return "misc"
	}
	return out
}

// extFromContentType 优先从 Content-Type 推扩展名；命中失败再退回到原始
// 文件名的扩展名；都没有就用 ".bin"。
func extFromContentType(contentType, filename string) string {
	if contentType != "" {
		// mime.ExtensionsByType 可能返回多个，挑第一个；返回值带 "."
		exts, _ := mime.ExtensionsByType(contentType)
		if len(exts) > 0 {
			return exts[0]
		}
	}
	if filename != "" {
		ext := strings.ToLower(path.Ext(filename))
		if ext != "" {
			return ext
		}
	}
	return ".bin"
}

// BuildObjectKey 拼出最终的对象 key。导出是为了让调用方能在 dry-run /
// 调试时预览 key 结构，不强制使用。
//
// 格式：{pathPrefix}/{userID}/{YYYY/MM/DD}/{uuid}{ext}
//
// pathPrefix 由 controller 决定（如 "playground/video/image"、
// "user-upload/image"），用户输入不可达；这里只做防御性 sanitize。
//
// 例：playground/video/image/123/2026/04/26/4f3e...d.png
func BuildObjectKey(pathPrefix string, userID int, contentType, filename string) string {
	pathPrefix = sanitizePathPrefix(pathPrefix)
	uid := strconv.Itoa(userID)
	if userID <= 0 {
		uid = "anonymous"
	}
	now := time.Now().UTC()
	datePart := now.Format("2006/01/02")
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	ext := extFromContentType(contentType, filename)
	return fmt.Sprintf("%s/%s/%s/%s%s", pathPrefix, uid, datePart, id, ext)
}

// PresignPut 给一次客户端直传生成预签名 PUT URL。
//
//   - pathPrefix: controller 决定的业务路径前缀，禁止任何用户输入直达
//   - size: 文件大小（字节），> 0；签进 URL，R2 会校验客户端实际 PUT 的
//     Content-Length 是否一致——客户端骗不了
//   - contentType: 同样签进 URL，客户端必须以同样的 Content-Type 发起 PUT
//   - ttl: URL 有效期；<=0 默认 5 分钟
//
// 失败语义：配置缺失 / SDK 签名失败均返回 error；不会落任何对象。
func PresignPut(
	ctx context.Context,
	pathPrefix string,
	userID int,
	filename string,
	contentType string,
	size int64,
	ttl time.Duration,
) (*PresignResult, error) {
	cli, cfg, ok := getClient()
	if !ok {
		return nil, errors.New("object storage is not configured")
	}
	if cfg.publicBaseURL == "" {
		// 没配 public base URL，签出来的 key 也访问不到
		return nil, errors.New("storage public base URL is not configured")
	}
	if size <= 0 {
		return nil, errors.New("size must be > 0")
	}
	if contentType == "" {
		// 从扩展名兜底推 MIME
		if ext := strings.ToLower(path.Ext(filename)); ext != "" {
			contentType = mime.TypeByExtension(ext)
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	key := BuildObjectKey(pathPrefix, userID, contentType, filename)

	presigner := s3.NewPresignClient(cli)
	signed, err := presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(cfg.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return nil, fmt.Errorf("storage presign put: %w", err)
	}

	// 把 SDK 算出来的签名 header 透传给客户端，让 JS 原样回写。Host /
	// Content-Length 浏览器会自动设置，且大多数浏览器禁止 JS 通过
	// setRequestHeader 显式设置——把它们过滤掉。
	headers := map[string]string{}
	for k, v := range signed.SignedHeader {
		if len(v) == 0 {
			continue
		}
		switch strings.ToLower(k) {
		case "host", "content-length":
			continue
		}
		headers[k] = v[0]
	}
	// 兜底：Content-Type 已签进 URL，前端必须发同样的值
	if _, ok := headers["Content-Type"]; !ok {
		headers["Content-Type"] = contentType
	}

	publicURL := cfg.publicBaseURL + "/" + (&url.URL{Path: key}).EscapedPath()

	return &PresignResult{
		UploadURL:   signed.URL,
		Method:      signed.Method,
		Headers:     headers,
		Key:         key,
		PublicURL:   publicURL,
		Bucket:      cfg.bucket,
		ExpiresAt:   time.Now().Add(ttl).Unix(),
		Size:        size,
		ContentType: contentType,
	}, nil
}

// TestStepResult 一个端到端测试步骤的结果。
type TestStepResult struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// TestResult "测试连接"按钮的回执。Success 为 true 当且仅当 Steps 全部 OK。
// 即使中途某步失败也会继续执行可选步骤（如 Delete 清理），保证不留垃圾对象。
type TestResult struct {
	Success       bool             `json:"success"`
	Bucket        string           `json:"bucket,omitempty"`
	TestKey       string           `json:"test_key,omitempty"`
	TestPublicURL string           `json:"test_public_url,omitempty"`
	Steps         []TestStepResult `json:"steps"`
}

// TestConnection 端到端验证当前对象存储配置是否真的可用。流程：
//
//  1. 检查凭证齐 + publicBaseURL 配置
//  2. 用 PresignPut 给一个 _test/{userID}/... 路径签 PUT URL
//  3. 用 net/http 真正 PUT 一段小字节流上去（mirror 客户端真实路径，
//     验证签名 / 凭证 / 桶权限）
//  4. 用 net/http GET 同一对象的 publicURL，比对内容（验证 publicBaseURL
//     正确指向同一桶 + 公开访问已生效）
//  5. 删除测试对象（无论前面是否成功，能删就删，不留垃圾）
//
// 注意：浏览器层面的 CORS 这里测不到——我们是服务器对服务器调 R2，
// 不会触发 CORS。CORS 错误得在前端测试时才能发现。
//
// 调用方：admin 控制器；不依赖总开关。
func TestConnection(ctx context.Context, userID int) *TestResult {
	out := &TestResult{Steps: make([]TestStepResult, 0, 4)}

	// Step 1: 配置完整性
	cfg := resolveConfig()
	if !cfg.credsReady {
		out.Steps = append(out.Steps, TestStepResult{
			Name: "config", OK: false,
			Message: "缺少 R2 凭证（access_key / secret / bucket / endpoint 任一项为空）",
		})
		return out
	}
	if cfg.publicBaseURL == "" {
		out.Steps = append(out.Steps, TestStepResult{
			Name: "config", OK: false,
			Message: "缺少 R2 公网访问基地址（R2_PUBLIC_BASE_URL）",
		})
		return out
	}
	out.Bucket = cfg.bucket
	out.Steps = append(out.Steps, TestStepResult{Name: "config", OK: true})

	// Step 2: 预签名
	const (
		testPrefix      = "_test"
		testContentType = "text/plain"
	)
	payload := []byte(fmt.Sprintf("amux-storage-test-%d", time.Now().UnixNano()))

	// 走 ForAdminTest 版本，跳过总开关
	cli, _, ok := getClientForAdminTest()
	if !ok {
		out.Steps = append(out.Steps, TestStepResult{
			Name: "presign", OK: false, Message: "无法初始化 S3 客户端",
		})
		return out
	}
	key := BuildObjectKey(testPrefix, userID, testContentType, "test.txt")
	out.TestKey = key
	publicURL := cfg.publicBaseURL + "/" + (&url.URL{Path: key}).EscapedPath()
	out.TestPublicURL = publicURL

	presigner := s3.NewPresignClient(cli)
	signed, err := presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(cfg.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(testContentType),
		ContentLength: aws.Int64(int64(len(payload))),
	}, s3.WithPresignExpires(2*time.Minute))
	if err != nil {
		out.Steps = append(out.Steps, TestStepResult{
			Name: "presign", OK: false, Message: "签名失败：" + err.Error(),
		})
		return out
	}
	out.Steps = append(out.Steps, TestStepResult{Name: "presign", OK: true})

	// 后续步骤不论成败都尝试清理；用 deferred cleanup 收尾
	cleaned := false
	cleanup := func() {
		if cleaned {
			return
		}
		cleaned = true
		// 用独立 context 避免上层 ctx 已 cancel 导致清理失败
		cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = cli.DeleteObject(cctx, &s3.DeleteObjectInput{
			Bucket: aws.String(cfg.bucket),
			Key:    aws.String(key),
		})
	}
	defer cleanup()

	httpClient := &http.Client{Timeout: 30 * time.Second}

	// Step 3: PUT 上传
	putReq, err := http.NewRequestWithContext(ctx, signed.Method, signed.URL, bytes.NewReader(payload))
	if err != nil {
		out.Steps = append(out.Steps, TestStepResult{
			Name: "put", OK: false, Message: "构造 PUT 请求失败：" + err.Error(),
		})
		return out
	}
	for k, v := range signed.SignedHeader {
		if len(v) == 0 {
			continue
		}
		// Host / Content-Length 由 net/http 自己处理
		switch strings.ToLower(k) {
		case "host", "content-length":
			continue
		}
		putReq.Header.Set(k, v[0])
	}
	if putReq.Header.Get("Content-Type") == "" {
		putReq.Header.Set("Content-Type", testContentType)
	}
	putReq.ContentLength = int64(len(payload))

	putResp, err := httpClient.Do(putReq)
	if err != nil {
		out.Steps = append(out.Steps, TestStepResult{
			Name: "put", OK: false, Message: "PUT 网络错误：" + err.Error(),
		})
		return out
	}
	putBody, _ := io.ReadAll(io.LimitReader(putResp.Body, 2048))
	_ = putResp.Body.Close()
	if putResp.StatusCode < 200 || putResp.StatusCode >= 300 {
		out.Steps = append(out.Steps, TestStepResult{
			Name: "put", OK: false,
			Message: fmt.Sprintf("R2 拒绝 PUT (HTTP %d): %s", putResp.StatusCode, truncate(string(putBody), 300)),
		})
		return out
	}
	out.Steps = append(out.Steps, TestStepResult{Name: "put", OK: true})

	// Step 4: 通过 publicURL 取回，验证内容一致
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, publicURL, nil)
	if err != nil {
		out.Steps = append(out.Steps, TestStepResult{
			Name: "public_get", OK: false, Message: "构造 GET 请求失败：" + err.Error(),
		})
		return out
	}
	getResp, err := httpClient.Do(getReq)
	if err != nil {
		out.Steps = append(out.Steps, TestStepResult{
			Name: "public_get", OK: false, Message: "通过 publicBaseURL 取回失败：" + err.Error(),
		})
		return out
	}
	getBody, _ := io.ReadAll(io.LimitReader(getResp.Body, int64(len(payload))+1024))
	_ = getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		out.Steps = append(out.Steps, TestStepResult{
			Name: "public_get", OK: false,
			Message: fmt.Sprintf("publicBaseURL 返回 HTTP %d，请检查 r2.dev / 自定义域名是否启用", getResp.StatusCode),
		})
		return out
	}
	if !bytes.Equal(bytes.TrimSpace(getBody), payload) {
		out.Steps = append(out.Steps, TestStepResult{
			Name: "public_get", OK: false,
			Message: "publicBaseURL 取回的内容与预期不符——可能指向了别的桶或被 CDN 缓存代理改写",
		})
		return out
	}
	out.Steps = append(out.Steps, TestStepResult{Name: "public_get", OK: true})

	// Step 5: cleanup 在 defer 里执行；这里只标记"清理已尝试"
	cleanup()
	out.Steps = append(out.Steps, TestStepResult{Name: "cleanup", OK: true})

	out.Success = true
	return out
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

// Delete 按 key 删除对象。多用于"用户主动撤回上传"或定时清理。
// 不存在的 key 不算错误（S3 DeleteObject 默认幂等）。
func Delete(ctx context.Context, key string) error {
	cli, cfg, ok := getClient()
	if !ok {
		return errors.New("object storage is not configured")
	}
	if key == "" {
		return errors.New("empty key")
	}
	_, err := cli.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(cfg.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage delete object: %w", err)
	}
	return nil
}
