package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const canvasRelayPrefix = "/api/canvas/relay"

func CanvasRelayRewritePath() gin.HandlerFunc {
	return func(c *gin.Context) {
		rewriteCanvasRelayPath(c)
		c.Next()
	}
}

func CanvasRelayAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := c.GetInt("id")
		userCache, err := model.GetUserCache(userId)
		if err != nil {
			common.ApiError(c, err)
			c.Abort()
			return
		}
		userCache.WriteContext(c)

		token, ok := ensureCanvasRelayToken(c, userId)
		if !ok {
			c.Abort()
			return
		}
		if err := middleware.SetupContextForToken(c, token); err != nil {
			c.Abort()
			return
		}
		common.SetContextKey(c, constant.ContextKeyUsingGroup, userCache.Group)
		c.Request.Header.Set("Authorization", "Bearer sk-"+token.GetFullKey())
		c.Next()
	}
}

func CanvasRelayOpenAI(c *gin.Context) {
	rewriteCanvasRelayPath(c)
	if c.Request.URL.Path == "" || c.Request.URL.Path == "/v1" || c.Request.URL.Path == "/v1/" {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "relay path is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}
	switch relayconstant.Path2RelayMode(c.Request.URL.Path) {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		Relay(c, types.RelayFormatOpenAIImage)
	case relayconstant.RelayModeAudioSpeech, relayconstant.RelayModeAudioTranscription, relayconstant.RelayModeAudioTranslation:
		Relay(c, types.RelayFormatOpenAIAudio)
	case relayconstant.RelayModeEmbeddings:
		Relay(c, types.RelayFormatEmbedding)
	case relayconstant.RelayModeResponses:
		Relay(c, types.RelayFormatOpenAIResponses)
	case relayconstant.RelayModeResponsesCompact:
		Relay(c, types.RelayFormatOpenAIResponsesCompaction)
	default:
		Relay(c, types.RelayFormatOpenAI)
	}
}

func CanvasRelayTask(c *gin.Context) {
	rewriteCanvasRelayPath(c)
	RelayTask(c)
}

func CanvasRelayTaskFetch(c *gin.Context) {
	rewriteCanvasRelayPath(c)
	RelayTaskFetch(c)
}

func CanvasRelayVideoContent(c *gin.Context) {
	rewriteCanvasRelayPath(c)
	VideoProxy(c)
}

func rewriteCanvasRelayPath(c *gin.Context) {
	if !strings.HasPrefix(c.Request.URL.Path, canvasRelayPrefix) {
		return
	}
	c.Request.URL.Path = "/v1" + canvasRelayPath(c)
}

func canvasRelayPath(c *gin.Context) string {
	path := strings.TrimPrefix(c.Request.URL.Path, canvasRelayPrefix)
	if path == "" {
		return "/"
	}
	return path
}
