package host

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndValidateSupportedUbuntu(t *testing.T) {
	path := filepath.Join(t.TempDir(), "os-release")
	content := "ID=ubuntu\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04.2 LTS\"\nUBUNTU_CODENAME=noble\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	release, err := LoadOSRelease(path)
	if err != nil {
		t.Fatalf("LoadOSRelease() error = %v", err)
	}
	if release.ID != "ubuntu" || release.VersionID != "24.04" || release.Codename != "noble" {
		t.Fatalf("unexpected release: %#v", release)
	}
	if err := ValidateSupportedUbuntu(release); err != nil {
		t.Fatalf("ValidateSupportedUbuntu() error = %v", err)
	}
}

func TestValidateSupportedUbuntuRejectsUnsupportedVersion(t *testing.T) {
	err := ValidateSupportedUbuntu(OSRelease{ID: "ubuntu", VersionID: "20.04"})
	if err == nil {
		t.Fatal("expected unsupported version error")
	}
}
