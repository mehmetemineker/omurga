package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omurga/internal/host"
	"omurga/internal/manifest"
	"omurga/internal/state"
)

type lifecycleRunner struct {
	calls     []string
	failUp    int
	failCaddy int
	composePS string
}

func (r *lifecycleRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	call := strings.Join(append([]string{name}, args...), " ")
	r.calls = append(r.calls, call)
	if name == "docker" && containsArgument(args, "up") && r.failUp > 0 {
		r.failUp--
		return "container failed health check", errors.New("command failed")
	}
	if name == "caddy" && containsArgument(args, "validate") && r.failCaddy > 0 {
		r.failCaddy--
		return "invalid configuration", errors.New("command failed")
	}
	if name == "docker" && containsArgument(args, "ps") {
		return r.composePS, nil
	}
	return "", nil
}

func (r *lifecycleRunner) LookPath(name string) (string, error) {
	return "/usr/bin/" + name, nil
}

func TestDeployDryRunDoesNotMutateFilesystemOrState(t *testing.T) {
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	lifecycle := NewLifecycle(paths, &lifecycleRunner{})
	loaded := lifecycleProject(true)

	result, err := lifecycle.Deploy(context.Background(), loaded, true)
	if err != nil {
		t.Fatalf("Deploy() dry-run error = %v", err)
	}
	if !result.DryRun || result.Ports["app:3000"] == 0 || len(result.Steps) == 0 {
		t.Fatalf("unexpected deployment plan: %#v", result)
	}
	if _, err := os.Stat(paths.StateDB); !os.IsNotExist(err) {
		t.Fatalf("dry-run created a state database: %v", err)
	}
	if _, err := os.Stat(result.Layout.Root); !os.IsNotExist(err) {
		t.Fatalf("dry-run created a deployment directory: %v", err)
	}
}

func TestDeployStatusAndControls(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	loaded := lifecycleProject(true)
	layout := NewLifecycle(paths, &lifecycleRunner{}).Layout(loaded.Project.Name, loaded.Environment)
	writeTestFile(t, paths.CaddyFile, []byte(":80 { respond \"ok\" }\n"), 0o644)
	writeTestFile(t, filepath.Join(layout.RuntimeSecrets, "api-token"), []byte("secret"), 0o600)
	runner := &lifecycleRunner{composePS: `[{"Name":"omurga-demo-production-app-1","Service":"app","State":"running","Health":"healthy"}]`}
	lifecycle := NewLifecycle(paths, runner)

	result, err := lifecycle.Deploy(ctx, loaded, false)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if result.DryRun || len(result.Revision) != 64 {
		t.Fatalf("unexpected deployment result: %#v", result)
	}
	for _, step := range result.Steps {
		if step.Status != "completed" {
			t.Fatalf("deployment step was not completed: %#v", step)
		}
	}
	compose := readTestFile(t, layout.Compose)
	if !strings.Contains(compose, "127.0.0.1") || !strings.Contains(compose, "api-token") {
		t.Fatalf("unexpected Compose artifact:\n%s", compose)
	}
	caddy := readTestFile(t, layout.Caddy)
	if !strings.Contains(caddy, "demo.example.com") {
		t.Fatalf("unexpected Caddy artifact:\n%s", caddy)
	}
	secondResult, err := lifecycle.Deploy(ctx, loaded, false)
	if err != nil {
		t.Fatalf("second Deploy() error = %v", err)
	}
	if secondResult.Ports["app:3000"] != result.Ports["app:3000"] {
		t.Fatalf("gateway port changed across deployments: %#v %#v", result.Ports, secondResult.Ports)
	}
	baseCaddy := readTestFile(t, paths.CaddyFile)
	if strings.Count(baseCaddy, "# Omurga managed project routes") != 1 {
		t.Fatalf("managed Caddy import was not added once:\n%s", baseCaddy)
	}

	status, err := lifecycle.Status(ctx, loaded)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Deployed || status.Deployment.Status != "running" || len(status.Containers) != 1 {
		t.Fatalf("unexpected status: %#v", status)
	}
	if _, err := lifecycle.Control(ctx, loaded, "stop", false); err != nil {
		t.Fatalf("Control(stop) error = %v", err)
	}
	status = deploymentState(t, ctx, paths, loaded)
	if status.Deployment.Status != "stopped" {
		t.Fatalf("deployment status = %q, want stopped", status.Deployment.Status)
	}
	if _, err := lifecycle.Control(ctx, loaded, "restart", false); err != nil {
		t.Fatalf("Control(restart) error = %v", err)
	}
	status = deploymentState(t, ctx, paths, loaded)
	if status.Deployment.Status != "running" {
		t.Fatalf("deployment status = %q, want running", status.Deployment.Status)
	}
	assertCallContains(t, runner.calls, "docker compose --project-name omurga-demo-production")
	assertCallContains(t, runner.calls, "caddy validate --config")
	assertCallContains(t, runner.calls, "systemctl reload caddy")
}

func TestDeployRestoresComposeAndPreservesCaddyWhenHealthCheckFails(t *testing.T) {
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	loaded := lifecycleProject(false)
	runner := &lifecycleRunner{failUp: 1}
	lifecycle := NewLifecycle(paths, runner)
	layout := lifecycle.Layout(loaded.Project.Name, loaded.Environment)
	oldCompose := []byte("services:\n  old:\n    image: example/old:1\n")
	oldCaddy := []byte("old.example.com { respond \"old\" }\n")
	writeTestFile(t, layout.Compose, oldCompose, 0o640)
	writeTestFile(t, layout.Caddy, oldCaddy, 0o640)
	writeTestFile(t, paths.CaddyFile, []byte("import /existing/*.caddy\n"), 0o644)

	_, err := lifecycle.Deploy(context.Background(), loaded, false)
	if err == nil || !strings.Contains(err.Error(), "did not become healthy") {
		t.Fatalf("expected health failure, got: %v", err)
	}
	if got := readTestFile(t, layout.Compose); got != string(oldCompose) {
		t.Fatalf("Compose artifact was not restored:\n%s", got)
	}
	if got := readTestFile(t, layout.Caddy); got != string(oldCaddy) {
		t.Fatalf("Caddy artifact changed before container health succeeded:\n%s", got)
	}
	if countCallsContaining(runner.calls, " up ") != 2 {
		t.Fatalf("previous Compose deployment was not reconciled: %#v", runner.calls)
	}
}

func TestDeployRestoresArtifactsWhenCaddyValidationFails(t *testing.T) {
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	loaded := lifecycleProject(false)
	runner := &lifecycleRunner{failCaddy: 1}
	lifecycle := NewLifecycle(paths, runner)
	layout := lifecycle.Layout(loaded.Project.Name, loaded.Environment)
	oldCompose := []byte("services:\n  old:\n    image: example/old:1\n")
	oldCaddy := []byte("old.example.com { respond \"old\" }\n")
	oldBase := []byte("import /existing/*.caddy\n")
	writeTestFile(t, layout.Compose, oldCompose, 0o640)
	writeTestFile(t, layout.Caddy, oldCaddy, 0o640)
	writeTestFile(t, paths.CaddyFile, oldBase, 0o644)

	_, err := lifecycle.Deploy(context.Background(), loaded, false)
	if err == nil || !strings.Contains(err.Error(), "Caddy configuration is invalid") {
		t.Fatalf("expected Caddy validation failure, got: %v", err)
	}
	if got := readTestFile(t, layout.Compose); got != string(oldCompose) {
		t.Fatalf("Compose artifact was not restored:\n%s", got)
	}
	if got := readTestFile(t, layout.Caddy); got != string(oldCaddy) {
		t.Fatalf("Caddy artifact was not restored:\n%s", got)
	}
	if got := readTestFile(t, paths.CaddyFile); got != string(oldBase) {
		t.Fatalf("base Caddyfile was not restored:\n%s", got)
	}
}

func TestDeployWithoutGatewayDoesNotRequireOrRunCaddy(t *testing.T) {
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	loaded := lifecycleProject(false)
	loaded.Project.Gateway.Routes = nil
	runner := &lifecycleRunner{}
	lifecycle := NewLifecycle(paths, runner)

	result, err := lifecycle.Deploy(context.Background(), loaded, false)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if result.Ports == nil || len(result.Ports) != 0 {
		t.Fatalf("unexpected gateway ports: %#v", result.Ports)
	}
	for _, call := range runner.calls {
		if strings.HasPrefix(call, "caddy ") || strings.HasPrefix(call, "systemctl ") {
			t.Fatalf("gateway command ran for a project without routes: %s", call)
		}
	}
	store, err := state.OpenReadOnly(context.Background(), paths.StateDB)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	defer store.Close()
	deployment, exists, err := store.GetDeployment(context.Background(), "demo", "production")
	if err != nil || !exists || deployment.CaddyPath != "" {
		t.Fatalf("unexpected deployment state: %#v, %v, %v", deployment, exists, err)
	}
}

func lifecycleProject(withSecret bool) manifest.LoadedProject {
	service := manifest.Service{
		Image:   "example/app:1",
		Expose:  []int{3000},
		Volumes: []manifest.VolumeMount{{Name: "uploads", Target: "/app/uploads"}},
	}
	if withSecret {
		service.Secrets = []manifest.SecretMount{{Name: "api-token", Target: "/run/secrets/api-token"}}
	}
	https := false
	return manifest.LoadedProject{
		Path:        "/projects/demo/omurga.yaml",
		Environment: "production",
		Project: manifest.Project{
			Version:  1,
			Name:     "demo",
			Services: map[string]manifest.Service{"app": service},
			Gateway: manifest.Gateway{Routes: []manifest.Route{{
				Domain: "demo.example.com", Service: "app", Port: 3000, HTTPS: &https,
			}}},
		},
	}
}

func deploymentState(t *testing.T, ctx context.Context, paths host.Paths, loaded manifest.LoadedProject) StatusResult {
	t.Helper()
	store, err := state.OpenReadOnly(ctx, paths.StateDB)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	defer store.Close()
	deployment, exists, err := store.GetDeployment(ctx, loaded.Project.Name, loaded.Environment)
	if err != nil || !exists {
		t.Fatalf("GetDeployment() = %#v, %v, %v", deployment, exists, err)
	}
	return StatusResult{Project: loaded.Project.Name, Environment: loaded.Environment, Deployed: true, Deployment: &deployment}
}

func writeTestFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("could not create test directory: %v", err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("could not write test file: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("could not set test file mode: %v", err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read test file %s: %v", path, err)
	}
	return string(data)
}

func containsArgument(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}

func assertCallContains(t *testing.T, calls []string, expected string) {
	t.Helper()
	for _, call := range calls {
		if strings.Contains(call, expected) {
			return
		}
	}
	t.Fatalf("command containing %q was not called: %#v", expected, calls)
}

func countCallsContaining(calls []string, expected string) int {
	count := 0
	for _, call := range calls {
		if strings.Contains(call, expected) {
			count++
		}
	}
	return count
}
