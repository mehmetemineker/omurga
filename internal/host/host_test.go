package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	available map[string]bool
	outputs   map[string]string
	errors    map[string]error
	calls     []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	key := commandKey(name, args...)
	r.calls = append(r.calls, key)
	if err := r.errors[key]; err != nil {
		return r.outputs[key], err
	}
	if name == "gpg" {
		for index, arg := range args {
			if arg == "--output" && index+1 < len(args) {
				if err := os.WriteFile(args[index+1], []byte("dearmored-key"), 0o600); err != nil {
					return "", err
				}
			}
		}
	}
	if name == "df" {
		return "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/sda1 100000 20000 80000 20% /", nil
	}
	return r.outputs[key], nil
}

func (r *fakeRunner) LookPath(name string) (string, error) {
	if r.available[name] {
		return "/usr/bin/" + name, nil
	}
	return "", fmt.Errorf("not found")
}

func TestUpdatePackagesUsesSafeUpgradeByDefault(t *testing.T) {
	runner := &fakeRunner{outputs: map[string]string{}, errors: map[string]error{}}
	result, err := UpdatePackages(context.Background(), runner, NewUbuntuProvider(), false, false)
	if err != nil {
		t.Fatalf("UpdatePackages() error = %v", err)
	}
	if result.Mode != "safe" || len(runner.calls) != 2 {
		t.Fatalf("unexpected update result: %#v, calls: %#v", result, runner.calls)
	}
	if !strings.Contains(runner.calls[1], " upgrade") || strings.Contains(runner.calls[1], "full-upgrade") {
		t.Fatalf("safe update used the wrong command: %s", runner.calls[1])
	}
}

func TestUpdatePackagesDryRunDoesNotExecuteCommands(t *testing.T) {
	runner := &fakeRunner{}
	result, err := UpdatePackages(context.Background(), runner, NewUbuntuProvider(), true, true)
	if err != nil {
		t.Fatalf("UpdatePackages(dryRun) error = %v", err)
	}
	if len(runner.calls) != 0 || !result.DryRun || result.Mode != "full" {
		t.Fatalf("unexpected dry-run result: %#v, calls: %#v", result, runner.calls)
	}
}

func TestDoctorReportsHealthyInitializedHost(t *testing.T) {
	paths := createInitializedPaths(t)
	runner := healthyRunner()

	report := RunDoctor(context.Background(), paths, runner)
	if report.ExitCode() != 0 {
		t.Fatalf("expected healthy report, got: %#v", report)
	}
}

func TestDoctorReportsMissingDockerAsCritical(t *testing.T) {
	paths := createInitializedPaths(t)
	runner := healthyRunner()
	delete(runner.available, "docker")

	report := RunDoctor(context.Background(), paths, runner)
	if report.ExitCode() != 2 || report.Summary.Critical == 0 {
		t.Fatalf("expected critical report, got: %#v", report)
	}
}

func healthyRunner() *fakeRunner {
	return &fakeRunner{
		available: map[string]bool{
			"apt-get":   true,
			"docker":    true,
			"caddy":     true,
			"df":        true,
			"systemctl": true,
		},
		outputs: map[string]string{
			commandKey("id", "-u"):                                         "0",
			commandKey("apt-get", "--version"):                             "apt 2.4.14",
			commandKey("docker", "info", "--format", "{{.ServerVersion}}"): "27.5.1",
			commandKey("docker", "compose", "version", "--short"):          "2.32.4",
			commandKey("caddy", "version"):                                 "v2.9.1",
			commandKey("systemctl", "is-active", "--quiet", "docker"):      "",
			commandKey("systemctl", "is-active", "--quiet", "caddy"):       "",
		},
		errors: map[string]error{},
	}
}

func createInitializedPaths(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	paths := DefaultPaths(root)
	if err := os.MkdirAll(filepath.Dir(paths.OSRelease), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.OSRelease, []byte("ID=ubuntu\nVERSION_ID=24.04\nVERSION_CODENAME=noble\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, directory := range paths.ManagedDirectories() {
		if err := os.MkdirAll(directory.Path, directory.Mode); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

func commandKey(name string, args ...string) string {
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}

func TestMergedEnvironmentReplacesExistingValues(t *testing.T) {
	actual := mergedEnvironment([]string{"PATH=/bin", "AWS_ACCESS_KEY_ID=old"}, map[string]string{"AWS_ACCESS_KEY_ID": "new"})
	joined := strings.Join(actual, "\n")
	if strings.Contains(joined, "AWS_ACCESS_KEY_ID=old") || !strings.Contains(joined, "AWS_ACCESS_KEY_ID=new") {
		t.Fatalf("unexpected merged environment: %#v", actual)
	}
}
