package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestAliVideoEndpointTypes(t *testing.T) {
	for _, modelName := range []string{
		"happyhorse-1.1-t2v",
		"happyhorse-1.0-t2v",
		"wan2.7-i2v-2026-04-25",
	} {
		got := GetEndpointTypesByChannelType(constant.ChannelTypeAli, modelName)
		if len(got) != 1 || got[0] != constant.EndpointTypeOpenAIVideo {
			t.Fatalf("endpoint types for %s=%v", modelName, got)
		}
	}
	got := GetEndpointTypesByChannelType(constant.ChannelTypeAli, "qwen-plus")
	if len(got) != 1 || got[0] != constant.EndpointTypeOpenAI {
		t.Fatalf("qwen-plus endpoint types=%v", got)
	}
}
