package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMergesEnvironment(t *testing.T) {
	dir := t.TempDir()
	environments := filepath.Join(dir, "environments")
	if err := os.MkdirAll(environments, 0o755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(dir, "omurga.yaml"), `
version: 1
name: blog
services:
  app:
    image: example/blog:1
    expose: [3000]
    environment:
      APP_ENV: development
      LOG_LEVEL: debug
gateway:
  routes:
    - domain: local.example.test
      service: app
      port: 3000
`)
	writeTestFile(t, filepath.Join(environments, "production.yaml"), `
host: production
services:
  app:
    image: example/blog:2
    environment:
      APP_ENV: production
gateway:
  routes:
    - domain: blog.example.com
      service: app
      port: 3000
`)

	loaded, err := Load(dir, "production")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Project.Services["app"].Image != "example/blog:2" {
		t.Fatalf("image override was not applied: %q", loaded.Project.Services["app"].Image)
	}
	if loaded.Project.Services["app"].Environment["LOG_LEVEL"] != "debug" {
		t.Fatal("recursive map merge did not preserve the base value")
	}
	if loaded.Project.Services["app"].Environment["APP_ENV"] != "production" {
		t.Fatal("environment override was not applied")
	}
	if got := loaded.Project.Gateway.Routes[0].Domain; got != "blog.example.com" {
		t.Fatalf("list was not replaced: %q", got)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "omurga.yaml"), `
version: 1
name: blog
unknown: true
services:
  app:
    image: example/blog:1
`)

	_, err := Load(dir, "")
	if err == nil || !strings.Contains(err.Error(), "field unknown") {
		t.Fatalf("expected an unknown field error, got: %v", err)
	}
}

func TestLoadRejectsUnsafeEnvironmentName(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "omurga.yaml"), `
version: 1
name: blog
services:
  app:
    image: example/blog:1
`)

	_, err := Load(dir, "../production")
	if err == nil || !strings.Contains(err.Error(), "invalid environment name") {
		t.Fatalf("unsafe environment name should be rejected, got: %v", err)
	}
}

func TestValidateRejectsRouteToUnknownService(t *testing.T) {
	err := Validate(Project{
		Version: 1,
		Name:    "blog",
		Services: map[string]Service{
			"app": {Image: "example/blog:1", Expose: []int{3000}},
		},
		Gateway: Gateway{Routes: []Route{{Domain: "blog.example.com", Service: "api", Port: 3000}}},
	})
	if err == nil || !strings.Contains(err.Error(), "defined service") {
		t.Fatalf("expected an unknown service error, got: %v", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
