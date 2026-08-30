package project

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omurga/internal/host"
	"omurga/internal/manifest"
	"omurga/internal/state"
)

type lifecycleRunner struct {
	calls        []string
	failUp       int
	failCaddy    int
	composePS    string
	streamOutput string
}

type dynamicLifecycleRunner struct {
	*lifecycleRunner
}

func (r *dynamicLifecycleRunner) SupportsDynamicPorts() bool { return true }

func (r *dynamicLifecycleRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	if name == "docker" && containsArgument(args, "port") {
		r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
		return "127.0.0.1:32768", nil
	}
	return r.lifecycleRunner.Run(ctx, name, args...)
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

func (r *lifecycleRunner) Stream(_ context.Context, stdout, _ io.Writer, name string, args ...string) error {
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	_, err := io.WriteString(stdout, r.streamOutput)
	return err
}

func (r *lifecycleRunner) RunIO(_ context.Context, _ io.Reader, stdout, _ io.Writer, name string, args ...string) error {
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	_, err := io.WriteString(stdout, r.streamOutput)
	return err
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

func TestDeployImageOverridesResolvedServiceWithoutChangingManifest(t *testing.T) {
	loaded := lifecycleProject(false)
	original := loaded.Project.Services["app"].Image
	lifecycle := NewLifecycle(host.DefaultPaths(t.TempDir()), &lifecycleRunner{})

	result, err := lifecycle.DeployImage(context.Background(), loaded, "app", "ghcr.io/acme/demo:build-42@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", true)
	if err != nil {
		t.Fatalf("DeployImage() error = %v", err)
	}
	if !result.DryRun || loaded.Project.Services["app"].Image != original {
		t.Fatalf("DeployImage modified the source manifest: result=%#v image=%q", result, loaded.Project.Services["app"].Image)
	}
}

func TestEnsureCaddyImportMigratesLegacyImport(t *testing.T) {
	root := t.TempDir()
	caddyFile := filepath.Join(root, "etc", "caddy", "Caddyfile")
	newDirectory := filepath.Dir(caddyFile)
	for _, legacyImport := range []string{"/etc/omurga/caddy/projects/*.caddy", "/etc/caddy/omurga/projects/*.caddy"} {
		writeTestFile(t, caddyFile, []byte(":80 { respond \"ok\" }\n\nimport "+legacyImport+"\n"), 0o644)

		if err := ensureCaddyImport(caddyFile, newDirectory); err != nil {
			t.Fatalf("ensureCaddyImport() error = %v", err)
		}
		content := readTestFile(t, caddyFile)
		if strings.Contains(content, legacyImport) || !strings.Contains(content, "import "+filepath.ToSlash(filepath.Join(newDirectory, "omurga-*.caddy"))) {
			t.Fatalf("legacy Caddy import was not migrated:\n%s", content)
		}
	}
}

func TestEnsureCaddyImportRepairsCaddyRuntimePermissions(t *testing.T) {
	root := t.TempDir()
	caddyDirectory := filepath.Join(root, "etc", "caddy")
	caddyFile := filepath.Join(caddyDirectory, "Caddyfile")
	importLine := "import " + filepath.ToSlash(filepath.Join(caddyDirectory, "omurga-*.caddy"))
	writeTestFile(t, caddyFile, []byte(":80 { respond \"ok\" }\n\n"+importLine+"\n"), 0o600)
	if err := os.Chmod(caddyDirectory, 0o700); err != nil {
		t.Fatalf("could not restrict test Caddy directory: %v", err)
	}

	if err := ensureCaddyImport(caddyFile, caddyDirectory); err != nil {
		t.Fatalf("ensureCaddyImport() error = %v", err)
	}
	if info, err := os.Stat(caddyDirectory); err != nil || info.Mode().Perm() != caddyDirectoryMode {
		t.Fatalf("Caddy directory mode = %v, %v; want %v", infoMode(info), err, caddyDirectoryMode)
	}
	if info, err := os.Stat(caddyFile); err != nil || info.Mode().Perm() != caddyArtifactMode {
		t.Fatalf("Caddyfile mode = %v, %v; want %v", infoMode(info), err, caddyArtifactMode)
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
	if info, err := os.Stat(filepath.Dir(layout.Caddy)); err != nil || info.Mode().Perm() != caddyDirectoryMode {
		t.Fatalf("Caddy directory mode = %v, %v; want %v", infoMode(info), err, caddyDirectoryMode)
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

func TestStatelessRedeployUsesBlueGreenSlots(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	loaded := lifecycleProject(false)
	service := serviceWithImage(loaded.Project.Services["app"], "example/app:2")
	service.Volumes = nil
	loaded.Project.Services["app"] = service
	writeTestFile(t, paths.CaddyFile, []byte(":80 { respond \"ok\" }\n"), 0o644)
	runner := &dynamicLifecycleRunner{lifecycleRunner: &lifecycleRunner{}}
	lifecycle := NewLifecycle(paths, runner)

	if _, err := lifecycle.Deploy(ctx, loaded, false); err != nil {
		t.Fatalf("initial Deploy() error = %v", err)
	}
	result, err := lifecycle.Deploy(ctx, loaded, false)
	if err != nil {
		t.Fatalf("blue-green Deploy() error = %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(result.Layout.Compose), "/slots/a/compose.yaml") {
		t.Fatalf("active Compose path = %q, want slot a", result.Layout.Compose)
	}
	if result.Ports["app:3000"] != 32768 {
		t.Fatalf("active gateway port = %#v, want dynamically resolved port", result.Ports)
	}
	assertCallContains(t, runner.calls, "docker compose --project-name omurga-demo-production-slot-a")
	assertCallContains(t, runner.calls, "docker compose --project-name omurga-demo-production-slot-a --file")
	assertCallContains(t, runner.calls, " port app 3000")
	assertCallContains(t, runner.calls, "docker compose --project-name omurga-demo-production --file")
	if !strings.Contains(runner.calls[len(runner.calls)-1], " down --remove-orphans") {
		t.Fatalf("previous project was not stopped last: %#v", runner.calls)
	}
}

func TestBlueGreenHealthFailureKeepsCurrentDeployment(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	loaded := lifecycleProject(false)
	service := loaded.Project.Services["app"]
	service.Volumes = nil
	loaded.Project.Services["app"] = service
	writeTestFile(t, paths.CaddyFile, []byte(":80 { respond \"ok\" }\n"), 0o644)
	runner := &dynamicLifecycleRunner{lifecycleRunner: &lifecycleRunner{}}
	lifecycle := NewLifecycle(paths, runner)
	if _, err := lifecycle.Deploy(ctx, loaded, false); err != nil {
		t.Fatalf("initial Deploy() error = %v", err)
	}
	layout := lifecycle.Layout(loaded.Project.Name, loaded.Environment)
	beforeCompose := readTestFile(t, layout.Compose)
	beforeCaddy := readTestFile(t, layout.Caddy)
	runner.failUp = 1
	_, err := lifecycle.Deploy(ctx, loaded, false)
	if err == nil || !strings.Contains(err.Error(), "automatic rollback kept the current deployment active") {
		t.Fatalf("expected automatic rollback error, got: %v", err)
	}
	if got := readTestFile(t, layout.Compose); got != beforeCompose {
		t.Fatalf("current Compose artifact changed after failed replacement:\n%s", got)
	}
	if got := readTestFile(t, layout.Caddy); got != beforeCaddy {
		t.Fatalf("current Caddy artifact changed after failed replacement:\n%s", got)
	}
	assertCallContains(t, runner.calls, "docker compose --project-name omurga-demo-production-slot-a --file")
	assertCallContains(t, runner.calls, "down --remove-orphans")
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

func TestLogsStreamsSelectedService(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	loaded := lifecycleProject(false)
	writeTestFile(t, paths.CaddyFile, []byte(":80 { respond \"ok\" }\n"), 0o644)
	runner := &lifecycleRunner{streamOutput: "app-1 | ready\n"}
	lifecycle := NewLifecycle(paths, runner)
	if _, err := lifecycle.Deploy(ctx, loaded, false); err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}

	output := &bytes.Buffer{}
	result, err := lifecycle.Logs(ctx, loaded, LogOptions{
		Follow: true, Tail: "25", Since: "10m", Timestamps: true, Services: []string{"app"},
	}, false, output, output)
	if err != nil {
		t.Fatalf("Logs() error = %v", err)
	}
	if output.String() != runner.streamOutput || !strings.Contains(strings.Join(result.Command, " "), "logs --follow --tail 25 --since 10m --timestamps app") {
		t.Fatalf("unexpected log result: %#v output=%q", result, output.String())
	}
	if _, err := lifecycle.Logs(ctx, loaded, LogOptions{Tail: "invalid"}, true, output, output); err == nil {
		t.Fatal("Logs() accepted an invalid tail value")
	}
	if _, err := lifecycle.Logs(ctx, loaded, LogOptions{Tail: "10", Services: []string{"missing"}}, true, output, output); err == nil {
		t.Fatal("Logs() accepted an unknown service")
	}
}

func TestExecUsesActiveDeploymentAndDeclaredService(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	writeTestFile(t, paths.CaddyFile, []byte(":80 { respond \"ok\" }\n"), 0o644)
	runner := &lifecycleRunner{streamOutput: "ready\n"}
	lifecycle := NewLifecycle(paths, runner)
	loaded := lifecycleProject(false)
	if _, err := lifecycle.Deploy(ctx, loaded, false); err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}

	output := &bytes.Buffer{}
	result, err := lifecycle.Exec(ctx, loaded, "app", ExecOptions{Command: []string{"nginx", "-T"}}, false, strings.NewReader(""), output, output)
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	composeCommand := strings.Join(result.Command, " ")
	if output.String() != "ready\n" || !strings.Contains(composeCommand, "compose --project-name "+ComposeProjectName("demo", "production")+" --file") || !strings.Contains(composeCommand, "exec --no-TTY app nginx -T") {
		t.Fatalf("unexpected exec result: %#v output=%q calls=%v", result, output.String(), runner.calls)
	}
	if _, err := lifecycle.Exec(ctx, loaded, "missing", ExecOptions{Command: []string{"true"}}, true, nil, output, output); err == nil {
		t.Fatal("Exec() accepted an undeclared service")
	}
}

func TestRollbackSwapsHealthyArtifactsAndCanRollForward(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	writeTestFile(t, paths.CaddyFile, []byte(":80 { respond \"ok\" }\n"), 0o644)
	runner := &lifecycleRunner{}
	lifecycle := NewLifecycle(paths, runner)
	first := lifecycleProject(false)
	first.Project.Services["app"] = serviceWithImage(first.Project.Services["app"], "example/app:1")
	firstResult, err := lifecycle.Deploy(ctx, first, false)
	if err != nil {
		t.Fatalf("first Deploy() error = %v", err)
	}
	second := lifecycleProject(false)
	second.Project.Services["app"] = serviceWithImage(second.Project.Services["app"], "example/app:2")
	second.Project.Gateway.Routes[0].Domain = "v2.example.com"
	secondResult, err := lifecycle.Deploy(ctx, second, false)
	if err != nil {
		t.Fatalf("second Deploy() error = %v", err)
	}
	layout := lifecycle.Layout("demo", "production")
	if !strings.Contains(readTestFile(t, layout.Compose), "example/app:2") {
		t.Fatal("second deployment was not active before rollback")
	}

	plan, err := lifecycle.Rollback(ctx, second, true)
	if err != nil {
		t.Fatalf("Rollback() dry-run error = %v", err)
	}
	if !plan.DryRun || plan.FromRevision != secondResult.Revision || plan.ToRevision != firstResult.Revision {
		t.Fatalf("unexpected rollback plan: %#v", plan)
	}
	result, err := lifecycle.Rollback(ctx, second, false)
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if result.ToRevision != firstResult.Revision || !strings.Contains(readTestFile(t, layout.Compose), "example/app:1") {
		t.Fatalf("previous Compose artifact was not activated: %#v", result)
	}
	if !strings.Contains(readTestFile(t, layout.Caddy), "demo.example.com") || !strings.Contains(readTestFile(t, layout.PreviousCompose), "example/app:2") {
		t.Fatal("rollback artifacts were not swapped")
	}
	status := deploymentState(t, ctx, paths, second)
	if status.Deployment.Revision != firstResult.Revision {
		t.Fatalf("deployment revision = %q, want %q", status.Deployment.Revision, firstResult.Revision)
	}

	forward, err := lifecycle.Rollback(ctx, second, false)
	if err != nil {
		t.Fatalf("second Rollback() error = %v", err)
	}
	if forward.ToRevision != secondResult.Revision || !strings.Contains(readTestFile(t, layout.Compose), "example/app:2") {
		t.Fatalf("rollback did not roll forward: %#v", forward)
	}
}

func TestRollbackFailureRestoresCurrentAndPreviousArtifacts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	writeTestFile(t, paths.CaddyFile, []byte(":80 { respond \"ok\" }\n"), 0o644)
	runner := &lifecycleRunner{}
	lifecycle := NewLifecycle(paths, runner)
	first := lifecycleProject(false)
	if _, err := lifecycle.Deploy(ctx, first, false); err != nil {
		t.Fatalf("first Deploy() error = %v", err)
	}
	second := lifecycleProject(false)
	second.Project.Services["app"] = serviceWithImage(second.Project.Services["app"], "example/app:2")
	if _, err := lifecycle.Deploy(ctx, second, false); err != nil {
		t.Fatalf("second Deploy() error = %v", err)
	}
	layout := lifecycle.Layout("demo", "production")
	currentBefore := readTestFile(t, layout.Compose)
	previousBefore := readTestFile(t, layout.PreviousCompose)
	runner.failUp = 1
	if _, err := lifecycle.Rollback(ctx, second, false); err == nil {
		t.Fatal("Rollback() succeeded despite a failed health check")
	}
	if got := readTestFile(t, layout.Compose); got != currentBefore {
		t.Fatalf("current Compose artifact was not restored:\n%s", got)
	}
	if got := readTestFile(t, layout.PreviousCompose); got != previousBefore {
		t.Fatalf("previous Compose artifact was not restored:\n%s", got)
	}
}

func TestDeletePreservesDataByDefaultAndCanPurgeExplicitly(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	paths := host.DefaultPaths(root)
	writeTestFile(t, paths.CaddyFile, []byte(":80 { respond \"ok\" }\n"), 0o644)
	runner := &lifecycleRunner{}
	lifecycle := NewLifecycle(paths, runner)
	loaded := lifecycleProject(false)
	if _, err := lifecycle.Deploy(ctx, loaded, false); err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	layout := lifecycle.Layout("demo", "production")
	dataFile := filepath.Join(layout.Root, "data", "uploads", "asset.txt")
	writeTestFile(t, dataFile, []byte("keep"), 0o640)

	result, err := lifecycle.Delete(ctx, loaded, DeleteOptions{})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !result.DataPreserved || result.State.DeploymentsDeleted != 1 || result.State.PortsReleased != 1 {
		t.Fatalf("unexpected delete result: %#v", result)
	}
	if got := readTestFile(t, dataFile); got != "keep" {
		t.Fatalf("persistent data changed: %q", got)
	}
	if _, err := os.Stat(layout.Compose); !os.IsNotExist(err) {
		t.Fatalf("Compose artifact still exists: %v", err)
	}
	if _, err := os.Stat(layout.Caddy); !os.IsNotExist(err) {
		t.Fatalf("Caddy artifact still exists: %v", err)
	}
	store, err := state.OpenReadOnly(ctx, paths.StateDB)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	if _, exists, err := store.GetDeployment(ctx, "demo", "production"); err != nil || exists {
		store.Close()
		t.Fatalf("deployment state still exists: %v, %v", exists, err)
	}
	ports, err := store.ListGatewayPorts(ctx, "demo", "production")
	store.Close()
	if err != nil || len(ports) != 0 {
		t.Fatalf("gateway ports were not released: %#v, %v", ports, err)
	}

	if _, err := lifecycle.Delete(ctx, loaded, DeleteOptions{PurgeData: true}); err != nil {
		t.Fatalf("Delete(purge preserved data) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.Root, "data")); !os.IsNotExist(err) {
		t.Fatalf("persistent data was not purged: %v", err)
	}
}

func serviceWithImage(service manifest.Service, image string) manifest.Service {
	service.Image = image
	return service
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

func infoMode(info os.FileInfo) os.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode().Perm()
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
