package zhipu_4v

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
)

func TestConvertImageRequestUsesZhipuPayload(t *testing.T) {
	t.Parallel()

	n := uint(3)
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}
	request := dto.ImageRequest{
		Model:          "glm-image",
		Prompt:         "蓝色宝石戒指",
		N:              &n,
		Quality:        "auto",
		Size:           "1824x1024",
		ResponseFormat: "b64_json",
		OutputFormat:   json.RawMessage(`"png"`),
	}

	got, err := adaptor.ConvertImageRequest(gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()), info, request)
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if payload["model"] != "glm-image" {
		t.Fatalf("model = %#v, want glm-image", payload["model"])
	}
	if payload["prompt"] != request.Prompt {
		t.Fatalf("prompt = %#v, want %q", payload["prompt"], request.Prompt)
	}
	if payload["quality"] != "hd" {
		t.Fatalf("quality = %#v, want hd", payload["quality"])
	}
	if payload["size"] != "1728x960" {
		t.Fatalf("size = %#v, want 1728x960", payload["size"])
	}
	if _, ok := payload["n"]; ok {
		t.Fatalf("payload should not include n: %s", string(body))
	}
	if _, ok := payload["response_format"]; ok {
		t.Fatalf("payload should not include response_format: %s", string(body))
	}
	if _, ok := payload["output_format"]; ok {
		t.Fatalf("payload should not include output_format: %s", string(body))
	}
}

func TestConvertImageRequestRejectsEdits(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}

	_, err := adaptor.ConvertImageRequest(gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()), info, dto.ImageRequest{
		Model:  "glm-image",
		Prompt: "参考这张图生成",
	})
	if err == nil {
		t.Fatalf("ConvertImageRequest should reject image edits")
	}
}

func TestGetRequestURLRejectsImageEdits(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	_, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://open.bigmodel.cn",
		},
	})
	if err == nil {
		t.Fatalf("GetRequestURL should reject image edits")
	}
}
