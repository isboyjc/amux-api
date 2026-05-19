package resend

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/service/events"

	rd "github.com/resend/resend-go/v3"
)

func TestClassifyErr_Nil(t *testing.T) {
	if err := classifyErr(nil, true); err != nil {
		t.Fatalf("nil should be nil, got %v", err)
	}
}

func TestClassifyErr_RateLimitIsTransient(t *testing.T) {
	rateErr := &rd.RateLimitError{Message: "too many"}
	got := classifyErr(rateErr, false)
	if got == nil {
		t.Fatal("rate limit should not be nil")
	}
	if errors.Is(got, events.ErrPermanent) {
		t.Fatal("rate limit should not be permanent")
	}
}

func TestClassifyErr_NotFoundIgnored(t *testing.T) {
	cases := []string{
		"[ERROR]: Contact not found",
		"[ERROR]: 404 not found",
		"[ERROR]: Not Found",
	}
	for _, msg := range cases {
		if err := classifyErr(errors.New(msg), true); err != nil {
			t.Errorf("with ignoreNotFound=true, %q should be nil; got %v", msg, err)
		}
	}
}

func TestClassifyErr_NotFoundNotIgnored(t *testing.T) {
	err := errors.New("[ERROR]: Contact not found")
	got := classifyErr(err, false)
	if got == nil {
		t.Fatal("with ignoreNotFound=false, should pass through err")
	}
}

func TestClassifyErr_AuthErrorsArePermanent(t *testing.T) {
	cases := []string{
		"[ERROR]: Invalid API key",
		"[ERROR]: Unauthorized",
		"[ERROR]: 401 Unauthorized",
		"[ERROR]: 403 Forbidden",
		"[ERROR]: Forbidden",
	}
	for _, msg := range cases {
		got := classifyErr(errors.New(msg), false)
		if !errors.Is(got, events.ErrPermanent) {
			t.Errorf("%q should be ErrPermanent; got %v", msg, got)
		}
	}
}

func TestClassifyErr_UnknownErrorIsTransient(t *testing.T) {
	// 5xx / 网络错 / 其他未识别错 → 返回原 err，让 worker 重试
	err := errors.New("[ERROR]: Internal server error")
	got := classifyErr(err, false)
	if got == nil {
		t.Fatal("should not be nil")
	}
	if errors.Is(got, events.ErrPermanent) {
		t.Fatal("unknown server error should be transient (worker can retry)")
	}
}

func TestErrIsAlreadyExists(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"[ERROR]: Contact already exists", true},
		{"[ERROR]: contact_already_exists", true},
		{"[ERROR]: Email already_exists", true},
		{"[ERROR]: not found", false},
		{"", false},
	}
	for _, c := range cases {
		var err error
		if c.msg != "" {
			err = errors.New(c.msg)
		}
		if errIsAlreadyExists(err) != c.want {
			t.Errorf("errIsAlreadyExists(%q) = %v, want %v", c.msg, !c.want, c.want)
		}
	}
}

func TestSplitName(t *testing.T) {
	cases := []struct {
		in            string
		first, last   string
	}{
		{"", "", ""},
		{"Alice", "Alice", ""},
		{"Alice Smith", "Alice", "Smith"},
		{"Alice Van Der Berg", "Alice", "Van Der Berg"},
		{"   Bob   ", "Bob", ""},
		{"  Bob  Smith  ", "Bob", "Smith"},
	}
	for _, c := range cases {
		first, last := splitName(c.in)
		if first != c.first || last != c.last {
			t.Errorf("splitName(%q) = (%q, %q), want (%q, %q)", c.in, first, last, c.first, c.last)
		}
	}
}
