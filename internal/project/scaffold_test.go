package project

import (
	"os"
	"path/filepath"
	"testing"

	"omurga/internal/manifest"
)

func TestCreateProducesValidProjectAndEnvironment(t *testing.T) {
	parent := t.TempDir()
	result, err := Create("demo-app", parent, false)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := os.Stat(result.Manifest); err != nil {
		t.Fatalf("manifest was not created: %v", err)
	}
	loaded, err := manifest.Load(result.Directory, "production")
	if err != nil {
		t.Fatalf("created project is invalid: %v", err)
	}
	if loaded.Project.Name != "demo-app" || loaded.Project.Host != "production" {
		t.Fatalf("unexpected created project: %#v", loaded.Project)
	}
}

func TestCreateDryRunDoesNotCreateDirectory(t *testing.T) {
	parent := t.TempDir()
	result, err := Create("demo", parent, true)
	if err != nil {
		t.Fatalf("Create(dryRun) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "demo")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created a directory: %v", err)
	}
	if !result.DryRun {
		t.Fatal("expected dry-run result")
	}
}
