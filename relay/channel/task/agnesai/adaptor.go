package agnesai

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	provideragnesai "github.com/QuantumNous/new-api/relay/channel/agnesai"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	baseURL string
	apiKey  string
}

const videoIDTaskPrefix = "video_id:"

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if strings.Contains(c.GetHeader("Content-Type"), "multipart/form-data") {
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
		defer form.RemoveAll()
		if multipartFormHasFiles(form) {
			return service.TaskErrorWrapperLocal(agnesAIPublicURLRequiredError(), "invalid_request", http.StatusBadRequest)
		}
	}
	return relaycommon.ValidateMultipartDirect(c, info)
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/videos", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	if strings.Contains(c.GetHeader("Content-Type"), "multipart/form-data") {
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return nil, err
		}
		defer form.RemoveAll()
		if multipartFormHasFiles(form) {
			return nil, agnesAIPublicURLRequiredError()
		}
		payload := payloadFromMultipartValues(form.Value)
		return a.buildJSONRequestBody(payload, info)
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}
	cachedBody, err := storage.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "read_body_bytes_failed")
	}

	var payload map[string]any
	if err := common.Unmarshal(cachedBody, &payload); err != nil {
		return nil, errors.Wrap(err, "unmarshal_request_body_failed")
	}
	return a.buildJSONRequestBody(payload, info)
}

func (a *TaskAdaptor) buildJSONRequestBody(payload map[string]any, info *relaycommon.RelayInfo) (io.Reader, error) {
	payload["model"] = info.UpstreamModelName
	if inputReference, ok := payload["input_reference"].(string); ok && strings.TrimSpace(inputReference) != "" {
		payload["image"] = inputReference
		delete(payload, "input_reference")
	}
	if images, ok := payload["images"]; ok {
		extraBody, _ := payload["extra_body"].(map[string]any)
		if extraBody == nil {
			extraBody = map[string]any{}
		}
		extraBody["image"] = images
		payload["extra_body"] = extraBody
		delete(payload, "images")
	}

	requestBody, err := common.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal_request_body_failed")
	}
	return bytes.NewReader(requestBody), nil
}

func multipartFormHasFiles(form *multipart.Form) bool {
	if form == nil {
		return false
	}
	for _, files := range form.File {
		if len(files) > 0 {
			return true
		}
	}
	return false
}

func payloadFromMultipartValues(values map[string][]string) map[string]any {
	payload := map[string]any{}
	for key, vals := range values {
		if len(vals) == 1 {
			payload[key] = vals[0]
		} else if len(vals) > 1 {
			items := make([]string, len(vals))
			copy(items, vals)
			payload[key] = items
		}
	}
	return payload
}

func agnesAIPublicURLRequiredError() error {
	return fmt.Errorf("agnesai video requires publicly accessible image URLs; multipart reference files are not supported")
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var dResp responseTask
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}

	upstreamID := dResp.ID
	if upstreamID == "" {
		upstreamID = dResp.TaskID
	}
	if dResp.VideoID != "" {
		upstreamID = videoIDTaskPrefix + dResp.VideoID
	}
	if upstreamID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	dResp.ID = info.PublicTaskID
	dResp.TaskID = info.PublicTaskID
	c.JSON(http.StatusOK, dResp)
	return upstreamID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	videoID, _ := body["video_id"].(string)
	taskID, _ := body["task_id"].(string)
	if strings.HasPrefix(taskID, videoIDTaskPrefix) {
		videoID = strings.TrimPrefix(taskID, videoIDTaskPrefix)
	}
	if taskID == "" && videoID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}

	fetchURL := fmt.Sprintf("%s/v1/videos/%s", baseUrl, taskID)
	if videoID != "" {
		fetchURL = fmt.Sprintf("%s/agnesapi?video_id=%s", baseUrl, url.QueryEscape(videoID))
	}

	req, err := http.NewRequest(http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var resTask responseTask
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{Code: 0}
	switch resTask.Status {
	case "queued", "pending":
		taskResult.Status = model.TaskStatusQueued
	case "processing", "in_progress":
		taskResult.Status = model.TaskStatusInProgress
	case "completed":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Url = firstNonEmpty(resTask.RemixedFromVideoID, resTask.VideoURL, resTask.URL)
	case "failed", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		if resTask.Error != nil {
			taskResult.Reason = resTask.Error.Message
		} else {
			taskResult.Reason = "task failed"
		}
	}
	if resTask.Progress > 0 && resTask.Progress < 100 {
		taskResult.Progress = fmt.Sprintf("%d%%", resTask.Progress)
	}
	return &taskResult, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (a *TaskAdaptor) GetModelList() []string {
	return provideragnesai.ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return provideragnesai.ChannelName
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	return common.Marshal(task.ToOpenAIVideo())
}

type responseTask struct {
	ID                 string `json:"id"`
	TaskID             string `json:"task_id,omitempty"`
	VideoID            string `json:"video_id,omitempty"`
	Object             string `json:"object,omitempty"`
	Model              string `json:"model,omitempty"`
	Status             string `json:"status,omitempty"`
	Progress           int    `json:"progress,omitempty"`
	RemixedFromVideoID string `json:"remixed_from_video_id,omitempty"`
	VideoURL           string `json:"video_url,omitempty"`
	URL                string `json:"url,omitempty"`
	Error              *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}
