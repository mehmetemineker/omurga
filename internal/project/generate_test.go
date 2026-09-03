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

func TestGenerateAddsACMEEmailForHTTPSRoute(t *testing.T) {
	project := manifest.Project{
		Version: 1,
		Name:    "demo",
		Gateway: manifest.Gateway{
			Email:  "ops@example.com",
			Routes: []manifest.Route{{Domain: "demo.example.com", Service: "app", Port: 80, HTTPS: boolPointer(true)}},
		},
		Services: map[string]manifest.Service{
			"app": {Image: "nginx:alpine", Expose: []int{80}},
		},
	}
	artifacts, err := Generate(project, DefaultRenderOptions(project, "production"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(artifacts.Caddy), "tls ops@example.com") {
		t.Fatalf("expected ACME email in generated Caddy config:\n%s", artifacts.Caddy)
	}
}

func TestGenerateAddsRouteResponseHeaders(t *testing.T) {
	project := manifest.Project{
		Version: 1,
		Name:    "demo",
		Services: map[string]manifest.Service{
			"app": {Image: "nginx:alpine", Expose: []int{8080}},
		},
		Gateway: manifest.Gateway{Routes: []manifest.Route{{
			Domain:  "preview.example.com",
			Service: "app",
			Port:    8080,
			HTTPS:   boolPointer(false),
			ResponseHeaders: manifest.ResponseHeaders{
				Remove: []string{"Server", "Via"},
				Set: map[string]string{
					"X-Robots-Tag": "noindex, nofollow, noarchive, nosnippet",
				},
			},
		}}},
	}

	artifacts, err := Generate(project, DefaultRenderOptions(project, "preview"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(artifacts.Caddy)
	for _, expected := range []string{
		"header {",
		"-Server",
		"-Via",
		`X-Robots-Tag "noindex, nofollow, noarchive, nosnippet"`,
		"header_down -Server",
		"header_down -Via",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("generated Caddy config does not contain %q:\n%s", expected, content)
		}
	}
}

func boolPointer(value bool) *bool { return &value }
