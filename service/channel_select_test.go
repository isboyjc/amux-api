package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func newTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	return c
}

func TestIsClientValidationStatusCode(t *testing.T) {
	cases := map[int]bool{
		http.StatusBadRequest:            true,  // 400
		http.StatusUnprocessableEntity:   true,  // 422
		http.StatusRequestEntityTooLarge: true,  // 413
		http.StatusTooManyRequests:       false, // 429
		http.StatusInternalServerError:   false, // 500
		http.StatusNotFound:              false, // 404
		http.StatusBadGateway:            false, // 502
		http.StatusGatewayTimeout:        false, // 504
	}
	for code, want := range cases {
		if got := isClientValidationStatusCode(code); got != want {
			t.Errorf("isClientValidationStatusCode(%d) = %v, want %v", code, got, want)
		}
	}
}

func TestCrossGroupShouldFallback(t *testing.T) {
	errWith := func(status int, opts ...types.NewAPIErrorOptions) *types.NewAPIError {
		return types.NewErrorWithStatusCode(errors.New("boom"), types.ErrorCodeDoRequestFailed, status, opts...)
	}

	t.Run("nil error never falls back", func(t *testing.T) {
		if CrossGroupShouldFallback(newTestContext(), nil) {
			t.Fatal("nil error should not fall back")
		}
	})

	t.Run("channel-side errors fall back", func(t *testing.T) {
		for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusNotFound, http.StatusGatewayTimeout} {
			if !CrossGroupShouldFallback(newTestContext(), errWith(status)) {
				t.Errorf("status %d should fall back to next group", status)
			}
		}
	})

	t.Run("client validation errors do not fall back", func(t *testing.T) {
		for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity} {
			if CrossGroupShouldFallback(newTestContext(), errWith(status)) {
				t.Errorf("status %d should NOT fall back", status)
			}
		}
	})

	t.Run("skip-retry errors do not fall back", func(t *testing.T) {
		err := errWith(http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
		if CrossGroupShouldFallback(newTestContext(), err) {
			t.Fatal("skip-retry error should NOT fall back even with a 5xx status")
		}
	})

	t.Run("token bound to specific channel does not fall back", func(t *testing.T) {
		c := newTestContext()
		c.Set("specific_channel_id", "42")
		if CrossGroupShouldFallback(c, errWith(http.StatusInternalServerError)) {
			t.Fatal("specific_channel_id bound token should NOT cross-group fall back")
		}
	})
}

// withAutoGroupSetup configures auto groups + user usable groups and restores them after the test.
func withAutoGroupSetup(t *testing.T, autoGroupsJSON, usableGroupsJSON string) {
	t.Helper()
	prevAuto := setting.AutoGroups2JsonString()
	prevUsable := setting.UserUsableGroups2JSONString()
	if err := setting.UpdateAutoGroupsByJsonString(autoGroupsJSON); err != nil {
		t.Fatalf("set auto groups: %v", err)
	}
	if err := setting.UpdateUserUsableGroupsByJSONString(usableGroupsJSON); err != nil {
		t.Fatalf("set usable groups: %v", err)
	}
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(prevAuto)
		_ = setting.UpdateUserUsableGroupsByJSONString(prevUsable)
	})
}

func TestGetUserAutoGroupFiltersNotUsableGroups(t *testing.T) {
	// g3 is configured but the user's tier can't see it -> it must be dropped.
	withAutoGroupSetup(t, `["g1","g2","g3"]`, `{"g1":"G1","g2":"G2"}`)

	got := GetUserAutoGroup("default")
	want := []string{"g1", "g2"}
	if len(got) != len(want) {
		t.Fatalf("GetUserAutoGroup = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("GetUserAutoGroup = %v, want %v", got, want)
		}
	}
}

func TestAdvanceToNextAutoGroup(t *testing.T) {
	withAutoGroupSetup(t, `["g1","g2","g3"]`, `{"g1":"G1","g2":"G2","g3":"G3"}`)

	c := newTestContext()
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")

	// Start at group 0 (no index set yet).
	if !AdvanceToNextAutoGroup(c) {
		t.Fatal("expected advance from index 0 -> 1 to succeed")
	}
	if idx := common.GetContextKeyInt(c, constant.ContextKeyAutoGroupIndex); idx != 1 {
		t.Fatalf("index after first advance = %d, want 1", idx)
	}
	if !AdvanceToNextAutoGroup(c) {
		t.Fatal("expected advance from index 1 -> 2 to succeed")
	}
	if idx := common.GetContextKeyInt(c, constant.ContextKeyAutoGroupIndex); idx != 2 {
		t.Fatalf("index after second advance = %d, want 2", idx)
	}
	// index 2 is the last group -> no more groups.
	if AdvanceToNextAutoGroup(c) {
		t.Fatal("expected advance from last group to fail")
	}
}

func TestAdvanceToNextAutoGroupStopsAtFilteredListEnd(t *testing.T) {
	// Configured 3 groups but only 2 usable -> effective list length is 2.
	withAutoGroupSetup(t, `["g1","g2","g3"]`, `{"g1":"G1","g2":"G2"}`)

	c := newTestContext()
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyAutoGroupIndex, 1) // already on last usable group

	if AdvanceToNextAutoGroup(c) {
		t.Fatal("expected no advance past the last usable group")
	}
}
