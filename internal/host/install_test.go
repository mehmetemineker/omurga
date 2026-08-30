package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
		available: map[string]bool{"ufw": true},
		outputs: map[string]string{
			commandKey("dpkg", "--print-architecture"):                  "amd64",
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
		Downloader: fakeDownloader{"https://download.docker.com/linux/ubuntu/gpg": []byte("docker-signing-key")},
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
	config, err := os.ReadFile(paths.DockerDaemonConfig)
	if err != nil || !strings.Contains(string(config), `"log-driver": "local"`) || !strings.Contains(string(config), `"max-size": "10m"`) || !strings.Contains(string(config), `"max-file": "3"`) {
		t.Fatalf("unexpected Docker daemon configuration: %q, %v", config, err)
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
	if err := os.MkdirAll(filepath.Dir(paths.DockerDaemonConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.DockerDaemonConfig, []byte(`{
  "log-driver": "local",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  }
}
`), 0o644); err != nil {
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

func TestInstallResticUsesDistributionPackage(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	runner := &fakeRunner{outputs: map[string]string{}, errors: map[string]error{}}
	installer := Installer{Paths: paths, Runner: runner, Downloader: fakeDownloader{}}

	result, err := installer.InstallRestic(context.Background(), supportedRelease(), InstallOptions{})
	if err != nil {
		t.Fatalf("InstallRestic() error = %v", err)
	}
	if result.Component != "restic" || result.AlreadyInstalled {
		t.Fatalf("unexpected install result: %#v", result)
	}
	if !containsCommand(runner.calls, "apt-get install -y restic") || !containsCommand(runner.calls, "restic version") {
		t.Fatalf("unexpected Restic installation commands: %#v", runner.calls)
	}
}

func TestInstallResticIsNoOpWhenHealthy(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	runner := &fakeRunner{
		available: map[string]bool{"restic": true},
		outputs: map[string]string{
			commandKey("dpkg-query", "-W", "-f=${Status}", "restic"): "install ok installed",
			commandKey("restic", "version"):                          "restic 0.17.3 compiled with go1.23",
		},
		errors: map[string]error{},
	}
	installer := Installer{Paths: paths, Runner: runner, Downloader: fakeDownloader{}}

	result, err := installer.InstallRestic(context.Background(), supportedRelease(), InstallOptions{})
	if err != nil {
		t.Fatalf("InstallRestic() error = %v", err)
	}
	if !result.AlreadyInstalled {
		t.Fatalf("expected a no-op installation result: %#v", result)
	}
	if containsCommand(runner.calls, "apt-get ") {
		t.Fatalf("healthy Restic installation executed APT: %#v", runner.calls)
	}
}

func TestInstallFail2banConfiguresSSHJail(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	runner := &fakeRunner{outputs: map[string]string{}, errors: map[string]error{}}
	installer := Installer{Paths: paths, Runner: runner, Downloader: fakeDownloader{}}

	result, err := installer.InstallFail2ban(context.Background(), supportedRelease(), InstallOptions{})
	if err != nil {
		t.Fatalf("InstallFail2ban() error = %v", err)
	}
	if result.Component != "fail2ban" || result.AlreadyInstalled {
		t.Fatalf("unexpected install result: %#v", result)
	}
	content, err := os.ReadFile(paths.Fail2banJail)
	if err != nil || !strings.Contains(string(content), "[sshd]") || !strings.Contains(string(content), "backend = systemd") {
		t.Fatalf("unexpected Fail2ban jail: %q, %v", content, err)
	}
	if !containsCommand(runner.calls, "apt-get install -y fail2ban") || !containsCommand(runner.calls, "systemctl enable --now fail2ban") {
		t.Fatalf("unexpected Fail2ban installation commands: %#v", runner.calls)
	}
}

func TestInstallFail2banIsNoOpWhenHealthy(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(paths.Fail2banJail), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Fail2banJail, []byte(fail2banSSHJail), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		available: map[string]bool{"fail2ban-client": true, "systemctl": true},
		outputs: map[string]string{
			commandKey("dpkg-query", "-W", "-f=${Status}", "fail2ban"):  "install ok installed",
			commandKey("systemctl", "is-active", "--quiet", "fail2ban"): "",
			commandKey("fail2ban-client", "status", "sshd"):             "Status for the jail: sshd",
		},
		errors: map[string]error{},
	}
	installer := Installer{Paths: paths, Runner: runner, Downloader: fakeDownloader{}}

	result, err := installer.InstallFail2ban(context.Background(), supportedRelease(), InstallOptions{})
	if err != nil {
		t.Fatalf("InstallFail2ban() error = %v", err)
	}
	if !result.AlreadyInstalled {
		t.Fatalf("expected a no-op installation result: %#v", result)
	}
	if containsCommand(runner.calls, "apt-get ") {
		t.Fatalf("healthy Fail2ban installation executed APT: %#v", runner.calls)
	}
}

func TestInstallUFWConfiguresDefaultFirewall(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	runner := &fakeRunner{
		available: map[string]bool{"ufw": true},
		outputs: map[string]string{
			commandKey("ufw", "status", "verbose"): "Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n22/tcp ALLOW IN Anywhere\n80/tcp ALLOW IN Anywhere\n443/tcp ALLOW IN Anywhere",
		},
		errors: map[string]error{},
	}
	installer := Installer{Paths: paths, Runner: runner, Downloader: fakeDownloader{}}

	result, err := installer.InstallUFW(context.Background(), supportedRelease(), InstallOptions{})
	if err != nil {
		t.Fatalf("InstallUFW() error = %v", err)
	}
	if result.Component != "ufw" || result.AlreadyInstalled {
		t.Fatalf("unexpected install result: %#v", result)
	}
	for _, command := range []string{
		"apt-get install -y ufw",
		"ufw default deny incoming",
		"ufw default allow outgoing",
		"ufw allow 22/tcp",
		"ufw allow 80/tcp",
		"ufw allow 443/tcp",
		"ufw --force enable",
	} {
		if !containsCommand(runner.calls, command) {
			t.Fatalf("expected command %q, got %#v", command, runner.calls)
		}
	}
}

func TestInstallUFWAllowsCustomSSHPort(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	runner := &fakeRunner{
		available: map[string]bool{"ufw": true},
		outputs: map[string]string{
			commandKey("ufw", "status", "verbose"): "Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n2222/tcp ALLOW IN Anywhere\n80/tcp ALLOW IN Anywhere\n443/tcp ALLOW IN Anywhere",
		},
		errors: map[string]error{},
	}
	installer := Installer{Paths: paths, Runner: runner, Downloader: fakeDownloader{}}

	if _, err := installer.InstallUFW(context.Background(), supportedRelease(), InstallOptions{UFWSSHPort: 2222}); err != nil {
		t.Fatalf("InstallUFW(custom port) error = %v", err)
	}
	if !containsCommand(runner.calls, "ufw allow 2222/tcp") {
		t.Fatalf("custom SSH port was not allowed: %#v", runner.calls)
	}
}

func TestInstallUFWIsNoOpWhenHealthy(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	runner := &fakeRunner{
		available: map[string]bool{"ufw": true},
		outputs: map[string]string{
			commandKey("dpkg-query", "-W", "-f=${Status}", "ufw"): "install ok installed",
			commandKey("ufw", "status", "verbose"):                "Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n22/tcp ALLOW IN Anywhere\n80/tcp ALLOW IN Anywhere\n443/tcp ALLOW IN Anywhere",
		},
		errors: map[string]error{},
	}
	installer := Installer{Paths: paths, Runner: runner, Downloader: fakeDownloader{}}

	result, err := installer.InstallUFW(context.Background(), supportedRelease(), InstallOptions{})
	if err != nil {
		t.Fatalf("InstallUFW() error = %v", err)
	}
	if !result.AlreadyInstalled {
		t.Fatalf("expected a no-op installation result: %#v", result)
	}
	if containsCommand(runner.calls, "apt-get ") {
		t.Fatalf("healthy UFW installation executed APT: %#v", runner.calls)
	}
}

func containsCommand(calls []string, prefix string) bool {
	for _, call := range calls {
		if strings.HasPrefix(call, prefix) {
			return true
		}
	}
	return false
}

func TestInstallDockerConfiguresOfficialDebianRepository(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	runner := &fakeRunner{outputs: map[string]string{commandKey("dpkg", "--print-architecture"): "arm64"}, errors: map[string]error{}}
	installer := Installer{
		Paths: paths, Runner: runner,
		Downloader: fakeDownloader{"https://download.docker.com/linux/debian/gpg": []byte("debian-docker-key")},
	}
	if _, err := installer.InstallDocker(context.Background(), supportedDebianRelease(), InstallOptions{}); err != nil {
		t.Fatalf("InstallDocker(Debian) error = %v", err)
	}
	source, err := os.ReadFile(paths.DockerSource)
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "URIs: https://download.docker.com/linux/debian") || !strings.Contains(text, "Suites: bookworm") || !strings.Contains(text, "Architectures: arm64") {
		t.Fatalf("unexpected Debian Docker source:\n%s", text)
	}
}

func TestRepositoryTargetRejectsPathsOutsideHostRoot(t *testing.T) {
	installer := NewInstaller(DefaultPaths(t.TempDir()))
	for _, target := range []string{"../outside", "/etc/outside", `etc\outside`, "C:/outside"} {
		if _, err := installer.repositoryTarget(target); err == nil {
			t.Fatalf("repositoryTarget(%q) accepted an unsafe path", target)
		}
	}
	resolved, err := installer.repositoryTarget("etc/example/repository.conf")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join(installer.Paths.Root, "etc", "example", "repository.conf") {
		t.Fatalf("unexpected repository target: %s", resolved)
	}
}

func supportedRelease() OSRelease {
	return OSRelease{ID: "ubuntu", VersionID: "24.04", Codename: "noble", PrettyName: "Ubuntu 24.04 LTS"}
}

func supportedDebianRelease() OSRelease {
	return OSRelease{ID: "debian", VersionID: "12", Codename: "bookworm", PrettyName: "Debian GNU/Linux 12 (bookworm)"}
}
