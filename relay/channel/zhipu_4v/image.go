package zhipu_4v

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type zhipuImageRequest struct {
	Model            string `json:"model"`
	Prompt           string `json:"prompt"`
	Quality          string `json:"quality,omitempty"`
	Size             string `json:"size,omitempty"`
	WatermarkEnabled *bool  `json:"watermark_enabled,omitempty"`
	UserID           string `json:"user_id,omitempty"`
}

type zhipuImageResponse struct {
	Created       *int64            `json:"created,omitempty"`
	Data          []zhipuImageData  `json:"data,omitempty"`
	ContentFilter any               `json:"content_filter,omitempty"`
	Usage         *dto.Usage        `json:"usage,omitempty"`
	Error         *zhipuImageError  `json:"error,omitempty"`
	RequestID     string            `json:"request_id,omitempty"`
	ExtendParam   map[string]string `json:"extendParam,omitempty"`
}

type zhipuImageError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type zhipuImageData struct {
	Url      string `json:"url,omitempty"`
	ImageUrl string `json:"image_url,omitempty"`
	B64Json  string `json:"b64_json,omitempty"`
	B64Image string `json:"b64_image,omitempty"`
}

type openAIImagePayload struct {
	Created int64             `json:"created"`
	Data    []openAIImageData `json:"data"`
}

type openAIImageData struct {
	B64Json string `json:"b64_json"`
}

func oaiImage2ZhipuImageRequest(request dto.ImageRequest) zhipuImageRequest {
	imageRequest := zhipuImageRequest{
		Model:   request.Model,
		Prompt:  request.Prompt,
		Quality: normalizeZhipuImageQuality(request.Quality),
		Size:    normalizeZhipuImageSize(request.Size),
	}
	if imageRequest.Model == "" {
		imageRequest.Model = "glm-image"
	}
	if request.WatermarkEnabled != nil {
		var enabled bool
		if err := json.Unmarshal(request.WatermarkEnabled, &enabled); err == nil {
			imageRequest.WatermarkEnabled = &enabled
		}
	}
	if request.Watermark != nil {
		imageRequest.WatermarkEnabled = request.Watermark
	}
	if request.UserId != nil {
		var userID string
		if err := json.Unmarshal(request.UserId, &userID); err == nil {
			imageRequest.UserID = userID
		}
	}
	if request.User != nil {
		var userID string
		if err := json.Unmarshal(request.User, &userID); err == nil {
			imageRequest.UserID = userID
		}
	}
	return imageRequest
}

func normalizeZhipuImageQuality(quality string) string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "", "auto", "hd", "high", "4k":
		return "hd"
	default:
		return "hd"
	}
}

func normalizeZhipuImageSize(size string) string {
	switch strings.TrimSpace(size) {
	case "", "auto":
		return "1280x1280"
	case "1024x1024":
		return "1280x1280"
	case "1824x1024", "1792x1024", "1728x972":
		return "1728x960"
	case "1024x1824", "1024x1792", "972x1728":
		return "960x1728"
	case "1360x1024", "1152x864":
		return "1472x1088"
	case "1024x1360", "864x1152":
		return "1088x1472"
	case "1440x1024", "1536x1024":
		return "1568x1056"
	case "1024x1440", "1024x1536":
		return "1056x1568"
	default:
		return strings.TrimSpace(size)
	}
}

func zhipu4vImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	var zhipuResp zhipuImageResponse
	if err := common.Unmarshal(responseBody, &zhipuResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if zhipuResp.Error != nil && zhipuResp.Error.Message != "" {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: zhipuResp.Error.Message,
			Type:    "zhipu_image_error",
			Code:    zhipuResp.Error.Code,
		}, resp.StatusCode)
	}

	payload := openAIImagePayload{}
	if zhipuResp.Created != nil && *zhipuResp.Created != 0 {
		payload.Created = *zhipuResp.Created
	} else {
		payload.Created = info.StartTime.Unix()
	}
	for _, data := range zhipuResp.Data {
		url := data.Url
		if url == "" {
			url = data.ImageUrl
		}
		if url == "" {
			logger.LogWarn(c, "zhipu_image_missing_url")
			continue
		}

		var b64 string
		switch {
		case data.B64Json != "":
			b64 = data.B64Json
		case data.B64Image != "":
			b64 = data.B64Image
		default:
			_, downloaded, err := service.GetImageFromUrl(url)
			if err != nil {
				logger.LogError(c, "zhipu_image_get_b64_failed: "+err.Error())
				continue
			}
			b64 = downloaded
		}

		if b64 == "" {
			logger.LogWarn(c, "zhipu_image_empty_b64")
			continue
		}

		imageData := openAIImageData{
			B64Json: b64,
		}
		payload.Data = append(payload.Data, imageData)
	}

	jsonResp, err := common.Marshal(payload)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	service.IOCopyBytesGracefully(c, resp, jsonResp)

	return &dto.Usage{}, nil
}
