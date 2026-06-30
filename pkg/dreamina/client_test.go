package dreamina

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/dreamina/authsign"
)

const testAccessToken = "test-access-token"

func TestClientAddsDreaminaAuthHeadersAndDefaultQuery(t *testing.T) {
	client, closeServer := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method mismatch: %s", r.Method)
		}
		if r.URL.Path != EndpointUserInfo {
			t.Fatalf("path mismatch: %s", r.URL.Path)
		}
		assertQuery(t, r, "agent_detect", DefaultAgentDetect)
		assertQuery(t, r, "aid", DefaultAID)
		assertQuery(t, r, "cli_version", "test-cli")
		assertQuery(t, r, "from", DefaultFrom)
		verifyDreaminaProof(t, r)

		writeJSON(t, w, map[string]any{
			"ret": "0",
			"msg": "success",
			"data": map[string]any{
				"ok": true,
			},
		})
	}))
	defer closeServer()

	resp, err := client.UserInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Data["ok"] != true {
		t.Fatalf("unexpected response data: %#v", resp.Data)
	}
}

func TestSubmitTextToImageSendsCanonicalBodyAndQuery(t *testing.T) {
	client, closeServer := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method mismatch: %s", r.Method)
		}
		if r.URL.Path != EndpointImageGenerate {
			t.Fatalf("path mismatch: %s", r.URL.Path)
		}
		assertQuery(t, r, "generate_id", "generate-1")
		assertQuery(t, r, "babi_param", `{"scene":"test"}`)
		assertBusinessHeaders(t, r)
		verifyDreaminaProof(t, r)

		body := readBody(t, r)
		if !strings.Contains(string(body), `"workspace_id":0`) {
			t.Fatalf("workspace_id=0 must be sent explicitly, body: %s", body)
		}
		if !strings.Contains(string(body), `"generate_num":1`) {
			t.Fatalf("generate_num=1 must be sent explicitly, body: %s", body)
		}

		var payload imageGeneratePayload
		if err := common.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Prompt != "一只玻璃质感的星球" {
			t.Fatalf("prompt mismatch: %s", payload.Prompt)
		}
		if payload.SubmitID != "submit-1" || payload.SubjectID != "submit-1" {
			t.Fatalf("submit id mismatch: %#v", payload)
		}
		if payload.GenerateType != DefaultTextToImageType {
			t.Fatalf("generate type mismatch: %s", payload.GenerateType)
		}

		writeJSON(t, w, map[string]any{
			"ret":       "0",
			"msg":       "success",
			"msgDetail": "success",
			"data": map[string]any{
				"submit_id":  "submit-1",
				"history_id": "history-1",
				"model_key":  "high_aes_general_v50",
				"ratio":      "1:1",
				"forecast_resolution": map[string]any{
					"width":  2048,
					"height": 2048,
				},
				"pre_gen_item_ids": []string{"item-1"},
				"commerce_info": map[string]any{
					"credit_count": 3,
				},
				"submit_info": map[string]any{
					"code": 0,
					"msg":  "",
				},
			},
		})
	}))
	defer closeServer()

	resp, err := client.SubmitTextToImage(context.Background(), TextToImageRequest{
		Prompt:     "一只玻璃质感的星球",
		SubmitID:   "submit-1",
		GenerateID: "generate-1",
		BabiParam:  `{"scene":"test"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Data.SubmitID != "submit-1" || resp.Data.ForecastResolution.Width != 2048 {
		t.Fatalf("unexpected response: %#v", resp.Data)
	}
}

func TestFetchHistoryUsesNullHistoryIDsAndExtractsImages(t *testing.T) {
	client, closeServer := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method mismatch: %s", r.Method)
		}
		if r.URL.Path != EndpointHistoryByIDs {
			t.Fatalf("path mismatch: %s", r.URL.Path)
		}
		assertBusinessHeaders(t, r)
		verifyDreaminaProof(t, r)

		body := readBody(t, r)
		if !strings.Contains(string(body), `"history_ids":null`) {
			t.Fatalf("history_ids must be explicit null, body: %s", body)
		}

		var payload historyByIDsRequest
		if err := common.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if !payload.NeedBatch || len(payload.SubmitIDs) != 1 || payload.SubmitIDs[0] != "submit-1" {
			t.Fatalf("unexpected payload: %#v", payload)
		}

		writeJSON(t, w, map[string]any{
			"ret":    "0",
			"errmsg": "success",
			"data": map[string]any{
				"submit-1": map[string]any{
					"submit_id": "submit-1",
					"status":    50,
					"item_list": []map[string]any{
						{
							"image": map[string]any{
								"large_images": []map[string]any{
									{
										"image_uri": "tos-cn-i-tb4s082cfz/redacted",
										"image_url": "https://example.invalid/redacted",
										"width":     2048,
										"height":    2048,
										"format":    "png",
										"size":      98374,
									},
								},
							},
							"extra": map[string]any{
								"credits_consume": 3,
								"template_type":   "image",
							},
							"gen_result_data": map[string]any{
								"result_code": 0,
								"result_msg":  "Success",
							},
						},
					},
					"task": map[string]any{
						"history_id": "history-1",
						"task_id":    "history-1",
						"status":     50,
						"submit_id":  "submit-1",
					},
					"workspace_id": 0,
				},
			},
		})
	}))
	defer closeServer()

	resp, err := client.FetchHistory(context.Background(), []string{"submit-1"})
	if err != nil {
		t.Fatal(err)
	}
	images := resp.LargeImages("submit-1")
	if len(images) != 1 || images[0].ImageURI != "tos-cn-i-tb4s082cfz/redacted" {
		t.Fatalf("unexpected images: %#v", images)
	}
}

func TestGetUploadTokenUsesDefaultScene(t *testing.T) {
	client, closeServer := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method mismatch: %s", r.Method)
		}
		if r.URL.Path != EndpointUploadToken {
			t.Fatalf("path mismatch: %s", r.URL.Path)
		}
		assertBusinessHeaders(t, r)
		verifyDreaminaProof(t, r)

		body := readBody(t, r)
		var payload uploadTokenRequest
		if err := common.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Scene != 2 {
			t.Fatalf("scene mismatch: %d", payload.Scene)
		}

		writeJSON(t, w, map[string]any{
			"ret":    "0",
			"errmsg": "success",
			"data": map[string]any{
				"access_key_id":     "redacted",
				"secret_access_key": "redacted",
				"session_token":     "redacted",
				"region":            "cn",
				"space_type":        2,
				"upload_domain":     "imagex.bytedanceapi.com",
			},
		})
	}))
	defer closeServer()

	resp, err := client.GetUploadToken(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Data.UploadDomain != "imagex.bytedanceapi.com" {
		t.Fatalf("upload domain mismatch: %s", resp.Data.UploadDomain)
	}
}

func TestHTTPStatusErrorDoesNotLeakResponseBody(t *testing.T) {
	client, closeServer := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifyDreaminaProof(t, r)
		http.Error(w, "token=secret-token sign=secret-sign", http.StatusUnauthorized)
	}))
	defer closeServer()

	_, err := client.UserInfo(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	errText := err.Error()
	if strings.Contains(errText, "secret-token") || strings.Contains(errText, "secret-sign") {
		t.Fatalf("error leaked sensitive response body: %s", errText)
	}
}

func newTestClient(t *testing.T, handler http.Handler) (*Client, func()) {
	t.Helper()

	deviceKey, err := authsign.GenerateDeviceKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(handler)
	client, err := NewClient(
		StaticCredentialProvider{
			Value: Credential{
				AccessToken: testAccessToken,
				DeviceKey:   deviceKey,
			},
		},
		WithBaseURL(server.URL),
		WithCLIVersion("test-cli"),
		WithSignOptions(authsign.SignOptions{
			Now:   time.Unix(1710000000, 0),
			Nonce: "00112233445566778899aabbccddeeff",
		}),
	)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	return client, server.Close
}

func verifyDreaminaProof(t *testing.T, r *http.Request) {
	t.Helper()

	if got := r.Header.Get("Authorization"); got != "Bearer "+testAccessToken {
		t.Fatalf("authorization mismatch: %s", got)
	}
	if r.Header.Get(authsign.HeaderReqTs) != "1710000000" {
		t.Fatalf("timestamp mismatch: %s", r.Header.Get(authsign.HeaderReqTs))
	}
	if r.Header.Get(authsign.HeaderNonce) != "00112233445566778899aabbccddeeff" {
		t.Fatalf("nonce mismatch: %s", r.Header.Get(authsign.HeaderNonce))
	}

	publicDER, err := base64.StdEncoding.DecodeString(r.Header.Get(authsign.HeaderPubKey))
	if err != nil {
		t.Fatal(err)
	}
	parsedPublic, err := x509.ParsePKIXPublicKey(publicDER)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, ok := parsedPublic.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("public key is not ecdsa")
	}

	signature, err := base64.StdEncoding.DecodeString(r.Header.Get(authsign.HeaderReqSign))
	if err != nil {
		t.Fatal(err)
	}
	payload := authsign.CanonicalPayload(
		r.Method,
		r.URL.EscapedPath(),
		testAccessToken,
		r.Header.Get(authsign.HeaderReqTs),
		r.Header.Get(authsign.HeaderNonce),
	)
	digest := sha256.Sum256([]byte(payload))
	if !ecdsa.VerifyASN1(publicKey, digest[:], signature) {
		t.Fatalf("signature is not verifiable")
	}
}

func readBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	data, err := common.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
}

func assertQuery(t *testing.T, r *http.Request, key, want string) {
	t.Helper()
	if got := r.URL.Query().Get(key); got != want {
		t.Fatalf("query %s mismatch: want %q, got %q", key, want, got)
	}
}

func assertBusinessHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get(HeaderAppID); got != DefaultAID {
		t.Fatalf("appid mismatch: want %q, got %q", DefaultAID, got)
	}
	if got := r.Header.Get(HeaderPF); got != DefaultPF {
		t.Fatalf("pf mismatch: want %q, got %q", DefaultPF, got)
	}
	if got := r.Header.Get(HeaderTTLogID); len(got) != 33 {
		t.Fatalf("x-tt-logid length mismatch: %d", len(got))
	}
}
