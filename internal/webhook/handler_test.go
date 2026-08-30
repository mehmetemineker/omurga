package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestHandlerDeploysSignedImageDigest(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	now := time.Unix(1_700_000_000, 0)
	manifestPath := filepath.Join(t.TempDir(), "omurga.yaml")
	secretFile := filepath.Join(t.TempDir(), "secret")
	payload := Payload{Project: "demo", Environment: "production", Service: "app", Image: "ghcr.io/acme/demo:build-42", Digest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	var deployed string
	handler, err := NewHandler([]RuntimeHook{{Hook: Hook{Name: "demo-production", Project: "demo", Environment: "production", Service: "app", ManifestPath: manifestPath, ImagePrefix: "ghcr.io/acme/demo", SecretFile: secretFile, Enabled: true}, secret: secret}}, filepath.Join(t.TempDir(), "replay.json"), func(_ context.Context, _ Hook, image string) (DeploymentResult, error) {
		deployed = image
		return DeploymentResult{Project: "demo", Environment: "production", Service: "app", Image: image, Revision: "revision-1"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	handler.clock = func() time.Time { return now }
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := signedRequest(t, "/webhooks/demo-production", body, secret, now, "delivery-1")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	expected := payload.Image + "@" + payload.Digest
	if deployed != expected {
		t.Fatalf("deployed image = %q, want %q", deployed, expected)
	}

	replay := signedRequest(t, "/webhooks/demo-production", body, secret, now, "delivery-1")
	replayRecorder := httptest.NewRecorder()
	handler.ServeHTTP(replayRecorder, replay)
	if replayRecorder.Code != http.StatusConflict {
		t.Fatalf("replay status = %d, body = %s", replayRecorder.Code, replayRecorder.Body.String())
	}
}

func TestHandlerRejectsInvalidSignatureWithoutDeploying(t *testing.T) {
	called := false
	secret := []byte("01234567890123456789012345678901")
	manifestPath := filepath.Join(t.TempDir(), "omurga.yaml")
	secretFile := filepath.Join(t.TempDir(), "secret")
	handler, err := NewHandler([]RuntimeHook{{Hook: Hook{Name: "demo", Project: "demo", Environment: "production", Service: "app", ManifestPath: manifestPath, ImagePrefix: "ghcr.io/acme/demo", SecretFile: secretFile, Enabled: true}, secret: secret}}, filepath.Join(t.TempDir(), "replay.json"), func(_ context.Context, _ Hook, _ string) (DeploymentResult, error) {
		called = true
		return DeploymentResult{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	handler.clock = func() time.Time { return now }
	body := []byte(`{"project":"demo","environment":"production","service":"app","image":"ghcr.io/acme/demo","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`)
	request := signedRequest(t, "/webhooks/demo", body, []byte("wrong-secret-that-is-long-enough-123"), now, "delivery-2")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized || called {
		t.Fatalf("invalid signature status = %d, called = %v", recorder.Code, called)
	}
}

func TestHandlerRejectsExpiredAndUnsafePayloads(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	manifestPath := filepath.Join(t.TempDir(), "omurga.yaml")
	secretFile := filepath.Join(t.TempDir(), "secret")
	handler, err := NewHandler([]RuntimeHook{{Hook: Hook{Name: "demo", Project: "demo", Environment: "production", Service: "app", ManifestPath: manifestPath, ImagePrefix: "ghcr.io/acme/demo", SecretFile: secretFile, Enabled: true}, secret: secret}}, filepath.Join(t.TempDir(), "replay.json"), func(_ context.Context, _ Hook, _ string) (DeploymentResult, error) {
		return DeploymentResult{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	handler.clock = func() time.Time { return now }
	body := []byte(`{"project":"demo","environment":"production","service":"app","image":"ghcr.io/acme/demo","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`)
	request := signedRequest(t, "/webhooks/demo", body, secret, now.Add(-maxClockSkew-time.Second), "delivery-3")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expired timestamp status = %d", recorder.Code)
	}

	unsafeBody := []byte(`{"project":"demo","environment":"production","service":"app","image":"docker.io/other/demo","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`)
	unsafeRequest := signedRequest(t, "/webhooks/demo", unsafeBody, secret, now, "delivery-4")
	unsafeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unsafeRecorder, unsafeRequest)
	if unsafeRecorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe image status = %d", unsafeRecorder.Code)
	}
}

func signedRequest(t *testing.T, path string, body, secret []byte, timestamp time.Time, delivery string) *http.Request {
	t.Helper()
	seconds := fmt.Sprint(timestamp.Unix())
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(seconds + "."))
	_, _ = mac.Write(body)
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("X-Omurga-Timestamp", seconds)
	request.Header.Set("X-Omurga-Delivery", delivery)
	request.Header.Set("X-Omurga-Event", "image.published")
	request.Header.Set("X-Omurga-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return request
}
