package agnesai

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestUsesJSONForSinglePublicImageURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body, err := common.Marshal(map[string]any{
		"model":  "agnes-video-v2.0",
		"prompt": "animate the still image",
		"image":  "https://cdn.example.com/reference.png",
	})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "agnes-video-v2.0",
			ApiKey:            "test-key",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	requestBody, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	bodyBytes, err := io.ReadAll(requestBody)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(bodyBytes, &payload))
	require.Equal(t, "agnes-video-v2.0", payload["model"])
	require.Equal(t, "animate the still image", payload["prompt"])
	require.Equal(t, "https://cdn.example.com/reference.png", payload["image"])
	require.NotContains(t, string(bodyBytes), "multipart/form-data")

	outboundReq := httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	require.NoError(t, adaptor.BuildRequestHeader(c, outboundReq, info))
	require.Equal(t, "application/json", outboundReq.Header.Get("Content-Type"))
	require.Equal(t, "Bearer test-key", outboundReq.Header.Get("Authorization"))
}

func TestBuildRequestMapsInputReferenceURLToImage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body, err := common.Marshal(map[string]any{
		"model":           "agnes-video-v2.0",
		"prompt":          "animate the reference",
		"input_reference": "https://cdn.example.com/reference.png",
	})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "agnes-video-v2.0",
			ApiKey:            "test-key",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	requestBody, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	bodyBytes, err := io.ReadAll(requestBody)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(bodyBytes, &payload))
	require.Equal(t, "https://cdn.example.com/reference.png", payload["image"])
	require.NotContains(t, payload, "input_reference")
}

func TestBuildRequestUsesExtraBodyImageForMultiplePublicImageURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body, err := common.Marshal(map[string]any{
		"model":  "agnes-video-v2.0",
		"prompt": "animate the sequence",
		"images": []string{
			"https://cdn.example.com/first.png",
			"https://cdn.example.com/last.png",
		},
	})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "agnes-video-v2.0",
			ApiKey:            "test-key",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	requestBody, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	bodyBytes, err := io.ReadAll(requestBody)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(bodyBytes, &payload))
	require.NotContains(t, payload, "images")

	extraBody, ok := payload["extra_body"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{
		"https://cdn.example.com/first.png",
		"https://cdn.example.com/last.png",
	}, extraBody["image"])
}

func TestBuildRequestRejectsMultipartReferenceFilesWithoutPublicURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "agnes-video-v2.0"))
	require.NoError(t, writer.WriteField("prompt", "animate upload"))
	part, err := writer.CreateFormFile("input_reference", "reference.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake image bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "agnes-video-v2.0",
			ApiKey:            "test-key",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	requestBody, err := adaptor.BuildRequestBody(c, info)
	require.Error(t, err)
	require.Nil(t, requestBody)
	require.Contains(t, strings.ToLower(err.Error()), "public")
	require.Contains(t, strings.ToLower(err.Error()), "url")
}

func TestBuildRequestConvertsMultipartTextToVideoFormToJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "agnes-video-v2.0"))
	require.NoError(t, writer.WriteField("prompt", "a calm cinematic ocean at sunrise"))
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "agnes-video-v2.0",
			ApiKey:            "test-key",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	taskErr := adaptor.ValidateRequestAndSetAction(c, info)
	require.Nil(t, taskErr)

	requestBody, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	bodyBytes, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	require.NotContains(t, string(bodyBytes), "multipart/form-data")

	var payload map[string]any
	require.NoError(t, common.Unmarshal(bodyBytes, &payload))
	require.Equal(t, "agnes-video-v2.0", payload["model"])
	require.Equal(t, "a calm cinematic ocean at sunrise", payload["prompt"])
	require.NotContains(t, payload, "image")
	require.NotContains(t, payload, "extra_body")
}

func TestValidateRequestRejectsMultipartReferenceFilesWithBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "agnes-video-v2.0"))
	require.NoError(t, writer.WriteField("prompt", "animate upload"))
	part, err := writer.CreateFormFile("input_reference", "reference.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake image bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{})
	require.NotNil(t, taskErr)
	require.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	require.Contains(t, strings.ToLower(taskErr.Message), "public")
	require.Contains(t, strings.ToLower(taskErr.Message), "url")
}

func TestFetchTaskUsesVideoIDWhenAvailable(t *testing.T) {
	var requestedPath string
	var requestedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"completed"}`))
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	resp, err := adaptor.FetchTask(server.URL, "test-key", map[string]any{
		"task_id":  "upstream-task-id",
		"video_id": "agnes-video-id",
	}, "")
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, "/agnesapi", requestedPath)
	require.Equal(t, "video_id=agnes-video-id", requestedQuery)
}

func TestFetchTaskUsesEncodedVideoIDFromStoredUpstreamTaskID(t *testing.T) {
	var requestedPath string
	var requestedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"completed"}`))
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	resp, err := adaptor.FetchTask(server.URL, "test-key", map[string]any{
		"task_id": "video_id:agnes-video-id",
	}, "")
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, "/agnesapi", requestedPath)
	require.Equal(t, "video_id=agnes-video-id", requestedQuery)
}

func TestDoResponseStoresVideoIDForPollingAndReturnsPublicTaskID(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_public",
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(`{
			"id": "upstream-response-id",
			"task_id": "upstream-task-id",
			"video_id": "agnes-video-id",
			"status": "queued"
		}`)),
	}

	upstreamTaskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, info)
	require.Nil(t, taskErr)
	require.Equal(t, "video_id:agnes-video-id", upstreamTaskID)
	var storedPayload map[string]any
	require.NoError(t, common.Unmarshal(taskData, &storedPayload))
	require.Equal(t, "agnes-video-id", storedPayload["video_id"])
	require.Contains(t, recorder.Body.String(), `"id":"task_public"`)
	require.Contains(t, recorder.Body.String(), `"task_id":"task_public"`)
}

func TestParseTaskResultExtractsCompletedVideoURL(t *testing.T) {
	body := []byte(`{
		"status": "completed",
		"progress": 100,
		"remixed_from_video_id": "https://cdn.agnes-ai.com/result.mp4"
	}`)

	taskInfo, err := (&TaskAdaptor{}).ParseTaskResult(body)
	require.NoError(t, err)
	require.Equal(t, "SUCCESS", taskInfo.Status)
	require.Equal(t, "https://cdn.agnes-ai.com/result.mp4", taskInfo.Url)
}

func TestConvertToOpenAIVideoReturnsStoredTaskResult(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_public",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://cdn.agnes-ai.com/result.mp4",
		},
	}
	task.Properties.OriginModelName = "agnes-video-v2.0"

	body, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	require.Equal(t, "task_public", payload["id"])
	require.Equal(t, "completed", payload["status"])
	require.Equal(t, "agnes-video-v2.0", payload["model"])

	metadata, ok := payload["metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "https://cdn.agnes-ai.com/result.mp4", metadata["url"])
}
