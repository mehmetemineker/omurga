package support

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omurga/internal/host"
)

type bundleRunner struct{}

func (bundleRunner) LookPath(name string) (string, error) { return "/usr/bin/" + name, nil }

func (bundleRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	if name == "id" {
		return "0", nil
	}
	if name == "ufw" {
		return "Status: active\n", nil
	}
	if name == "df" {
		return "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/root 100 10 90 10% /\n", nil
	}
	if name == "docker" && contains(args, "ps") {
		return "omurga-demo\tnginx:latest\tUp 2 hours\n", nil
	}
	if name == "caddy" && contains(args, "validate") {
		return "", nil
	}
	if name == "systemctl" && contains(args, "is-active") {
		return "active", nil
	}
	return "ok", nil
}

func TestCreateBundleExcludesSecretContent(t *testing.T) {
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	if err := os.MkdirAll(filepath.Dir(paths.OSRelease), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.OSRelease, []byte("ID=debian\nVERSION_ID=13\nPRETTY_NAME=Debian\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "support.tar.gz")
	result, err := Create(context.Background(), paths, bundleRunner{}, output)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Path != output || len(result.Entries) == 0 {
		t.Fatalf("unexpected result: %#v", result)
	}

	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	archive := tar.NewReader(gzipReader)
	var names []string
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
		content, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), "password") || strings.Contains(string(content), "token") {
			t.Fatalf("support bundle contains secret-like content in %s: %s", header.Name, content)
		}
		if header.Mode&0o077 != 0 {
			t.Fatalf("bundle entry %s is too broadly readable: %o", header.Name, header.Mode)
		}
	}
	if !contains(names, "doctor.json") || !contains(names, "projects.json") {
		t.Fatalf("expected diagnostic entries, got %v", names)
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
