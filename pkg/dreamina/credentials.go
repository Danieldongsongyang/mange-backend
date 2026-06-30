package dreamina

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/dreamina/authsign"
)

var (
	ErrMissingCredential = errors.New("missing dreamina credential")
	ErrInvalidCredential = errors.New("invalid dreamina credential")
)

type Credential struct {
	AccessToken string
	DeviceKey   authsign.DeviceKey
}

type CredentialProvider interface {
	GetCredential(ctx context.Context) (Credential, error)
}

type StaticCredentialProvider struct {
	Value Credential
}

type SecretCredentialProvider struct {
	Raw string
}

type CredentialSecret struct {
	AccessToken      string              `json:"access_token"`
	DeviceKey        CredentialDeviceKey `json:"device_key"`
	DeviceAlgorithm  string              `json:"device_algorithm,omitempty"`
	DevicePublicKey  string              `json:"device_public_key,omitempty"`
	DevicePrivateKey string              `json:"device_private_key,omitempty"`
}

type CredentialDeviceKey struct {
	Algorithm        string `json:"algorithm,omitempty"`
	PublicKey        string `json:"public_key,omitempty"`
	PrivateKey       string `json:"private_key,omitempty"`
	AlgorithmCompat  string `json:"Algorithm,omitempty"`
	PublicKeyBase64  string `json:"PublicKeyBase64,omitempty"`
	PrivateKeyBase64 string `json:"PrivateKeyBase64,omitempty"`
}

func (p StaticCredentialProvider) GetCredential(ctx context.Context) (Credential, error) {
	if err := ctx.Err(); err != nil {
		return Credential{}, err
	}
	if err := p.Value.Validate(); err != nil {
		return Credential{}, err
	}
	return p.Value, nil
}

func (p SecretCredentialProvider) GetCredential(ctx context.Context) (Credential, error) {
	if err := ctx.Err(); err != nil {
		return Credential{}, err
	}
	return ParseCredentialSecret(p.Raw)
}

func ParseCredentialSecret(raw string) (Credential, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Credential{}, fmt.Errorf("%w: secret is required", ErrMissingCredential)
	}
	if strings.HasPrefix(raw, "go-keyring-base64:") {
		payload := strings.TrimPrefix(raw, "go-keyring-base64:")
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return Credential{}, fmt.Errorf("%w: keyring payload must be valid base64", ErrInvalidCredential)
		}
		raw = string(decoded)
	}

	var secret CredentialSecret
	if err := common.UnmarshalJsonStr(raw, &secret); err != nil {
		return Credential{}, fmt.Errorf("%w: secret must be a valid JSON object", ErrInvalidCredential)
	}

	deviceKey := secret.DeviceKey.toAuthsignDeviceKey()
	if deviceKey.Algorithm == "" {
		deviceKey.Algorithm = secret.DeviceAlgorithm
	}
	if deviceKey.PublicKey == "" {
		deviceKey.PublicKey = secret.DevicePublicKey
	}
	if deviceKey.PrivateKey == "" {
		deviceKey.PrivateKey = secret.DevicePrivateKey
	}
	if deviceKey.Algorithm == "" && (deviceKey.PublicKey != "" || deviceKey.PrivateKey != "") {
		deviceKey.Algorithm = authsign.Algorithm
	}

	credential := Credential{
		AccessToken: secret.AccessToken,
		DeviceKey:   deviceKey,
	}
	if err := credential.Validate(); err != nil {
		return Credential{}, err
	}
	return credential, nil
}

func (d CredentialDeviceKey) toAuthsignDeviceKey() authsign.DeviceKey {
	deviceKey := authsign.DeviceKey{
		Algorithm:  d.Algorithm,
		PublicKey:  d.PublicKey,
		PrivateKey: d.PrivateKey,
	}
	if deviceKey.Algorithm == "" {
		deviceKey.Algorithm = d.AlgorithmCompat
	}
	if deviceKey.PublicKey == "" {
		deviceKey.PublicKey = d.PublicKeyBase64
	}
	if deviceKey.PrivateKey == "" {
		deviceKey.PrivateKey = d.PrivateKeyBase64
	}
	return deviceKey
}

func (c Credential) Validate() error {
	if c.AccessToken == "" {
		return fmt.Errorf("%w: access token is required", ErrMissingCredential)
	}
	if _, _, err := authsign.ParseDeviceKey(c.DeviceKey); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCredential, err)
	}
	return nil
}
