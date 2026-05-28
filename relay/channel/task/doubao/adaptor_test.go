package doubao

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// zeroCutSuccessBody 是 ZeroCut 聚合器返回的真实成功响应（脱去无关字段后保留结构）。
const zeroCutSuccessBody = `{
  "code": 200,
  "message": "获取工作流状态成功",
  "data": {
    "id": 11662,
    "type": "seedance-2.0-api",
    "status": "SUCCESS",
    "output": {
      "url": "https://resource.zerocut.cn/upstream-original.mp4",
      "ratio": "16:9",
      "usage": {
        "credits": 75,
        "total_tokens": 108900,
        "completion_tokens": 108900
      },
      "duration": 5,
      "resolution": "720p",
      "revised_prompt": "猫在跳跃"
    },
    "created_at": "2026-05-27T15:09:55.781Z",
    "updated_at": "2026-05-27T15:14:27.556Z"
  }
}`

func TestConvertToDoubaoV3_SuccessZeroCut(t *testing.T) {
	a := &TaskAdaptor{}
	task := &model.Task{
		TaskID:           "task_abc123",
		Status:           model.TaskStatusSuccess,
		CompletionTokens: 108900,
		TotalTokens:      108900,
		CreatedAt:        1000,
		UpdatedAt:        2000,
		Data:             []byte(zeroCutSuccessBody),
	}
	task.Properties.OriginModelName = "doubao-seedance-2-0-260128"
	// 模拟脱敏/代理后的结果 URL（权威来源是 PrivateData.ResultURL）
	task.PrivateData.ResultURL = "https://proxy.example.com/video.mp4"

	out, err := a.ConvertToDoubaoV3(task)
	if err != nil {
		t.Fatalf("ConvertToDoubaoV3 error: %v", err)
	}

	var v dto.DoubaoV3Video
	if err := common.Unmarshal(out, &v); err != nil {
		t.Fatalf("unmarshal result error: %v", err)
	}

	if v.ID != "task_abc123" {
		t.Errorf("id = %q, want gateway public task id", v.ID)
	}
	if v.Status != dto.DoubaoV3StatusSucceeded {
		t.Errorf("status = %q, want %q", v.Status, dto.DoubaoV3StatusSucceeded)
	}
	if v.Model != "doubao-seedance-2-0-260128" {
		t.Errorf("model = %q", v.Model)
	}
	if v.Content == nil || v.Content.VideoURL != "https://proxy.example.com/video.mp4" {
		t.Errorf("content.video_url should be the proxied URL, got %+v", v.Content)
	}
	if v.Usage == nil || v.Usage.CompletionTokens != 108900 || v.Usage.TotalTokens != 108900 {
		t.Errorf("usage = %+v, want completion/total = 108900", v.Usage)
	}
	// 来自 ZeroCut output 的元数据透传
	if v.Resolution != "720p" || v.Duration != 5 || v.Ratio != "16:9" {
		t.Errorf("metadata mismatch: resolution=%q duration=%d ratio=%q", v.Resolution, v.Duration, v.Ratio)
	}
	if v.CreatedAt != 1000 || v.UpdatedAt != 2000 {
		t.Errorf("timestamps should use gateway unix values, got created=%d updated=%d", v.CreatedAt, v.UpdatedAt)
	}
	if v.Error != nil {
		t.Errorf("success task should not carry error, got %+v", v.Error)
	}
}

func TestConvertToDoubaoV3_Failure(t *testing.T) {
	a := &TaskAdaptor{}
	body := `{"code":200,"data":{"id":1,"status":"FAILED","output":{"error":"content policy violation"}}}`
	task := &model.Task{
		TaskID:     "task_fail",
		Status:     model.TaskStatusFailure,
		FailReason: "fallback reason",
		Data:       []byte(body),
	}

	out, err := a.ConvertToDoubaoV3(task)
	if err != nil {
		t.Fatalf("ConvertToDoubaoV3 error: %v", err)
	}
	var v dto.DoubaoV3Video
	if err := common.Unmarshal(out, &v); err != nil {
		t.Fatalf("unmarshal result error: %v", err)
	}

	if v.Status != dto.DoubaoV3StatusFailed {
		t.Errorf("status = %q, want failed", v.Status)
	}
	if v.Content != nil {
		t.Errorf("failed task should not carry content, got %+v", v.Content)
	}
	if v.Error == nil || v.Error.Message != "content policy violation" {
		t.Errorf("error should come from upstream output.error, got %+v", v.Error)
	}
}

func TestConvertToDoubaoV3_InProgress(t *testing.T) {
	a := &TaskAdaptor{}
	task := &model.Task{
		TaskID: "task_running",
		Status: model.TaskStatusInProgress,
	}
	out, err := a.ConvertToDoubaoV3(task)
	if err != nil {
		t.Fatalf("ConvertToDoubaoV3 error: %v", err)
	}
	var v dto.DoubaoV3Video
	if err := common.Unmarshal(out, &v); err != nil {
		t.Fatalf("unmarshal result error: %v", err)
	}

	if v.Status != dto.DoubaoV3StatusRunning {
		t.Errorf("status = %q, want running", v.Status)
	}
	if v.Content != nil {
		t.Errorf("in-progress task should not carry content, got %+v", v.Content)
	}
	if v.Usage != nil {
		t.Errorf("in-progress task should not carry usage, got %+v", v.Usage)
	}
}
