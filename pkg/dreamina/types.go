package dreamina

import (
	"fmt"
	"net/url"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
)

const (
	DefaultAgentScene           = "workbench"
	DefaultCreationAgentVersion = "3.0.0"
	DefaultTextToImageType      = "text2imageByConfig"
	DefaultRatio                = "1:1"
	DefaultResolutionType       = "2k"
	DefaultTextToImageBabiParam = `{"scene_lv2":"tool_image","tab_name":"cli","edit_type":"cli","enter_from":"cli","tool_id":"tool_image","sub_tool_id":"tool_image","scene_lv1":"cli"}`
)

type BaseResponse struct {
	Ret       string `json:"ret"`
	Msg       string `json:"msg,omitempty"`
	ErrMsg    string `json:"errmsg,omitempty"`
	MsgDetail string `json:"msgDetail,omitempty"`
	SysTime   string `json:"systime,omitempty"`
}

func (r BaseResponse) Check() error {
	if r.Ret == "0" {
		return nil
	}
	return &UpstreamError{
		Ret:     r.Ret,
		Message: r.message(),
	}
}

func (r BaseResponse) message() string {
	if r.ErrMsg != "" {
		return r.ErrMsg
	}
	if r.Msg != "" {
		return r.Msg
	}
	return r.MsgDetail
}

type UserInfoResponse struct {
	BaseResponse
	Data map[string]any `json:"data,omitempty"`
}

type UploadTokenResponse struct {
	BaseResponse
	Data UploadTokenData `json:"data"`
}

type UploadTokenData struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
	Region          string `json:"region"`
	SpaceName       string `json:"space_name"`
	SpaceType       int    `json:"space_type"`
	UploadDomain    string `json:"upload_domain"`
	CurrentTime     string `json:"current_time"`
	ExpiredTime     string `json:"expired_time"`
}

type TextToImageRequest struct {
	Prompt               string
	Ratio                string
	ResolutionType       string
	GenerateNum          int
	GenerateType         string
	AgentScene           string
	CreationAgentVersion string
	SubmitID             string
	GenerateID           string
	BabiParam            string
	WorkspaceID          int64
}

type ImageGenerateResponse struct {
	BaseResponse
	LogID string            `json:"logId,omitempty"`
	Data  ImageGenerateData `json:"data"`
}

type ImageGenerateData struct {
	SubmitID           string             `json:"submit_id"`
	HistoryID          string             `json:"history_id"`
	ModelKey           string             `json:"model_key"`
	Ratio              string             `json:"ratio"`
	ForecastResolution ForecastResolution `json:"forecast_resolution"`
	PreGenItemIDs      []string           `json:"pre_gen_item_ids"`
	CommerceInfo       CommerceInfo       `json:"commerce_info"`
	SubmitInfo         SubmitInfo         `json:"submit_info"`
}

type ForecastResolution struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type CommerceInfo struct {
	CreditCount int               `json:"credit_count"`
	Triplets    []CommerceTriplet `json:"triplets"`
}

type CommerceTriplet struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	BenefitType  string `json:"benefit_type"`
}

type SubmitInfo struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type HistoryResponse struct {
	BaseResponse
	LogID string                 `json:"logid,omitempty"`
	Data  map[string]HistoryItem `json:"data"`
}

type HistoryItem struct {
	SubmitID  string        `json:"submit_id"`
	Status    int           `json:"status"`
	QueueInfo QueueInfo     `json:"queue_info"`
	ItemList  []HistoryList `json:"item_list"`
	Task      HistoryTask   `json:"task"`
	Workspace int64         `json:"workspace_id"`
}

type QueueInfo struct {
	QueueIdx    int `json:"queue_idx"`
	Priority    int `json:"priority"`
	QueueStatus int `json:"queue_status"`
	QueueLength int `json:"queue_length"`
}

type HistoryList struct {
	Image         HistoryImage  `json:"image"`
	Extra         HistoryExtra  `json:"extra"`
	GenResultData GenResultData `json:"gen_result_data"`
}

type HistoryImage struct {
	LargeImages []LargeImage `json:"large_images"`
}

type LargeImage struct {
	ImageURI string `json:"image_uri"`
	ImageURL string `json:"image_url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Format   string `json:"format"`
	Size     int64  `json:"size"`
}

type HistoryExtra struct {
	CreditsConsume int    `json:"credits_consume"`
	TemplateType   string `json:"template_type"`
}

type GenResultData struct {
	ResultCode int    `json:"result_code"`
	ResultMsg  string `json:"result_msg"`
}

type HistoryTask struct {
	HistoryID string `json:"history_id"`
	TaskID    string `json:"task_id"`
	Status    int    `json:"status"`
	SubmitID  string `json:"submit_id"`
}

func (r *HistoryResponse) LargeImages(submitID string) []LargeImage {
	if r == nil {
		return nil
	}
	item, ok := r.Data[submitID]
	if !ok {
		return nil
	}
	var images []LargeImage
	for _, listItem := range item.ItemList {
		images = append(images, listItem.Image.LargeImages...)
	}
	return images
}

type imageGeneratePayload struct {
	AgentScene           string `json:"agent_scene"`
	CreationAgentVersion string `json:"creation_agent_version"`
	GenerateNum          int    `json:"generate_num"`
	GenerateType         string `json:"generate_type"`
	Prompt               string `json:"prompt"`
	Ratio                string `json:"ratio"`
	ResolutionType       string `json:"resolution_type"`
	SubjectID            string `json:"subject_id"`
	SubmitID             string `json:"submit_id"`
	WorkspaceID          int64  `json:"workspace_id"`
}

type historyByIDsRequest struct {
	HistoryIDs []string `json:"history_ids"`
	NeedBatch  bool     `json:"need_batch"`
	SubmitIDs  []string `json:"submit_ids"`
}

type uploadTokenRequest struct {
	Scene int `json:"scene"`
}

func buildImageGeneratePayload(input TextToImageRequest) (imageGeneratePayload, url.Values, error) {
	if input.Prompt == "" {
		return imageGeneratePayload{}, nil, fmt.Errorf("%w: prompt is required", ErrInvalidRequest)
	}
	if input.SubmitID == "" {
		input.SubmitID = uuid.NewString()
	}
	if input.GenerateID == "" {
		input.GenerateID = newGenerateID()
	}
	if input.GenerateNum == 0 {
		input.GenerateNum = 1
	}
	if input.AgentScene == "" {
		input.AgentScene = DefaultAgentScene
	}
	if input.CreationAgentVersion == "" {
		input.CreationAgentVersion = DefaultCreationAgentVersion
	}
	if input.GenerateType == "" {
		input.GenerateType = DefaultTextToImageType
	}
	if input.Ratio == "" {
		input.Ratio = DefaultRatio
	}
	if input.ResolutionType == "" {
		input.ResolutionType = DefaultResolutionType
	}
	if input.BabiParam == "" {
		input.BabiParam = DefaultTextToImageBabiParam
	}

	query := url.Values{}
	query.Set("generate_id", input.GenerateID)
	if input.BabiParam != "" {
		query.Set("babi_param", input.BabiParam)
	}

	return imageGeneratePayload{
		AgentScene:           input.AgentScene,
		CreationAgentVersion: input.CreationAgentVersion,
		GenerateNum:          input.GenerateNum,
		GenerateType:         input.GenerateType,
		Prompt:               input.Prompt,
		Ratio:                input.Ratio,
		ResolutionType:       input.ResolutionType,
		SubjectID:            input.SubmitID,
		SubmitID:             input.SubmitID,
		WorkspaceID:          input.WorkspaceID,
	}, query, nil
}

func newGenerateID() string {
	return common.GetUUID() + common.GetUUID()[:8]
}
