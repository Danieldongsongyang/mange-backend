package dreamina

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestSmokeUserInfoFromSecretEnv(t *testing.T) {
	rawSecret := os.Getenv("DREAMINA_SMOKE_SECRET")
	if rawSecret == "" {
		t.Skip("set DREAMINA_SMOKE_SECRET to run the real Dreamina auth smoke test")
	}

	var opts []ClientOption
	if cliVersion := os.Getenv("DREAMINA_SMOKE_CLI_VERSION"); cliVersion != "" {
		opts = append(opts, WithCLIVersion(cliVersion))
	}
	if baseURL := os.Getenv("DREAMINA_SMOKE_BASE_URL"); baseURL != "" {
		opts = append(opts, WithBaseURL(baseURL))
	}

	client, err := NewClient(SecretCredentialProvider{Raw: rawSecret}, opts...)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := client.UserInfo(ctx); err != nil {
		t.Fatalf("dreamina user_info smoke test failed: %v", err)
	}
}

func TestSmokeTextToImageSubmitFetchFromSecretEnv(t *testing.T) {
	rawSecret := os.Getenv("DREAMINA_SMOKE_SECRET")
	if rawSecret == "" {
		t.Skip("set DREAMINA_SMOKE_SECRET to run the real Dreamina generation smoke test")
	}
	if os.Getenv("DREAMINA_SMOKE_RUN_GENERATION") != "1" {
		t.Skip("set DREAMINA_SMOKE_RUN_GENERATION=1 to spend Dreamina credits")
	}

	var opts []ClientOption
	if cliVersion := os.Getenv("DREAMINA_SMOKE_CLI_VERSION"); cliVersion != "" {
		opts = append(opts, WithCLIVersion(cliVersion))
	}
	if baseURL := os.Getenv("DREAMINA_SMOKE_BASE_URL"); baseURL != "" {
		opts = append(opts, WithBaseURL(baseURL))
	}

	client, err := NewClient(SecretCredentialProvider{Raw: rawSecret}, opts...)
	if err != nil {
		t.Fatal(err)
	}

	prompt := os.Getenv("DREAMINA_SMOKE_PROMPT")
	if prompt == "" {
		prompt = "minimal abstract blue circle on white background, integration test"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	submitResp, err := client.SubmitTextToImage(ctx, TextToImageRequest{
		Prompt:         prompt,
		Ratio:          "1:1",
		ResolutionType: "2k",
		GenerateNum:    1,
		BabiParam:      os.Getenv("DREAMINA_SMOKE_BABI_PARAM"),
	})
	if err != nil {
		t.Fatalf("dreamina text2image submit failed: %v", err)
	}
	if submitResp.Data.SubmitID == "" {
		t.Fatal("dreamina text2image submit returned empty submit id")
	}

	deadline := time.Now().Add(3 * time.Minute)
	for {
		if time.Now().After(deadline) {
			t.Fatal("dreamina text2image fetch timed out")
		}

		historyResp, err := client.FetchHistory(ctx, []string{submitResp.Data.SubmitID})
		if err != nil {
			t.Fatalf("dreamina history fetch failed: %v", err)
		}
		history, ok := historyResp.Data[submitResp.Data.SubmitID]
		if !ok {
			time.Sleep(5 * time.Second)
			continue
		}

		if history.Status == 50 {
			images := historyResp.LargeImages(submitResp.Data.SubmitID)
			if len(images) == 0 {
				t.Fatal("dreamina text2image completed without large images")
			}
			for _, image := range images {
				if image.ImageURI == "" && image.ImageURL == "" {
					t.Fatal("dreamina text2image returned image without uri or url")
				}
			}
			return
		}

		for _, item := range history.ItemList {
			if item.GenResultData.ResultCode != 0 {
				t.Fatalf("dreamina text2image failed: status=%d result_code=%d result_msg=%s", history.Status, item.GenResultData.ResultCode, item.GenResultData.ResultMsg)
			}
		}

		time.Sleep(5 * time.Second)
	}
}
