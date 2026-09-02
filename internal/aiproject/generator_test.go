package aiproject

import (
	"strings"
	"testing"
)

func TestParseManifestAcceptsJSONFence(t *testing.T) {
	project, err := ParseManifest("```json\n{\"version\":1,\"name\":\"demo\",\"services\":{\"app\":{\"image\":\"nginx:alpine\"}}}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != "demo" {
		t.Fatalf("unexpected project name: %s", project.Name)
	}
}

func TestParseManifestRejectsSecretValue(t *testing.T) {
	_, err := ParseManifest(`{"version":1,"name":"demo","services":{"app":{"image":"nginx:alpine","environment":{"API_KEY":"secret"}}}}`)
	if err == nil || !strings.Contains(err.Error(), "secret-like value") {
		t.Fatalf("expected secret rejection, got %v", err)
	}
}
