package authsign

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	Algorithm = "ecdsa-p256-sha256"

	HeaderPubKey  = "X-Pub-Key"
	HeaderReqSign = "X-Req-Sign"
	HeaderReqTs   = "X-Req-Ts"
	HeaderNonce   = "X-Req-Nonce"
)

var (
	ErrInvalidDeviceKey = errors.New("invalid dreamina device key")
	ErrInvalidRequest   = errors.New("invalid dreamina sign request")
)

type DeviceKey struct {
	Algorithm  string
	PublicKey  string
	PrivateKey string
}

type Headers struct {
	PublicKey string
	Signature string
	Timestamp string
	Nonce     string
}

type SignOptions struct {
	Now   time.Time
	Nonce string
	Rand  io.Reader
}

func GenerateDeviceKey(random io.Reader) (DeviceKey, error) {
	if random == nil {
		random = rand.Reader
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), random)
	if err != nil {
		return DeviceKey{}, fmt.Errorf("%w: generate ecdsa key: %v", ErrInvalidDeviceKey, err)
	}

	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return DeviceKey{}, fmt.Errorf("%w: marshal public key: %v", ErrInvalidDeviceKey, err)
	}

	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return DeviceKey{}, fmt.Errorf("%w: marshal private key: %v", ErrInvalidDeviceKey, err)
	}

	return DeviceKey{
		Algorithm:  Algorithm,
		PublicKey:  base64.StdEncoding.EncodeToString(publicDER),
		PrivateKey: base64.StdEncoding.EncodeToString(privateDER),
	}, nil
}

func Sign(method, path, accessToken string, deviceKey DeviceKey, opts SignOptions) (Headers, error) {
	if method == "" {
		return Headers{}, fmt.Errorf("%w: method is required", ErrInvalidRequest)
	}
	if accessToken == "" {
		return Headers{}, fmt.Errorf("%w: access token is required", ErrInvalidRequest)
	}
	path = normalizePath(path)
	if !strings.HasPrefix(path, "/") {
		return Headers{}, fmt.Errorf("%w: path must start with /", ErrInvalidRequest)
	}

	privateKey, publicKey, err := ParseDeviceKey(deviceKey)
	if err != nil {
		return Headers{}, err
	}

	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.Rand == nil {
		opts.Rand = rand.Reader
	}

	nonce := opts.Nonce
	if nonce == "" {
		nonce, err = randomNonce(opts.Rand)
		if err != nil {
			return Headers{}, err
		}
	}

	timestamp := strconv.FormatInt(opts.Now.Unix(), 10)
	payload := CanonicalPayload(method, path, accessToken, timestamp, nonce)
	digest := sha256.Sum256([]byte(payload))

	signature, err := ecdsa.SignASN1(opts.Rand, privateKey, digest[:])
	if err != nil {
		return Headers{}, fmt.Errorf("%w: %v", ErrInvalidDeviceKey, err)
	}
	if !ecdsa.VerifyASN1(publicKey, digest[:], signature) {
		return Headers{}, fmt.Errorf("%w: generated signature does not match public key", ErrInvalidDeviceKey)
	}

	return Headers{
		PublicKey: deviceKey.PublicKey,
		Signature: base64.StdEncoding.EncodeToString(signature),
		Timestamp: timestamp,
		Nonce:     nonce,
	}, nil
}

func CanonicalPayload(method, path, accessToken, timestamp, nonce string) string {
	return strings.Join([]string{
		method,
		normalizePath(path),
		AccessTokenDigest(accessToken),
		timestamp,
		nonce,
	}, "\n")
}

func AccessTokenDigest(accessToken string) string {
	digest := sha256.Sum256([]byte(accessToken))
	return base64.StdEncoding.EncodeToString(digest[:])
}

func (h Headers) ApplyTo(header http.Header) {
	header.Set(HeaderPubKey, h.PublicKey)
	header.Set(HeaderReqSign, h.Signature)
	header.Set(HeaderReqTs, h.Timestamp)
	header.Set(HeaderNonce, h.Nonce)
}

func ParseDeviceKey(deviceKey DeviceKey) (*ecdsa.PrivateKey, *ecdsa.PublicKey, error) {
	if deviceKey.Algorithm != Algorithm {
		return nil, nil, fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidDeviceKey, deviceKey.Algorithm)
	}
	if deviceKey.PublicKey == "" || deviceKey.PrivateKey == "" {
		return nil, nil, fmt.Errorf("%w: missing key material", ErrInvalidDeviceKey)
	}

	privateDER, err := base64.StdEncoding.DecodeString(deviceKey.PrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: decode private key: %v", ErrInvalidDeviceKey, err)
	}
	parsedPrivate, err := x509.ParsePKCS8PrivateKey(privateDER)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: parse private key: %v", ErrInvalidDeviceKey, err)
	}
	privateKey, ok := parsedPrivate.(*ecdsa.PrivateKey)
	if !ok || privateKey.Curve != elliptic.P256() {
		return nil, nil, fmt.Errorf("%w: private key is not ecdsa p-256", ErrInvalidDeviceKey)
	}

	publicDER, err := base64.StdEncoding.DecodeString(deviceKey.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: decode public key: %v", ErrInvalidDeviceKey, err)
	}
	parsedPublic, err := x509.ParsePKIXPublicKey(publicDER)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: parse public key: %v", ErrInvalidDeviceKey, err)
	}
	publicKey, ok := parsedPublic.(*ecdsa.PublicKey)
	if !ok || publicKey.Curve != elliptic.P256() {
		return nil, nil, fmt.Errorf("%w: public key is not ecdsa p-256", ErrInvalidDeviceKey)
	}
	if !publicKeysMatch(&privateKey.PublicKey, publicKey) {
		return nil, nil, fmt.Errorf("%w: public key does not match private key", ErrInvalidDeviceKey)
	}

	return privateKey, publicKey, nil
}

func normalizePath(path string) string {
	if path == "" {
		return "/"
	}
	return path
}

func randomNonce(random io.Reader) (string, error) {
	buf := make([]byte, 16)
	if _, err := io.ReadAtLeast(random, buf, len(buf)); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidDeviceKey, err)
	}

	const hex = "0123456789abcdef"
	out := make([]byte, 32)
	for i, b := range buf {
		out[i*2] = hex[b>>4]
		out[i*2+1] = hex[b&0x0f]
	}
	return string(out), nil
}

func publicKeysMatch(left, right *ecdsa.PublicKey) bool {
	if left == nil || right == nil {
		return false
	}
	return left.X.Cmp(right.X) == 0 &&
		left.Y.Cmp(right.Y) == 0 &&
		left.Curve.Params().Name == right.Curve.Params().Name
}
