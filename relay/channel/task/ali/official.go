package ali

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func mapTaskStatusToAli(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusNotStart, model.TaskStatusSubmitted, model.TaskStatusQueued:
		return "PENDING"
	case model.TaskStatusInProgress:
		return "RUNNING"
	case model.TaskStatusSuccess:
		return "SUCCEEDED"
	case model.TaskStatusFailure:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// ConvertToAliVideo 把网关内部任务转换为 DashScope 官方任务查询结构。
// task_id 始终使用网关公开 ID，成功 URL 使用归档/脱敏后的结果地址。
func (a *TaskAdaptor) ConvertToAliVideo(originTask *model.Task) ([]byte, error) {
	var response AliVideoResponse
	if len(originTask.Data) > 0 {
		_ = common.Unmarshal(originTask.Data, &response)
	}

	storedStatus := strings.ToUpper(strings.TrimSpace(response.Output.TaskStatus))
	response.Output.TaskID = originTask.TaskID
	response.Output.TaskStatus = mapTaskStatusToAli(originTask.Status)
	if originTask.Status == model.TaskStatusFailure {
		switch storedStatus {
		case "FAILED", "CANCELED", "UNKNOWN":
			response.Output.TaskStatus = storedStatus
		}
	}
	// 归档或 URL 重写完成前不返回上游临时直链。只有网关内部任务已经成功时，
	// 才允许把持久化/脱敏后的结果 URL 放入官方协议响应。
	response.Output.VideoURL = ""
	if originTask.Status == model.TaskStatusSuccess {
		if resultURL := originTask.GetResultURL(); resultURL != "" {
			response.Output.VideoURL = resultURL
		} else {
			// 未启用归档且旧任务尚未写入 ResultURL 时，兼容存量上游结果。
			var stored AliVideoResponse
			if err := common.Unmarshal(originTask.Data, &stored); err == nil {
				response.Output.VideoURL = stored.Output.VideoURL
			}
		}
		response.Output.Code = ""
		response.Output.Message = ""
	}
	if originTask.Status == model.TaskStatusFailure {
		if response.Output.TaskStatus == "FAILED" {
			if response.Output.Code == "" {
				response.Output.Code = "TaskFailed"
			}
			if response.Output.Message == "" {
				response.Output.Message = originTask.FailReason
			}
		} else {
			response.Output.Code = ""
			response.Output.Message = ""
		}
	}
	response.Code = ""
	response.Message = ""

	return common.Marshal(response)
}
