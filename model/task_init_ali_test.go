package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestInitTaskStoresAliSubmissionKey(t *testing.T) {
	task := InitTask(constant.TaskPlatform("17"), &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeAli,
			ApiKey:      "ali-task-key",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	})
	if task.PrivateData.Key != "ali-task-key" {
		t.Fatalf("stored key=%q", task.PrivateData.Key)
	}
}
