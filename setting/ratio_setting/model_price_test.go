package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelPriceIncludesAgnesVideoV20(t *testing.T) {
	price, ok := GetDefaultModelPriceMap()["agnes-video-v2.0"]
	require.True(t, ok)
	require.Equal(t, 0.0, price)
}
