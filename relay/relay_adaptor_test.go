package relay

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	taskagnesai "github.com/QuantumNous/new-api/relay/channel/task/agnesai"

	"github.com/stretchr/testify/require"
)

func TestGetTaskAdaptorUsesAgnesAIAdaptorForAgnesAIVideo(t *testing.T) {
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAgnesAI)))
	require.IsType(t, &taskagnesai.TaskAdaptor{}, adaptor)
}
