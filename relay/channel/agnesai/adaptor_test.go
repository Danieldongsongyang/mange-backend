package agnesai

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
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

func TestConvertEmbeddingRequestIsUnsupported(t *testing.T) {
	adaptor := &Adaptor{}

	_, err := adaptor.ConvertEmbeddingRequest(nil, &relaycommon.RelayInfo{}, dto.EmbeddingRequest{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "embeddings")
}

func ptr[T any](value T) *T {
	return &value
}
