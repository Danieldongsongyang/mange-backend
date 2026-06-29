package agnesai

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLAvoidsDuplicatedV1(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RequestURLPath: "/v1/chat/completions",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://apihub.agnes-ai.com/v1",
		},
	}

	requestURL, err := adaptor.GetRequestURL(info)

	require.NoError(t, err)
	require.Equal(t, "https://apihub.agnes-ai.com/v1/chat/completions", requestURL)
}

func TestConvertOpenAIRequestPreservesStreamOptions(t *testing.T) {
	adaptor := &Adaptor{}
	req := &dto.GeneralOpenAIRequest{
		Model:  "agnes-2.0-flash",
		Stream: ptr(true),
		StreamOptions: &dto.StreamOptions{
			IncludeUsage: true,
		},
	}

	converted, err := adaptor.ConvertOpenAIRequest(nil, &relaycommon.RelayInfo{}, req)

	require.NoError(t, err)
	convertedReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, convertedReq.StreamOptions)
	require.True(t, convertedReq.StreamOptions.IncludeUsage)
}

func TestConvertImageRequestMovesResponseFormatToExtraBody(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
	}
	req := dto.ImageRequest{
		Model:          "agnes-image-2.1-flash",
		Prompt:         "a neon city",
		ResponseFormat: "b64_json",
	}

	converted, err := adaptor.ConvertImageRequest(nil, info, req)

	require.NoError(t, err)
	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.NotContains(t, payload, "response_format")
	extraBody, ok := payload["extra_body"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "b64_json", extraBody["response_format"])
}

func TestConvertImageEditRequestMultipartToGenerationsPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "agnes-image-2.1-flash"))
	require.NoError(t, writer.WriteField("prompt", "turn this sketch into a watercolor illustration"))
	require.NoError(t, writer.WriteField("response_format", "b64_json"))
	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake image bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
	}
	req := dto.ImageRequest{
		Model:  "agnes-image-2.1-flash",
		Prompt: "turn this sketch into a watercolor illustration",
	}

	converted, err := adaptor.ConvertImageRequest(c, info, req)

	require.NoError(t, err)
	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "agnes-image-2.1-flash", payload["model"])
	require.Equal(t, "turn this sketch into a watercolor illustration", payload["prompt"])
	require.NotContains(t, payload, "image")
	extraBody, ok := payload["extra_body"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "b64_json", extraBody["response_format"])
	images, ok := extraBody["image"].([]string)
	require.True(t, ok)
	require.Len(t, images, 1)
	require.Contains(t, images[0], "data:")
	require.Contains(t, images[0], ";base64,")
}

func TestConvertImageEditRequestMultipartArrayDoesNotDuplicateFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "agnes-image-2.1-flash"))
	require.NoError(t, writer.WriteField("prompt", "blend these references"))
	for _, content := range []string{"first image bytes", "second image bytes"} {
		part, err := writer.CreateFormFile("image[]", "input.png")
		require.NoError(t, err)
		_, err = part.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	converted, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
	}, dto.ImageRequest{
		Model:  "agnes-image-2.1-flash",
		Prompt: "blend these references",
	})

	require.NoError(t, err)
	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	extraBody, ok := payload["extra_body"].(map[string]any)
	require.True(t, ok)
	images, ok := extraBody["image"].([]string)
	require.True(t, ok)
	require.Len(t, images, 2)
}

func TestConvertImageEditRequestMultipartTextImageFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const imageValue = "data:image/png;base64,aW1hZ2U="
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "agnes-image-2.1-flash"))
	require.NoError(t, writer.WriteField("prompt", "use this reference"))
	require.NoError(t, writer.WriteField("image", imageValue))
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	converted, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
	}, dto.ImageRequest{
		Model:  "agnes-image-2.1-flash",
		Prompt: "use this reference",
	})

	require.NoError(t, err)
	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	extraBody, ok := payload["extra_body"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []string{imageValue}, extraBody["image"])
}

func TestGetRequestURLMapsImageEditsToGenerations(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeImagesEdits,
		RequestURLPath: "/v1/images/edits",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://apihub.agnes-ai.com/v1",
		},
	}

	requestURL, err := adaptor.GetRequestURL(info)

	require.NoError(t, err)
	require.Equal(t, "https://apihub.agnes-ai.com/v1/images/generations", requestURL)
}

func TestConvertEmbeddingRequestIsUnsupported(t *testing.T) {
	adaptor := &Adaptor{}

	_, err := adaptor.ConvertEmbeddingRequest(nil, &relaycommon.RelayInfo{}, dto.EmbeddingRequest{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "embeddings")
}

func ptr[T any](value T) *T {
	return &value
}
