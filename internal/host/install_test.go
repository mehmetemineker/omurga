package host

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

type fakeDownloader map[string][]byte

func (d fakeDownloader) Download(_ context.Context, url string) ([]byte, error) {
	data, exists := d[url]
	if !exists {
		return nil, fmt.Errorf("unexpected download: %s", url)
	}
	return data, nil
}

func TestInstallDockerDryRunDoesNotMutateHost(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	runner := &fakeRunner{
		outputs: map[string]string{commandKey("dpkg", "--print-architecture"): "amd64"},
		errors:  map[string]error{},
	}
	installer := Installer{Paths: paths, Runner: runner, Downloader: fakeDownloader{}}

	result, err := installer.InstallDocker(context.Background(), supportedRelease(), InstallOptions{DryRun: true})
	if err != nil {
		t.Fatalf("InstallDocker(dryRun) error = %v", err)
	}
	if !result.DryRun || len(result.Steps) == 0 {
		t.Fatalf("unexpected dry-run result: %#v", result)
	}
	if _, err := os.Stat(paths.DockerSource); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote Docker source: %v", err)
	}
	for _, call := range runner.calls {
		if strings.HasPrefix(call, "apt-get ") || strings.HasPrefix(call, "systemctl ") {
			t.Fatalf("dry-run executed a mutating command: %s", call)
		}
	}
}

func TestInstallDockerRequiresExplicitConflictReplacement(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	runner := &fakeRunner{
		outputs: map[string]string{
			commandKey("dpkg-query", "-W", "-f=${Status}", "docker.io"): "install ok installed",
		},
		errors: map[string]error{},
	}
	installer := Installer{Paths: paths, Runner: runner, Downloader: fakeDownloader{}}

	_, err := installer.InstallDocker(context.Background(), supportedRelease(), InstallOptions{DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "--replace-conflicting-docker") {
		t.Fatalf("expected explicit conflict replacement error, got: %v", err)
	}
}

func TestInstallDockerConfiguresOfficialRepository(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	runner := &fakeRunner{
		outputs: map[string]string{commandKey("dpkg", "--print-architecture"): "amd64"},
		errors:  map[string]error{},
	}
	installer := Installer{
		Paths:      paths,
		Runner:     runner,
		Downloader: fakeDownloader{dockerKeyURL: []byte("docker-signing-key")},
	}

	result, err := installer.InstallDocker(context.Background(), supportedRelease(), InstallOptions{})
	if err != nil {
		t.Fatalf("InstallDocker() error = %v", err)
	}
	if result.Component != "docker" || result.AlreadyInstalled {
		t.Fatalf("unexpected install result: %#v", result)
	}
	source, err := os.ReadFile(paths.DockerSource)
	if err != nil {
		t.Fatalf("Docker source was not created: %v", err)
	}
	text := string(source)
	if !strings.Contains(text, "Suites: noble") || !strings.Contains(text, "Architectures: amd64") {
		t.Fatalf("unexpected Docker source:\n%s", text)
	}
	if key, err := os.ReadFile(paths.DockerKey); err != nil || string(key) != "docker-signing-key" {
		t.Fatalf("unexpected Docker key: %q, %v", key, err)
	}
}

func TestInstallDockerIsNoOpWhenManagedInstallationIsHealthy(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	if err := os.MkdirAll(paths.APTKeyrings, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.APTSources, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.DockerKey, []byte("key"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.DockerSource, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		available: map[string]bool{"docker": true},
		outputs: map[string]string{
			commandKey("dpkg-query", "-W", "-f=${Status}", "docker-ce"):    "install ok installed",
			commandKey("docker", "info", "--format", "{{.ServerVersion}}"): "29.0.0",
			commandKey("docker", "compose", "version", "--short"):          "2.40.0",
		},
		errors: map[string]error{},
	}
	installer := Installer{Paths: paths, Runner: runner, Downloader: fakeDownloader{}}

	result, err := installer.InstallDocker(context.Background(), supportedRelease(), InstallOptions{})
	if err != nil {
		t.Fatalf("InstallDocker() error = %v", err)
	}
	if !result.AlreadyInstalled {
		t.Fatalf("expected a no-op installation result: %#v", result)
	}
	for _, call := range runner.calls {
		if strings.HasPrefix(call, "apt-get ") {
			t.Fatalf("healthy installation executed APT: %s", call)
		}
	}
}

func TestInstallCaddyConfiguresOfficialRepository(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	runner := &fakeRunner{outputs: map[string]string{}, errors: map[string]error{}}
	installer := Installer{
		Paths:  paths,
		Runner: runner,
		Downloader: fakeDownloader{
			caddyKeyURL:    []byte("caddy-signing-key"),
			caddySourceURL: []byte("deb [signed-by=/usr/share/keyrings/caddy-stable-archive-keyring.gpg] https://dl.cloudsmith.io/public/caddy/stable/deb/debian any-version main\n"),
		},
	}

	result, err := installer.InstallCaddy(context.Background(), supportedRelease(), InstallOptions{})
	if err != nil {
		t.Fatalf("InstallCaddy() error = %v", err)
	}
	if result.Component != "caddy" || result.AlreadyInstalled {
		t.Fatalf("unexpected install result: %#v", result)
	}
	if key, err := os.ReadFile(paths.CaddyKey); err != nil || string(key) != "dearmored-key" {
		t.Fatalf("unexpected Caddy key: %q, %v", key, err)
	}
	if source, err := os.ReadFile(paths.CaddySource); err != nil || !strings.Contains(string(source), "caddy/stable") {
		t.Fatalf("unexpected Caddy source: %q, %v", source, err)
	}
}

func supportedRelease() OSRelease {
	return OSRelease{ID: "ubuntu", VersionID: "24.04", Codename: "noble", PrettyName: "Ubuntu 24.04 LTS"}
}
