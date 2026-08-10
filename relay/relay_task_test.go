package relay

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
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

func TestTaskQuotaWithRatios(t *testing.T) {
	if got, want := taskQuotaWithRatios(500_000, map[string]float64{"video_cost": 0.84}), 420_000; got != want {
		t.Fatalf("taskQuotaWithRatios=%d, want %d", got, want)
	}
}

func TestBuildSimpleVideoTaskResponseHidesNonTerminalData(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusInProgress,
		Data:   []byte(`{"output":{"task_id":"upstream-id","video_url":"https://upstream.example/video.mp4"}}`),
	}
	body := string(buildSimpleVideoTaskResponse(task, "mp4"))
	if strings.Contains(body, "upstream-id") || strings.Contains(body, "upstream.example") {
		t.Fatalf("simple task response leaked upstream data: %s", body)
	}
	if !strings.Contains(body, `"task_id":"task_public"`) || !strings.Contains(body, `"url":""`) {
		t.Fatalf("unexpected simple task response: %s", body)
	}
}

func TestBuildAliUnknownTaskResponse(t *testing.T) {
	body := string(buildAliUnknownTaskResponse("missing-task", "request-id"))
	for _, fragment := range []string{`"task_id":"missing-task"`, `"task_status":"UNKNOWN"`, `"request_id":"request-id"`} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("UNKNOWN response missing %s: %s", fragment, body)
		}
	}
}

func TestBuildAliVideoFetchResponseRejectsNonAliTask(t *testing.T) {
	task := &model.Task{
		TaskID:   "public-task",
		Platform: constant.TaskPlatform("other-provider"),
		Data:     []byte(`{"output":{"task_id":"upstream-id","video_url":"https://upstream.example/video.mp4"}}`),
	}
	body, err := buildAliVideoFetchResponse(task, task.TaskID, "request-id")
	if err != nil {
		t.Fatalf("buildAliVideoFetchResponse: %v", err)
	}
	response := string(body)
	for _, secret := range []string{"upstream-id", "upstream.example"} {
		if strings.Contains(response, secret) {
			t.Fatalf("DashScope response leaked non-Ali task data %q: %s", secret, response)
		}
	}
	for _, fragment := range []string{`"task_id":"public-task"`, `"task_status":"UNKNOWN"`, `"request_id":"request-id"`} {
		if !strings.Contains(response, fragment) {
			t.Fatalf("DashScope UNKNOWN response missing %s: %s", fragment, response)
		}
	}
}

func TestTaskPollingKeyPrefersSubmissionKey(t *testing.T) {
	task := &model.Task{}
	task.PrivateData.Key = "submission-key"
	if got := taskPollingKey(task, "channel-default"); got != "submission-key" {
		t.Fatalf("taskPollingKey=%q", got)
	}
	task.PrivateData.Key = ""
	if got := taskPollingKey(task, "channel-default"); got != "channel-default" {
		t.Fatalf("fallback taskPollingKey=%q", got)
	}
}
