package agnesai

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	openaiAdaptor openai.Adaptor
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.openaiAdaptor.Init(info)
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	openaiRequest, err := service.GeminiToOpenAIRequest(request, info)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, openaiRequest)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	openaiRequest, err := service.ClaudeToOpenAIRequest(*request, info)
	if err != nil {
		return nil, err
	}
	if info.SupportStreamOptions && info.IsStream {
		openaiRequest.StreamOptions = &dto.StreamOptions{
			IncludeUsage: true,
		}
	}
	return a.ConvertOpenAIRequest(c, info, openaiRequest)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil {
		return "", errors.New("relay info is nil")
	}
	if info.RelayMode == relayconstant.RelayModeImagesEdits {
		return joinAgnesURL(info.ChannelBaseUrl, "/v1/images/generations"), nil
	}
	if (info.RelayFormat == types.RelayFormatClaude || info.RelayFormat == types.RelayFormatGemini) &&
		info.RelayMode != relayconstant.RelayModeResponses &&
		info.RelayMode != relayconstant.RelayModeResponsesCompact {
		return joinAgnesURL(info.ChannelBaseUrl, "/v1/chat/completions"), nil
	}
	return joinAgnesURL(info.ChannelBaseUrl, info.RequestURLPath), nil
}

func joinAgnesURL(baseURL string, requestPath string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = appconstant.ChannelBaseURLs[appconstant.ChannelTypeAgnesAI]
	}

	path := strings.TrimSpace(requestPath)
	if path == "" {
		path = "/v1/chat/completions"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	return base + path
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	if info != nil && info.RelayMode == relayconstant.RelayModeImagesEdits {
		req.Set("Content-Type", "application/json")
	}
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// AgnesAI supports stream_options.include_usage; keep StreamOptions intact.
	// Its thinking switch is chat_template_kwargs.enable_thinking, not OpenAI's
	// reasoning_effort, so avoid sending reasoning_effort as a claimed feature.
	request.ReasoningEffort = ""
	request.Modalities = nil
	request.Audio = nil
	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("AgnesAI does not support rerank")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("AgnesAI does not support embeddings")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("AgnesAI does not support audio")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	payload, err := imageRequestToMap(request)
	if err != nil {
		return nil, err
	}
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations:
	case relayconstant.RelayModeImagesEdits:
		if err := convertImageEditPayload(c, payload, request); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("AgnesAI does not support this image relay mode")
	}
	moveImageResponseFormatToExtraBody(payload)
	return payload, nil
}

func convertImageEditPayload(c *gin.Context, payload map[string]any, request dto.ImageRequest) error {
	extraBody := ensureExtraBody(payload)

	if isAgnesJSONRequest(c) || c == nil || c.Request == nil || !strings.Contains(c.ContentType(), "multipart/form-data") {
		imageValues, err := imageValuesFromJSONRequest(request)
		if err != nil {
			return err
		}
		extraBody["image"] = imageValues
		if len(request.Mask) > 0 {
			maskValue, err := decodeRawJSONValue(request.Mask)
			if err != nil {
				return fmt.Errorf("unmarshal image mask: %w", err)
			}
			extraBody["mask"] = maskValue
		}
	} else {
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return fmt.Errorf("failed to parse multipart form: %w", err)
		}
		applyImageFormValues(payload, form)

		imageValues, err := imageValuesFromMultipartRequest(form, request)
		if err != nil {
			return err
		}
		extraBody["image"] = imageValues

		maskValues, err := fileValuesFromMultipartForm(form, "mask")
		if err != nil {
			return err
		}
		if len(maskValues) > 0 {
			extraBody["mask"] = maskValues[0]
		}
	}

	delete(payload, "image")
	delete(payload, "images")
	delete(payload, "mask")
	return nil
}

func imageRequestToMap(request dto.ImageRequest) (map[string]any, error) {
	data, err := common.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal image request: %w", err)
	}

	var payload map[string]any
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal image request: %w", err)
	}

	for key, raw := range request.Extra {
		if _, exists := payload[key]; exists {
			continue
		}
		var value any
		if err := common.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("unmarshal image extra field %q: %w", key, err)
		}
		payload[key] = value
	}
	return payload, nil
}

func moveImageResponseFormatToExtraBody(payload map[string]any) {
	responseFormat, ok := payload["response_format"]
	if !ok {
		return
	}
	delete(payload, "response_format")

	extraBody := ensureExtraBody(payload)
	extraBody["response_format"] = responseFormat
	payload["extra_body"] = extraBody
}

func ensureExtraBody(payload map[string]any) map[string]any {
	extraBody, ok := payload["extra_body"].(map[string]any)
	if !ok {
		extraBody = map[string]any{}
		payload["extra_body"] = extraBody
	}
	return extraBody
}

func imageValuesFromJSONRequest(request dto.ImageRequest) ([]string, error) {
	if len(request.Images) > 0 {
		return normalizeImageValues(request.Images)
	}
	if len(request.Image) > 0 {
		return normalizeImageValues(request.Image)
	}
	return nil, errors.New("image is required")
}

func normalizeImageValues(raw []byte) ([]string, error) {
	value, err := decodeRawJSONValue(raw)
	if err != nil {
		return nil, err
	}
	switch typed := value.(type) {
	case string:
		return []string{typed}, nil
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			str, ok := item.(string)
			if !ok {
				return nil, errors.New("image must be a string or array of strings")
			}
			values = append(values, str)
		}
		if len(values) == 0 {
			return nil, errors.New("image is required")
		}
		return values, nil
	default:
		return nil, errors.New("image must be a string or array of strings")
	}
}

func decodeRawJSONValue(raw []byte) (any, error) {
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func imageValuesFromMultipartRequest(form *multipart.Form, request dto.ImageRequest) ([]string, error) {
	imageValues, err := fileValuesFromMultipartForm(form, "image")
	if err != nil {
		return nil, err
	}
	if len(imageValues) > 0 {
		return imageValues, nil
	}

	if len(request.Image) > 0 {
		return normalizeImageValues(request.Image)
	}
	if formValues := form.Value["image"]; len(formValues) > 0 {
		return normalizeStringImageValues(formValues)
	}
	if formValues := form.Value["image[]"]; len(formValues) > 0 {
		return normalizeStringImageValues(formValues)
	}
	return nil, errors.New("image is required")
}

func applyImageFormValues(payload map[string]any, form *multipart.Form) {
	for _, field := range []string{"response_format"} {
		if value := strings.TrimSpace(firstFormValue(form, field)); value != "" {
			payload[field] = value
		}
	}
}

func firstFormValue(form *multipart.Form, field string) string {
	if form == nil || form.Value == nil {
		return ""
	}
	values := form.Value[field]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func fileValuesFromMultipartForm(form *multipart.Form, field string) ([]string, error) {
	var fileHeaders []*multipart.FileHeader
	for name, files := range form.File {
		if isMultipartArrayField(name, field) && len(files) > 0 {
			fileHeaders = append(fileHeaders, files...)
		}
	}

	if len(fileHeaders) == 0 {
		return nil, nil
	}

	values := make([]string, 0, len(fileHeaders))
	for index, fileHeader := range fileHeaders {
		dataURL, err := fileHeaderToDataURL(fileHeader)
		if err != nil {
			return nil, fmt.Errorf("read %s file %d: %w", field, index, err)
		}
		values = append(values, dataURL)
	}
	return values, nil
}

func isMultipartArrayField(name string, field string) bool {
	return name == field || name == field+"[]" || strings.HasPrefix(name, field+"[")
}

func normalizeStringImageValues(values []string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return nil, errors.New("image is required")
	}
	return normalized, nil
}

func fileHeaderToDataURL(fileHeader *multipart.FileHeader) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	mimeType := http.DetectContentType(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)), nil
}

func isAgnesJSONRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return strings.HasPrefix(c.Request.Header.Get("Content-Type"), "application/json")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return a.openaiAdaptor.ConvertOpenAIResponsesRequest(c, info, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if err := validateSupportedRelayMode(info); err != nil {
		return nil, err
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func validateSupportedRelayMode(info *relaycommon.RelayInfo) error {
	if info == nil {
		return errors.New("relay info is nil")
	}
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions,
		relayconstant.RelayModeImagesGenerations,
		relayconstant.RelayModeImagesEdits,
		relayconstant.RelayModeResponses:
		return nil
	default:
		return fmt.Errorf("AgnesAI does not support relay mode %d", info.RelayMode)
	}
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	return a.openaiAdaptor.DoResponse(c, resp, info)
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
