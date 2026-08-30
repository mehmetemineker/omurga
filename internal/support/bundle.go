package support

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"omurga/internal/buildinfo"
	"omurga/internal/host"
	"omurga/internal/state"
)

type Result struct {
	Path    string   `json:"path"`
	Entries []string `json:"entries"`
}

// Create writes a diagnostic archive containing host health and service status.
// It deliberately excludes configuration files, logs, environment values, and
// secret directories because support bundles are commonly shared with others.
func Create(ctx context.Context, paths host.Paths, runner host.Runner, output string) (Result, error) {
	if output == "" {
		return Result{}, fmt.Errorf("support bundle output path is required")
	}
	if !filepath.IsAbs(output) {
		return Result{}, fmt.Errorf("support bundle output path must be absolute")
	}
	if runner == nil {
		return Result{}, fmt.Errorf("support bundle command runner is required")
	}

	entries := collect(ctx, paths, runner)
	if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
		return Result{}, fmt.Errorf("could not create support bundle directory: %w", err)
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Result{}, fmt.Errorf("could not create support bundle: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(output)
		}
	}()

	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range entries {
		if err := writeEntry(tarWriter, name, content); err != nil {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			_ = file.Close()
			return Result{}, fmt.Errorf("could not write support bundle entry %s: %w", name, err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		_ = gzipWriter.Close()
		_ = file.Close()
		return Result{}, fmt.Errorf("could not finalize support bundle: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		_ = file.Close()
		return Result{}, fmt.Errorf("could not finalize support bundle compression: %w", err)
	}
	if err := file.Close(); err != nil {
		return Result{}, fmt.Errorf("could not close support bundle: %w", err)
	}
	keep = true

	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	// collect uses a fixed set of names, so this is only for stable output.
	sort.Strings(names)
	return Result{Path: output, Entries: names}, nil
}

func collect(ctx context.Context, paths host.Paths, runner host.Runner) map[string][]byte {
	report := host.RunDoctor(ctx, paths, runner)
	doctor, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		doctor = []byte(`{"error":"could not encode doctor report"}`)
	}

	metadata, _ := json.MarshalIndent(struct {
		GeneratedAt string `json:"generatedAt"`
		Version     string `json:"version"`
		Notice      string `json:"notice"`
	}{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Version:     buildinfo.Version,
		Notice:      "This bundle excludes configuration files, logs, environment values, and secret contents.",
	}, "", "  ")

	projects := []state.Deployment{}
	if _, statErr := os.Stat(paths.StateDB); statErr == nil {
		if store, openErr := state.OpenReadOnly(ctx, paths.StateDB); openErr == nil {
			projects, _ = store.ListDeployments(ctx)
			_ = store.Close()
		}
	}
	projectData, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		projectData = []byte("[]")
	}

	return map[string][]byte{
		"metadata.json":         append(metadata, '\n'),
		"doctor.json":           append(doctor, '\n'),
		"projects.json":         append(projectData, '\n'),
		"docker-containers.txt": []byte(commandOutput(ctx, runner, "docker", "ps", "--all", "--format", "{{.Names}}\t{{.Image}}\t{{.Status}}")),
		"systemd-failed.txt":    []byte(commandOutput(ctx, runner, "systemctl", "--failed", "--no-legend", "--plain")),
		"caddy-status.txt":      []byte(commandOutput(ctx, runner, "systemctl", "is-active", "caddy")),
	}
}

func commandOutput(ctx context.Context, runner host.Runner, name string, args ...string) string {
	if _, err := runner.LookPath(name); err != nil {
		return name + ": not installed\n"
	}
	output, err := runner.Run(ctx, name, args...)
	output = strings.TrimSpace(output)
	if err != nil {
		if output == "" {
			return "error: " + err.Error() + "\n"
		}
		return "error: " + output + "\n"
	}
	if output == "" {
		return "(no output)\n"
	}
	return output + "\n"
}

func writeEntry(writer *tar.Writer, name string, content []byte) error {
	header := &tar.Header{
		Name:    name,
		Mode:    0o600,
		Size:    int64(len(content)),
		ModTime: time.Unix(0, 0).UTC(),
	}
	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	_, err := io.Copy(writer, bytes.NewReader(content))
	return err
}
