package storage

import "testing"

// TestExtFromContentType 上传对象的后缀会成为素材 URL 的一部分，被上游按
// 白名单校验。mime.ExtensionsByType 的首选值对媒体类型不可用：
// video/mp4→.m4v、image/jpeg→.jfif、audio/mpeg→.m2a，都会被上游拒收。
func TestExtFromContentType(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		filename    string
		want        string
	}{
		// 文件名后缀优先——那是用户实际持有的后缀
		{"文件名优先于 MIME", "video/mp4", "clip.mp4", ".mp4"},
		{"文件名大写归一", "video/mp4", "CLIP.MP4", ".mp4"},
		{"mov 保持 mov", "video/quicktime", "clip.mov", ".mov"},

		// 无文件名时走规范表，绝不能落到 mime 首选值
		{"video/mp4 不能是 m4v", "video/mp4", "", ".mp4"},
		{"image/jpeg 不能是 jfif", "image/jpeg", "", ".jpg"},
		{"audio/mpeg 不能是 m2a", "audio/mpeg", "", ".mp3"},
		{"image/png", "image/png", "", ".png"},
		{"image/webp", "image/webp", "", ".webp"},
		{"audio/wav", "audio/wav", "", ".wav"},
		{"video/quicktime", "video/quicktime", "", ".mov"},

		// Content-Type 带参数时要能解析出主体
		{"带 codecs 参数", "video/mp4; codecs=avc1.42E01E", "", ".mp4"},

		// 兜底
		{"都没有", "", "", ".bin"},
		{"未知类型无文件名", "application/x-nonexistent-type", "", ".bin"},
		{"超长伪后缀不当扩展名", "video/mp4", "archive.tar.verylongsuffix", ".mp4"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extFromContentType(tc.contentType, tc.filename); got != tc.want {
				t.Errorf("extFromContentType(%q, %q) = %q, want %q",
					tc.contentType, tc.filename, got, tc.want)
			}
		})
	}
}

// TestExtFromContentType_NeverReturnsRejectedExts 兜底防线：任何常见媒体
// 类型都不能推出上游白名单外的后缀。
func TestExtFromContentType_NeverReturnsRejectedExts(t *testing.T) {
	rejected := map[string]bool{".m4v": true, ".jfif": true, ".m2a": true, ".f4v": true, ".mpg4": true}
	for _, ct := range []string{
		"video/mp4", "video/quicktime", "image/jpeg", "image/png",
		"image/webp", "audio/mpeg", "audio/wav",
	} {
		if got := extFromContentType(ct, ""); rejected[got] {
			t.Errorf("%s → %q，该后缀会被上游拒收", ct, got)
		}
	}
}
