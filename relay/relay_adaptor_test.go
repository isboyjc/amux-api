package relay

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	taskamuxstt "github.com/QuantumNous/new-api/relay/channel/task/amux_stt"
)

// TestGetTaskAdaptor_AmuxSTT guards the routing wiring that the isolated
// adaptor tests cannot cover: an Amux STT request must resolve to the
// amux_stt adaptor whether the platform is the "amux_stt" string (submit
// path, set by the distributor) or the numeric channel type "58"
// (any caller resolving by strconv.Itoa(channelType)).
func TestGetTaskAdaptor_AmuxSTT(t *testing.T) {
	cases := []struct {
		name     string
		platform constant.TaskPlatform
	}{
		{"string platform", constant.TaskPlatformAmuxSTT},
		{"numeric channel type", constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAmux))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adaptor := GetTaskAdaptor(tc.platform)
			if adaptor == nil {
				t.Fatalf("GetTaskAdaptor(%q) = nil, want *amux_stt.TaskAdaptor", tc.platform)
			}
			if _, ok := adaptor.(*taskamuxstt.TaskAdaptor); !ok {
				t.Fatalf("GetTaskAdaptor(%q) = %T, want *amux_stt.TaskAdaptor", tc.platform, adaptor)
			}
		})
	}
}
