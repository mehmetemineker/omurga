package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxPayloadBytes = 64 * 1024
	maxClockSkew    = 5 * time.Minute
)

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type Payload struct {
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Service     string `json:"service"`
	Image       string `json:"image"`
	Digest      string `json:"digest"`
}

type DeploymentResult struct {
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Service     string `json:"service"`
	Image       string `json:"image"`
	Revision    string `json:"revision"`
}

type DeployFunc func(ctx context.Context, hook Hook, image string) (DeploymentResult, error)

type Handler struct {
	hooks   map[string]RuntimeHook
	deploy  DeployFunc
	replay  *replayGuard
	clock   func() time.Time
	deployM sync.Mutex
}

func NewHandler(hooks []RuntimeHook, replayPath string, deploy DeployFunc) (*Handler, error) {
	if deploy == nil {
		return nil, fmt.Errorf("webhook deploy function is required")
	}
	replay, err := newReplayGuard(replayPath)
	if err != nil {
		return nil, err
	}
	registered := make(map[string]RuntimeHook, len(hooks))
	for _, hook := range hooks {
		if err := validateHook(hook.Hook); err != nil {
			return nil, err
		}
		if len(hook.secret) < 32 {
			return nil, fmt.Errorf("webhook %s secret must contain at least 32 bytes", hook.Name)
		}
		if _, exists := registered[hook.Name]; exists {
			return nil, fmt.Errorf("duplicate webhook name %q", hook.Name)
		}
		registered[hook.Name] = hook
	}
	return &Handler{hooks: registered, deploy: deploy, replay: replay, clock: time.Now}, nil
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	name := strings.TrimPrefix(request.URL.Path, "/webhooks/")
	if name == request.URL.Path || strings.Contains(name, "/") || name == "" {
		writeError(writer, http.StatusNotFound, "webhook not found")
		return
	}
	hook, exists := h.hooks[name]
	if !exists || !hook.Enabled {
		writeError(writer, http.StatusNotFound, "webhook not found")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxPayloadBytes))
	if err != nil {
		writeError(writer, http.StatusRequestEntityTooLarge, "payload is too large")
		return
	}
	timestamp := request.Header.Get("X-Omurga-Timestamp")
	if !validTimestamp(timestamp, h.clock()) {
		writeError(writer, http.StatusUnauthorized, "invalid or expired webhook timestamp")
		return
	}
	delivery := request.Header.Get("X-Omurga-Delivery")
	if !validDeliveryID(delivery) {
		writeError(writer, http.StatusUnauthorized, "missing or invalid delivery id")
		return
	}
	if !validSignature(request.Header.Get("X-Omurga-Signature-256"), hook.secret, timestamp, body) {
		writeError(writer, http.StatusUnauthorized, "invalid webhook signature")
		return
	}
	if event := request.Header.Get("X-Omurga-Event"); event != "image.published" {
		writeError(writer, http.StatusBadRequest, "unsupported webhook event")
		return
	}
	if !h.replay.reserve(delivery, h.clock()) {
		writeError(writer, http.StatusConflict, "webhook delivery has already been processed or is in progress")
		return
	}

	var payload Payload
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		h.replay.release(delivery)
		writeError(writer, http.StatusBadRequest, "invalid webhook payload")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		h.replay.release(delivery)
		writeError(writer, http.StatusBadRequest, "invalid webhook payload")
		return
	}
	if err := validatePayload(hook.Hook, payload); err != nil {
		h.replay.release(delivery)
		writeError(writer, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// Serialize deployments so two valid image notifications cannot race while
	// replacing the same Compose and Caddy artifacts.
	h.deployM.Lock()
	result, err := h.deploy(request.Context(), hook.Hook, payload.Image+"@"+payload.Digest)
	h.deployM.Unlock()
	if err != nil {
		h.replay.release(delivery)
		writeError(writer, http.StatusInternalServerError, "deployment failed")
		return
	}
	if err := h.replay.commit(delivery, h.clock()); err != nil {
		writeError(writer, http.StatusInternalServerError, "deployment completed but delivery state could not be stored")
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func validatePayload(hook Hook, payload Payload) error {
	if payload.Project != hook.Project || payload.Environment != hook.Environment || payload.Service != hook.Service {
		return errors.New("webhook payload target does not match its configured target")
	}
	if strings.TrimSpace(payload.Image) == "" || strings.ContainsAny(payload.Image, "@ \t\r\n") {
		return errors.New("image must be a non-empty reference without a digest")
	}
	if !imageMatchesPrefix(payload.Image, hook.ImagePrefix) {
		return errors.New("image is outside the configured image prefix")
	}
	if !digestPattern.MatchString(payload.Digest) {
		return errors.New("digest must be a lowercase sha256 digest")
	}
	return nil
}

func imageMatchesPrefix(image, prefix string) bool {
	return image == prefix || strings.HasPrefix(image, prefix+":") || strings.HasPrefix(image, prefix+"/")
}

func validTimestamp(value string, now time.Time) bool {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || strings.TrimSpace(value) == "" {
		return false
	}
	when := time.Unix(seconds, 0)
	return when.After(now.Add(-maxClockSkew)) && when.Before(now.Add(maxClockSkew))
}

func validDeliveryID(value string) bool {
	return len(value) > 0 && len(value) <= 128 && !strings.ContainsAny(value, "\r\n")
}

func validSignature(value string, secret []byte, timestamp string, body []byte) bool {
	if !strings.HasPrefix(value, "sha256=") {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(value, "sha256="))
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
