package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPollAdaptor drives updateVideoSingleTask to its success branch:
// FetchTask returns a body that is NOT a new-api TaskResponse (so the code
// falls back to ParseTaskResult), and ParseTaskResult reports success with
// no media URL — exactly how the Amux STT adaptor behaves.
type mockPollAdaptor struct{}

func (m *mockPollAdaptor) Init(_ *relaycommon.RelayInfo) {}

func (m *mockPollAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"status":"done"}`))),
		Header:     http.Header{"Content-Type": {"application/json"}},
	}, nil
}

func (m *mockPollAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{TaskID: "upstream_x", Status: model.TaskStatusSuccess, Url: ""}, nil
}

func (m *mockPollAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int { return 0 }

type non2xxPollAdaptor struct {
	parseCalled bool
}

func (m *non2xxPollAdaptor) Init(_ *relaycommon.RelayInfo) {}

func (m *non2xxPollAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"code":"Throttling"}`))),
	}, nil
}

func (m *non2xxPollAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	m.parseCalled = true
	return &relaycommon.TaskInfo{Status: model.TaskStatusFailure}, nil
}

func (m *non2xxPollAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func runPollOnce(t *testing.T, channelType int) *model.Task {
	t.Helper()
	truncate(t)

	const channelID = 70
	task := makeTask(1, channelID, 1000, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	ch := &model.Channel{Id: channelID, Type: channelType, Key: "asr_test"}
	taskM := map[string]*model.Task{task.TaskID: task}

	require.NoError(t, updateVideoSingleTask(context.Background(), &mockPollAdaptor{}, ch, task.TaskID, taskM))
	return task
}

// Amux (STT) tasks have no media product — result is inline in data, so no
// video proxy URL should be fabricated.
func TestUpdateVideoSingleTask_AmuxNoResultURL(t *testing.T) {
	task := runPollOnce(t, constant.ChannelTypeAmux)

	assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
	assert.Empty(t, task.PrivateData.ResultURL, "Amux STT task must not get a fabricated video proxy URL")
}

// Control: a regular video channel returning no direct URL still gets the
// proxy URL fallback — the fix must not regress video channels.
func TestUpdateVideoSingleTask_VideoChannelHasResultURL(t *testing.T) {
	task := runPollOnce(t, constant.ChannelTypeKling)

	assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
	require.NotEmpty(t, task.PrivateData.ResultURL, "video channel should still get a proxy URL")
	assert.Contains(t, task.PrivateData.ResultURL, "/v1/videos/"+task.TaskID+"/content")
}

func TestUpdateVideoSingleTask_Non2xxDoesNotTerminalizeTask(t *testing.T) {
	adaptor := &non2xxPollAdaptor{}
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusInProgress,
	}
	task.PrivateData.UpstreamTaskID = "upstream_id"
	tasks := map[string]*model.Task{"upstream_id": task}

	err := updateVideoSingleTask(context.Background(), adaptor, &model.Channel{Type: constant.ChannelTypeAli}, "upstream_id", tasks)
	require.Error(t, err)
	assert.False(t, adaptor.parseCalled)
	assert.EqualValues(t, model.TaskStatusInProgress, task.Status)
}
