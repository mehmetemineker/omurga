package secret

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"omurga/internal/host"
)

func TestEncryptedStoreLifecycleAndMaterialization(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(host.DefaultPaths(root))
	value := []byte("not written in plaintext")
	if err := manager.Set("demo", "production", "database-password", value); err != nil {
		t.Fatal(err)
	}
	storePath, err := manager.StorePath("demo", "production")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, value) {
		t.Fatal("encrypted store contains the plaintext secret")
	}
	if err := manager.Set("demo", "production", "database-password", []byte("replacement")); err != nil {
		t.Fatal(err)
	}
	value = []byte("replacement")
	names, err := manager.List("demo", "production")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "database-password" {
		t.Fatalf("unexpected names: %#v", names)
	}
	runtimeRoot := filepath.Join(root, "run", "secrets")
	exists, err := manager.Materialize("demo", "production", runtimeRoot, []MaterializeSpec{{Name: "database-password", Mode: 0o400}})
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected encrypted store to exist")
	}
	materialized, err := os.ReadFile(filepath.Join(runtimeRoot, "database-password"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(materialized, value) {
		t.Fatalf("unexpected materialized value: %q", materialized)
	}
	removed, err := manager.Remove("demo", "production", "database-password")
	if err != nil || !removed {
		t.Fatalf("remove failed: removed=%v err=%v", removed, err)
	}
}

func TestMaterializeWithoutStorePreservesManualRuntimeSecrets(t *testing.T) {
	manager := NewManager(host.DefaultPaths(t.TempDir()))
	exists, err := manager.Materialize("demo", "default", t.TempDir(), []MaterializeSpec{{Name: "token"}})
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("did not expect an encrypted store")
	}
}

func TestParseMode(t *testing.T) {
	mode, err := ParseMode("0440")
	if err != nil || mode != 0o440 {
		t.Fatalf("unexpected mode: %v %v", mode, err)
	}
	if _, err := ParseMode("888"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}
