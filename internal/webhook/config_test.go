package webhook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddHookGeneratesSecretAndStoresOnlySecretFileReference(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "webhooks.yaml")
	secretPath := filepath.Join(root, "secrets", "demo.secret")
	hook := Hook{
		Name: "demo-production", Project: "demo", Environment: "production", Service: "app",
		ManifestPath: filepath.Join(root, "omurga.yaml"), ImagePrefix: "ghcr.io/acme/demo", SecretFile: secretPath,
	}

	secret, err := AddHook(configPath, hook)
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}
	if len(secret) < 32 {
		t.Fatalf("generated secret is too short: %d", len(secret))
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(config.Hooks) != 1 || config.Hooks[0].SecretFile != secretPath {
		t.Fatalf("unexpected config: %#v", config)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), secret) {
		t.Fatal("webhook secret was written into the configuration")
	}
	stored, err := os.ReadFile(secretPath)
	if err != nil || strings.TrimSpace(string(stored)) != secret {
		t.Fatalf("unexpected secret file: %q, %v", stored, err)
	}
	hooks, err := LoadRuntimeHooks(configPath)
	if err != nil {
		t.Fatalf("LoadRuntimeHooks() error = %v", err)
	}
	if len(hooks) != 1 || string(hooks[0].secret) != secret {
		t.Fatalf("runtime hook did not load the generated secret")
	}
}
