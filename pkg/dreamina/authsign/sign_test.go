package authsign

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCanonicalPayloadMatchesAuthSDKOrder(t *testing.T) {
	payload := CanonicalPayload("POST", "", "access-token", "1710000000", "001122")
	tokenDigest := sha256.Sum256([]byte("access-token"))
	want := strings.Join([]string{
		"POST",
		"/",
		base64.StdEncoding.EncodeToString(tokenDigest[:]),
		"1710000000",
		"001122",
	}, "\n")

	if payload != want {
		t.Fatalf("payload mismatch\nwant: %q\n got: %q", want, payload)
	}
}

func TestSignProducesVerifiableHeaders(t *testing.T) {
	deviceKey, err := GenerateDeviceKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	headers, err := Sign(
		"POST",
		"/mweb/v1/get_history_by_ids",
		"access-token",
		deviceKey,
		SignOptions{
			Now:   time.Unix(1710000000, 0),
			Nonce: "00112233445566778899aabbccddeeff",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if headers.PublicKey != deviceKey.PublicKey {
		t.Fatalf("public key mismatch")
	}
	if headers.Timestamp != "1710000000" {
		t.Fatalf("timestamp mismatch: %s", headers.Timestamp)
	}
	if headers.Nonce != "00112233445566778899aabbccddeeff" {
		t.Fatalf("nonce mismatch: %s", headers.Nonce)
	}

	_, publicKey, err := ParseDeviceKey(deviceKey)
	if err != nil {
		t.Fatal(err)
	}

	signature, err := base64.StdEncoding.DecodeString(headers.Signature)
	if err != nil {
		t.Fatal(err)
	}
	payload := CanonicalPayload("POST", "/mweb/v1/get_history_by_ids", "access-token", headers.Timestamp, headers.Nonce)
	digest := sha256.Sum256([]byte(payload))
	if !ecdsa.VerifyASN1(publicKey, digest[:], signature) {
		t.Fatalf("signature is not verifiable")
	}
}

func TestHeadersApplyToHTTPHeader(t *testing.T) {
	proof := Headers{
		PublicKey: "pub",
		Signature: "sig",
		Timestamp: "1710000000",
		Nonce:     "nonce",
	}
	header := http.Header{}
	proof.ApplyTo(header)

	assertHeader(t, header, HeaderPubKey, "pub")
	assertHeader(t, header, HeaderReqSign, "sig")
	assertHeader(t, header, HeaderReqTs, "1710000000")
	assertHeader(t, header, HeaderNonce, "nonce")
}

func TestSignRejectsPathWithoutLeadingSlash(t *testing.T) {
	deviceKey, err := GenerateDeviceKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Sign("POST", "mweb/v1/get_history_by_ids", "access-token", deviceKey, SignOptions{}); err == nil {
		t.Fatalf("expected invalid path error")
	}
}

func TestParseDeviceKeyRejectsMismatchedPublicKey(t *testing.T) {
	left, err := GenerateDeviceKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	right, err := GenerateDeviceKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	left.PublicKey = right.PublicKey
	if _, _, err := ParseDeviceKey(left); err == nil {
		t.Fatalf("expected mismatched public key error")
	}
}

func assertHeader(t *testing.T, header http.Header, key, want string) {
	t.Helper()
	if got := header.Get(key); got != want {
		t.Fatalf("%s mismatch: want %q, got %q", key, want, got)
	}
}
