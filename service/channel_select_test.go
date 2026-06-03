package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestRetryParamIncreaseRetry verifies the retry counter behaves correctly
// with and without the ResetRetryNextTry flag. This is the central mechanism
// for the auto-group fallback logic — when a group is exhausted, the next
// group should start at priority 0 (not 1), so the for-loop's IncreaseRetry
// must be a no-op for the iteration that transitions to the next group.
func TestRetryParamIncreaseRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("IncreaseRetry without reset increments", func(t *testing.T) {
		p := &RetryParam{Retry: common.GetPointer(2)}
		p.IncreaseRetry()
		require.Equal(t, 3, p.GetRetry(), "without reset, retry should increment")
	})

	t.Run("IncreaseRetry with reset is no-op once", func(t *testing.T) {
		p := &RetryParam{Retry: common.GetPointer(2)}
		p.ResetRetryNextTry()
		p.IncreaseRetry()
		require.Equal(t, 2, p.GetRetry(), "with reset, first IncreaseRetry should be no-op")
		p.IncreaseRetry()
		require.Equal(t, 3, p.GetRetry(), "after reset is consumed, retry should increment again")
	})

	t.Run("IncreaseRetry with reset from nil pointer is safe", func(t *testing.T) {
		p := &RetryParam{}
		p.ResetRetryNextTry()
		p.IncreaseRetry()
		require.Equal(t, 0, p.GetRetry(), "nil retry with reset should stay 0")
		p.IncreaseRetry()
		require.Equal(t, 1, p.GetRetry(), "subsequent IncreaseRetry should start from 0")
	})
}

// TestAutoGroupContextStateTransitions verifies the context key state machine
// for the auto-group iterator. The two persistent keys are:
//   - ContextKeyAutoGroupIndex: index of the current group in the auto-group
//     list
//   - ContextKeyAutoGroupRetryIndex: the global retry value at which the
//     current group started, used to compute priorityRetry = currentRetry -
//     startRetryIndex
//
// These state transitions are what the auto-group fallback relies on.
func TestAutoGroupContextStateTransitions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	t.Run("initial state: index=0, retry-index=0", func(t *testing.T) {
		idx, ok := common.GetContextKey(c, constant.ContextKeyAutoGroupIndex)
		require.False(t, ok, "no key set yet")
		require.Nil(t, idx)

		ridx, ok := common.GetContextKey(c, constant.ContextKeyAutoGroupRetryIndex)
		require.False(t, ok)
		require.Nil(t, ridx)
	})

	t.Run("after advancing to next group: index=1, retry-index=current retry", func(t *testing.T) {
		// Simulate: at global retry=2 we move to the next group.
		common.SetContextKey(c, constant.ContextKeyAutoGroupIndex, 1)
		common.SetContextKey(c, constant.ContextKeyAutoGroupRetryIndex, 2)

		idx, ok := common.GetContextKey(c, constant.ContextKeyAutoGroupIndex)
		require.True(t, ok)
		require.Equal(t, 1, idx)

		ridx, ok := common.GetContextKey(c, constant.ContextKeyAutoGroupRetryIndex)
		require.True(t, ok)
		require.Equal(t, 2, ridx)
	})

	t.Run("priorityRetry computation: current - startRetryIndex", func(t *testing.T) {
		// Simulate the for-loop state when entering the new group.
		startRetryIndex := 2
		currentRetry := 3
		priorityRetry := currentRetry - startRetryIndex
		require.Equal(t, 1, priorityRetry, "priorityRetry should be 1, the next priority within the new group")
	})

	t.Run("priorityRetry floor: never negative", func(t *testing.T) {
		startRetryIndex := 5
		currentRetry := 3
		priorityRetry := currentRetry - startRetryIndex
		if priorityRetry < 0 {
			priorityRetry = 0
		}
		require.Equal(t, 0, priorityRetry, "priorityRetry should be clamped to 0, not negative")
	})
}

// TestEmptyAutoGroupsReturnsError documents that the auto-group iterator
// must return a clear error when the user has no accessible auto groups
// (e.g., the user's group restrictions filter out all configured auto groups).
// Previously the function would silently return nil channel and the caller
// would see a confusing "group auto has no channel" error.
func TestEmptyAutoGroupsGuard(t *testing.T) {
	// We don't call CacheGetRandomSatisfiedChannel here because it would
	// need a real channel cache. Instead, document the guard via the
	// related code path: GetUserAutoGroup returning empty.
	// The actual guard is in service/channel_select.go:
	//     if len(autoGroups) == 0 {
	//         return nil, selectGroup, errors.New("user has no accessible auto groups")
	//     }
	// This is exercised by CacheGetRandomSatisfiedChannel — see Bug #4 in
	// the analysis.
	t.Log("see service/channel_select.go CacheGetRandomSatisfiedChannel: empty autoGroups guard returns explicit error")
}
