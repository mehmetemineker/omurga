package host

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitializeCreatesManagedDirectoriesAndConfig(t *testing.T) {
	root := t.TempDir()
	paths := DefaultPaths(root)
	if err := os.MkdirAll(filepath.Dir(paths.OSRelease), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.OSRelease, []byte("ID=ubuntu\nVERSION_ID=22.04\nPRETTY_NAME=\"Ubuntu 22.04 LTS\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Initialize(paths, false)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if len(result.Actions) != len(paths.ManagedDirectories())+1 {
		t.Fatalf("unexpected action count: %d", len(result.Actions))
	}
	for _, directory := range paths.ManagedDirectories() {
		info, err := os.Stat(directory.Path)
		if err != nil {
			t.Fatalf("managed directory missing: %s: %v", directory.Path, err)
		}
		if info.Mode().Perm() != directory.Mode.Perm() {
			t.Fatalf("unexpected mode for %s: %o", directory.Path, info.Mode().Perm())
		}
	}
	if _, err := os.Stat(paths.ConfigFile); err != nil {
		t.Fatalf("config file missing: %v", err)
	}

	second, err := Initialize(paths, false)
	if err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}
	for _, action := range second.Actions {
		if action.Changed {
			t.Fatalf("second initialization should be idempotent: %#v", action)
		}
	}
}

func TestInitializeDryRunDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	paths := DefaultPaths(root)
	if err := os.MkdirAll(filepath.Dir(paths.OSRelease), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.OSRelease, []byte("ID=ubuntu\nVERSION_ID=24.04\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Initialize(paths, true)
	if err != nil {
		t.Fatalf("Initialize(dryRun) error = %v", err)
	}
	if !result.DryRun {
		t.Fatal("expected dry-run result")
	}
	if _, err := os.Stat(paths.ConfigRoot); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote to the filesystem: %v", err)
	}
}
