package agnesai

import (
	"errors"
	"fmt"
	"io"
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
	if info.RelayMode != relayconstant.RelayModeImagesGenerations {
		return nil, errors.New("AgnesAI supports image generation via /v1/images/generations, not OpenAI image edits")
	}
	payload, err := imageRequestToMap(request)
	if err != nil {
		return nil, err
	}
	moveImageResponseFormatToExtraBody(payload)
	return payload, nil
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

	extraBody, ok := payload["extra_body"].(map[string]any)
	if !ok {
		extraBody = map[string]any{}
	}
	extraBody["response_format"] = responseFormat
	payload["extra_body"] = extraBody
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
