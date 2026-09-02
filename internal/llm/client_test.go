package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"version\":1,\"name\":\"demo\",\"services\":{\"app\":{\"image\":\"nginx:alpine\"}}}"}}]}`))
	}))
	defer server.Close()

	client := &Client{Config: Config{Endpoint: server.URL, Model: "test-model", APIKey: "test-key"}, HTTPClient: server.Client()}
	content, err := client.Generate(context.Background(), "create a demo")
	if err != nil {
		t.Fatal(err)
	}
	if content == "" {
		t.Fatal("expected completion content")
	}
}

func TestClientReportsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()
	client := &Client{Config: Config{Endpoint: server.URL, Model: "test-model", APIKey: "test-key"}, HTTPClient: server.Client()}
	if _, err := client.Generate(context.Background(), "create a demo"); err == nil {
		t.Fatal("expected provider error")
	}
}

func TestNewClientRejectsInsecureEndpoint(t *testing.T) {
	if _, err := NewClient(Config{Endpoint: "http://provider.example/v1/chat/completions", Model: "model", APIKey: "key"}); err == nil {
		t.Fatal("expected insecure endpoint to be rejected")
	}
}
