package relay

import (
	"net/http"
	"testing"
)

// TestTaskSubmitStatusOK_Non200Success guards the widened success criterion:
// async task upstreams often reply with a non-200 2xx (e.g. 201/202/204) on
// submit, and all of them must be treated as success — while non-2xx codes
// must still be rejected so video/Suno/MJ task channels don't regress.
func TestTaskSubmitStatusOK_Non200Success(t *testing.T) {
	cases := []struct {
		statusCode int
		wantOK     bool
	}{
		{http.StatusOK, true},               // 200
		{http.StatusCreated, true},          // 201
		{http.StatusAccepted, true},         // 202 — canonical async submit
		{http.StatusNoContent, true},        // 204
		{299, true},                         // upper 2xx boundary
		{http.StatusContinue, false},        // 100
		{199, false},                        // just below 2xx
		{http.StatusMultipleChoices, false}, // 300 — lower bound of rejection
		{http.StatusBadRequest, false},      // 400
		{http.StatusInternalServerError, false},
	}
	for _, tc := range cases {
		if got := taskSubmitStatusOK(tc.statusCode); got != tc.wantOK {
			t.Errorf("taskSubmitStatusOK(%d) = %v, want %v", tc.statusCode, got, tc.wantOK)
		}
	}
}
