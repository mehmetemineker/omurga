package project

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"omurga/internal/manifest"
)

func TestGenerateCreatesComposeAndCaddyArtifacts(t *testing.T) {
	loaded, err := manifest.Load(filepath.Join("..", "..", "examples", "basic"), "production")
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}
	artifacts, err := Generate(loaded.Project, DefaultRenderOptions(loaded.Project, loaded.Environment))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	var compose ComposeFile
	if err := yaml.Unmarshal(artifacts.Compose, &compose); err != nil {
		t.Fatalf("generated Compose YAML is invalid: %v", err)
	}
	app := compose.Services["app"]
	if app.PullPolicy != "missing" || app.Restart != "unless-stopped" {
		t.Fatalf("unexpected app policies: %#v", app)
	}
	if len(app.Ports) != 1 || app.Ports[0].HostIP != "127.0.0.1" || app.Ports[0].Target != 3000 {
		t.Fatalf("gateway port is not loopback-only: %#v", app.Ports)
	}
	if _, exists := compose.Services["postgres"]; !exists {
		t.Fatal("PostgreSQL dependency was not generated")
	}
	if _, exists := compose.Services["redis"]; !exists {
		t.Fatal("Redis dependency was not generated")
	}
	secret, exists := compose.Secrets["database-password"]
	if !exists || !strings.HasSuffix(secret.File, "/blog/production/database-password") {
		t.Fatalf("unexpected secret declaration: %#v", secret)
	}
	if !strings.Contains(string(artifacts.Caddy), "blog.example.com") || !strings.Contains(string(artifacts.Caddy), "reverse_proxy 127.0.0.1:") {
		t.Fatalf("unexpected Caddy config:\n%s", artifacts.Caddy)
	}
}

func TestPreviewPortsIsDeterministicAndUnique(t *testing.T) {
	project := manifest.Project{
		Name: "demo",
		Gateway: manifest.Gateway{Routes: []manifest.Route{
			{Service: "app", Port: 3000},
			{Service: "app", Port: 3000},
			{Service: "api", Port: 8080},
		}},
	}
	first := PreviewPorts(project, "production")
	second := PreviewPorts(project, "production")
	if first["app:3000"] != second["app:3000"] || first["api:8080"] != second["api:8080"] {
		t.Fatalf("port preview is not deterministic: %#v %#v", first, second)
	}
	if first["app:3000"] == first["api:8080"] {
		t.Fatalf("different targets received the same preview port: %#v", first)
	}
}

func TestGenerateRejectsSharedDependency(t *testing.T) {
	project := manifest.Project{
		Version: 1,
		Name:    "demo",
		Services: map[string]manifest.Service{
			"app": {Image: "example/app:1"},
		},
		Dependencies: map[string]manifest.Dependency{
			"database": {Type: "postgres", Version: "16", Mode: "shared", Instance: "main"},
		},
	}
	_, err := Generate(project, DefaultRenderOptions(project, "production"))
	if err == nil || !strings.Contains(err.Error(), "shared dependency") {
		t.Fatalf("expected shared dependency error, got: %v", err)
	}
}
