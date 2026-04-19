package operation_setting

import "testing"

// restore 在每个用例结尾把 taskURLRewriteSetting 恢复成原值，避免测试串扰。
func withSettingSnapshot(t *testing.T, f func()) {
	t.Helper()
	orig := taskURLRewriteSetting
	defer func() { taskURLRewriteSetting = orig }()
	f()
}

func TestApplyTaskURLRewrite_DisabledNoop(t *testing.T) {
	withSettingSnapshot(t, func() {
		taskURLRewriteSetting = TaskURLRewriteSetting{
			Enabled: false,
			Rules: []TaskURLRewriteRule{
				{From: "https://resource.zerocut.cn/zerocut/", To: "https://r.amux.ai/zc/"},
			},
		}
		in := "https://resource.zerocut.cn/zerocut/foo.mp4"
		if got := ApplyTaskURLRewrite(in); got != in {
			t.Fatalf("disabled should be noop, got %q", got)
		}
	})
}

func TestApplyTaskURLRewrite_BasicPrefix(t *testing.T) {
	withSettingSnapshot(t, func() {
		taskURLRewriteSetting = TaskURLRewriteSetting{
			Enabled: true,
			Rules: []TaskURLRewriteRule{
				{From: "https://resource.zerocut.cn/zerocut/", To: "https://r.amux.ai/zc/"},
			},
		}
		in := "https://resource.zerocut.cn/zerocut/workflow/seedance-2.0/abc/x.mp4"
		want := "https://r.amux.ai/zc/workflow/seedance-2.0/abc/x.mp4"
		if got := ApplyTaskURLRewrite(in); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestApplyTaskURLRewrite_NoMatch(t *testing.T) {
	withSettingSnapshot(t, func() {
		taskURLRewriteSetting = TaskURLRewriteSetting{
			Enabled: true,
			Rules: []TaskURLRewriteRule{
				{From: "https://resource.zerocut.cn/zerocut/", To: "https://r.amux.ai/zc/"},
			},
		}
		in := "https://other-upstream.example.com/video.mp4"
		if got := ApplyTaskURLRewrite(in); got != in {
			t.Fatalf("unrelated URL should be untouched, got %q", got)
		}
	})
}

func TestApplyTaskURLRewrite_SkipNonHTTP(t *testing.T) {
	withSettingSnapshot(t, func() {
		taskURLRewriteSetting = TaskURLRewriteSetting{
			Enabled: true,
			Rules: []TaskURLRewriteRule{
				{From: "data:", To: "http://evil/"},
			},
		}
		in := "data:video/mp4;base64,AAAA..."
		if got := ApplyTaskURLRewrite(in); got != in {
			t.Fatalf("data: URI must be skipped, got %q", got)
		}
	})
}

func TestApplyTaskURLRewrite_LongestPrefixWins(t *testing.T) {
	withSettingSnapshot(t, func() {
		taskURLRewriteSetting = TaskURLRewriteSetting{
			Enabled: true,
			Rules: []TaskURLRewriteRule{
				// 短前缀，如果顺序优先会错配
				{From: "https://resource.zerocut.cn/", To: "https://wrong/zc-root/"},
				// 长前缀应胜出
				{From: "https://resource.zerocut.cn/zerocut/", To: "https://r.amux.ai/zc/"},
			},
		}
		in := "https://resource.zerocut.cn/zerocut/x.mp4"
		want := "https://r.amux.ai/zc/x.mp4"
		if got := ApplyTaskURLRewrite(in); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestApplyTaskURLRewrite_EmptyInput(t *testing.T) {
	withSettingSnapshot(t, func() {
		taskURLRewriteSetting = TaskURLRewriteSetting{Enabled: true, Rules: []TaskURLRewriteRule{{From: "https://x/", To: "https://y/"}}}
		if got := ApplyTaskURLRewrite(""); got != "" {
			t.Fatalf("empty should stay empty, got %q", got)
		}
	})
}

func TestApplyTaskURLRewrite_IgnoresEmptyRule(t *testing.T) {
	withSettingSnapshot(t, func() {
		taskURLRewriteSetting = TaskURLRewriteSetting{
			Enabled: true,
			Rules: []TaskURLRewriteRule{
				{From: "", To: "https://absorb-all/"}, // 空 From 必须跳过，不能吞掉全部 URL
				{From: "https://a/", To: "https://b/"},
			},
		}
		in := "https://foo/bar"
		if got := ApplyTaskURLRewrite(in); got != in {
			t.Fatalf("empty From must be ignored, got %q", got)
		}
	})
}
