package dreamina

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/dreamina/authsign"
)

func TestParseCredentialSecretSupportsNestedDeviceKey(t *testing.T) {
	deviceKey, err := authsign.GenerateDeviceKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := common.Marshal(CredentialSecret{
		AccessToken: "access-token",
		DeviceKey: CredentialDeviceKey{
			Algorithm:  deviceKey.Algorithm,
			PublicKey:  deviceKey.PublicKey,
			PrivateKey: deviceKey.PrivateKey,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	credential, err := ParseCredentialSecret(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessToken != "access-token" {
		t.Fatalf("access token mismatch")
	}
	if credential.DeviceKey.PublicKey != deviceKey.PublicKey {
		t.Fatalf("device public key mismatch")
	}
}

func TestParseCredentialSecretSupportsGoKeyringAuthRecord(t *testing.T) {
	deviceKey, err := authsign.GenerateDeviceKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := common.Marshal(map[string]any{
		"access_token": "access-token",
		"device_key": map[string]string{
			"Algorithm":        deviceKey.Algorithm,
			"PublicKeyBase64":  deviceKey.PublicKey,
			"PrivateKeyBase64": deviceKey.PrivateKey,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := "go-keyring-base64:" + base64.StdEncoding.EncodeToString(raw)

	credential, err := ParseCredentialSecret(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if credential.DeviceKey.PrivateKey != deviceKey.PrivateKey {
		t.Fatalf("device private key mismatch")
	}
}

func TestParseCredentialSecretSupportsFlatDeviceKeyFields(t *testing.T) {
	deviceKey, err := authsign.GenerateDeviceKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := common.Marshal(CredentialSecret{
		AccessToken:      "access-token",
		DevicePublicKey:  deviceKey.PublicKey,
		DevicePrivateKey: deviceKey.PrivateKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	credential, err := ParseCredentialSecret(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if credential.DeviceKey.Algorithm != authsign.Algorithm {
		t.Fatalf("algorithm mismatch: %s", credential.DeviceKey.Algorithm)
	}
}

func TestSecretCredentialProviderDoesNotEchoInvalidSecret(t *testing.T) {
	const raw = `{"access_token":"secret-access-token","device_private_key":"secret-private-key"}`
	provider := SecretCredentialProvider{Raw: raw}

	_, err := provider.GetCredential(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-access-token") || strings.Contains(err.Error(), "secret-private-key") {
		t.Fatalf("error leaked raw secret: %s", err)
	}
}
