package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOSReleaseParsesDistributionMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "os-release")
	content := "ID=debian\nID_LIKE=debian\nVERSION_ID=\"12\"\nPRETTY_NAME=\"Debian GNU/Linux 12\"\nVERSION_CODENAME=bookworm\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := LoadOSRelease(path)
	if err != nil {
		t.Fatalf("LoadOSRelease() error = %v", err)
	}
	if release.ID != "debian" || release.VersionID != "12" || release.Codename != "bookworm" || len(release.IDLike) != 1 || release.IDLike[0] != "debian" {
		t.Fatalf("unexpected release: %#v", release)
	}
}

func TestDefaultProviderRegistrySupportsUbuntuAndDebian(t *testing.T) {
	tests := []OSRelease{
		{ID: "ubuntu", VersionID: "22.04", Codename: "jammy"},
		{ID: "ubuntu", VersionID: "24.04", Codename: "noble"},
		{ID: "ubuntu", VersionID: "26.04", Codename: "resolute"},
		{ID: "debian", VersionID: "11", Codename: "bullseye"},
		{ID: "debian", VersionID: "12", Codename: "bookworm"},
		{ID: "debian", VersionID: "13", Codename: "trixie"},
	}
	for _, release := range tests {
		provider, err := DefaultProviderRegistry().Resolve(release)
		if err != nil {
			t.Fatalf("Resolve(%s) error = %v", release.ID, err)
		}
		if provider.Family() != "debian" || provider.PackageManager().Name() != "apt" || provider.ServiceManager().Name() != "systemd" {
			t.Fatalf("unexpected provider for %s", release.ID)
		}
	}
}

func TestProviderRegistryRejectsUnsupportedDistributionAndVersion(t *testing.T) {
	if _, err := DefaultProviderRegistry().Resolve(OSRelease{ID: "fedora", VersionID: "42", Codename: ""}); err == nil || !strings.Contains(err.Error(), "supported distributions are debian and ubuntu") {
		t.Fatalf("unexpected distribution error: %v", err)
	}
	if _, err := DefaultProviderRegistry().Resolve(OSRelease{ID: "debian", VersionID: "10", Codename: "buster"}); err == nil || !strings.Contains(err.Error(), "11, 12, 13") {
		t.Fatalf("unexpected version error: %v", err)
	}
}

func TestProviderRegistryAcceptsAdditionalProviders(t *testing.T) {
	registry := NewProviderRegistry(newAPTSystemdProvider("futurelinux", []string{"1"}))
	provider, err := registry.Resolve(OSRelease{ID: "futurelinux", VersionID: "1", Codename: "next"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if provider.ID() != "futurelinux" || provider.PackageManager().Name() != "apt" {
		t.Fatalf("unexpected custom provider: %s/%s", provider.ID(), provider.PackageManager().Name())
	}
}

func TestProvidersUseDistributionSpecificDockerRepositories(t *testing.T) {
	tests := []struct {
		provider DistributionProvider
		release  OSRelease
		expected string
	}{
		{NewUbuntuProvider(), OSRelease{ID: "ubuntu", VersionID: "24.04", Codename: "noble"}, "https://download.docker.com/linux/ubuntu"},
		{NewDebianProvider(), OSRelease{ID: "debian", VersionID: "12", Codename: "bookworm"}, "https://download.docker.com/linux/debian"},
	}
	for _, test := range tests {
		spec, err := test.provider.DockerSpec(test.release, "amd64")
		if err != nil {
			t.Fatal(err)
		}
		if len(spec.Repository) != 2 || !strings.Contains(spec.Repository[0].URL, test.expected) || !strings.Contains(string(spec.Repository[1].Content), test.expected) {
			t.Fatalf("unexpected repository spec for %s: %#v", test.release.ID, spec.Repository)
		}
	}
}
