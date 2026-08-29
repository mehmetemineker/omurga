package environment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetAndUnsetServiceValue(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "omurga.yaml")
	if err := os.WriteFile(manifestPath, []byte("version: 1\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	path, err := Set(manifestPath, "production", "app", "LOG_LEVEL", "warning")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), "LOG_LEVEL: warning") {
		t.Fatalf("unexpected environment: %s %v", content, err)
	}
	_, removed, err := Unset(manifestPath, "production", "app", "LOG_LEVEL")
	if err != nil || !removed {
		t.Fatalf("unset failed: %v %v", removed, err)
	}
}
