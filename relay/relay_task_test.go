package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsOpenAIVideoRequestUsesRewrittenURLPathForCanvasRelay(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/canvas/relay/videos/task_123?model=agnes-video-v2.0", nil)
	c.Request.URL.Path = "/v1/videos/task_123"

	require.True(t, isOpenAIVideoRequest(c))
}
