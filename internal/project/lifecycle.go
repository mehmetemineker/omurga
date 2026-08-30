package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"omurga/internal/gateway"
	"omurga/internal/host"
	"omurga/internal/manifest"
	"omurga/internal/progress"
	"omurga/internal/secret"
	"omurga/internal/state"
)

const (
	defaultDeployWaitSeconds = 120
	caddyDirectoryMode       = 0o755
	caddyArtifactMode        = 0o644
)

type Lifecycle struct {
	Paths    host.Paths
	Runner   host.Runner
	Services host.ServiceManager
	Progress *progress.Reporter
}

type DeploymentLayout struct {
	Root            string `json:"root"`
	Compose         string `json:"compose"`
	PreviousCompose string `json:"previousCompose"`
	Caddy           string `json:"caddy,omitempty"`
	PreviousCaddy   string `json:"previousCaddy,omitempty"`
	RuntimeSecrets  string `json:"runtimeSecrets"`
}

type LifecycleStep struct {
	Name    string   `json:"name"`
	Path    string   `json:"path,omitempty"`
	Command []string `json:"command,omitempty"`
	Status  string   `json:"status"`
}

type DeployResult struct {
	Project         string           `json:"project"`
	Environment     string           `json:"environment"`
	Revision        string           `json:"revision"`
	DryRun          bool             `json:"dryRun"`
	Ports           map[string]int   `json:"ports"`
	RequiredSecrets []string         `json:"requiredSecrets,omitempty"`
	Layout          DeploymentLayout `json:"layout"`
	Steps           []LifecycleStep  `json:"steps"`
}

type StatusResult struct {
	Project     string            `json:"project"`
	Environment string            `json:"environment"`
	Deployed    bool              `json:"deployed"`
	Deployment  *state.Deployment `json:"deployment,omitempty"`
	Containers  []map[string]any  `json:"containers,omitempty"`
}

type ControlResult struct {
	Project     string   `json:"project"`
	Environment string   `json:"environment"`
	Action      string   `json:"action"`
	DryRun      bool     `json:"dryRun"`
	Command     []string `json:"command"`
}

func NewLifecycle(paths host.Paths, runner host.Runner) Lifecycle {
	return Lifecycle{Paths: paths, Runner: runner, Services: host.NewSystemdServiceManager()}
}

func (l Lifecycle) WithServiceManager(services host.ServiceManager) Lifecycle {
	if services != nil {
		l.Services = services
	}
	return l
}

func (l Lifecycle) WithProgress(reporter *progress.Reporter) Lifecycle {
	l.Progress = reporter
	return l
}

func (l Lifecycle) runStep(ctx context.Context, label, name string, args ...string) (string, error) {
	task := l.Progress.Start(label)
	output, err := l.Runner.Run(ctx, name, args...)
	if err != nil {
		task.Fail(err)
		return output, err
	}
	task.Complete()
	return output, nil
}

func (l Lifecycle) serviceManager() host.ServiceManager {
	if l.Services == nil {
		return host.NewSystemdServiceManager()
	}
	return l.Services
}

func (l Lifecycle) Layout(projectName, environment string) DeploymentLayout {
	environment = gateway.EnvironmentKey(environment)
	root := filepath.Join(l.Paths.ProjectsState, projectName, environment)
	caddy := filepath.Join(l.Paths.CaddyProjects, "omurga-"+projectName+"-"+environment+".caddy")
	return DeploymentLayout{
		Root:            root,
		Compose:         filepath.Join(root, "compose.yaml"),
		PreviousCompose: filepath.Join(root, "compose.previous.yaml"),
		Caddy:           caddy,
		PreviousCaddy:   caddy + ".previous",
		RuntimeSecrets:  filepath.Join(l.Paths.RuntimeSecrets, projectName, environment),
	}
}

func (l Lifecycle) Deploy(ctx context.Context, loaded manifest.LoadedProject, dryRun bool) (DeployResult, error) {
	if l.Runner == nil {
		return DeployResult{}, fmt.Errorf("deployment command runner is required")
	}
	layout := l.Layout(loaded.Project.Name, loaded.Environment)
	secrets := requiredSecrets(loaded.Project)
	needsCaddy, err := deploymentNeedsCaddy(layout, loaded.Project)
	if err != nil {
		return DeployResult{}, err
	}
	if !dryRun {
		if err := l.checkPrerequisites(needsCaddy); err != nil {
			return DeployResult{}, err
		}
		materialized, err := secret.NewManager(l.Paths).Materialize(
			loaded.Project.Name,
			gateway.EnvironmentKey(loaded.Environment),
			layout.RuntimeSecrets,
			requiredSecretSpecs(loaded.Project),
		)
		if err != nil {
			return DeployResult{}, fmt.Errorf("could not materialize encrypted secrets: %w", err)
		}
		_ = materialized
		if err := ensureRuntimeSecrets(layout.RuntimeSecrets, secrets); err != nil {
			return DeployResult{}, err
		}
	}
	ports, store, err := l.resolvePorts(ctx, loaded.Project, loaded.Environment, dryRun)
	if err != nil {
		return DeployResult{}, err
	}
	if store != nil {
		defer store.Close()
	}

	options := DefaultRenderOptions(loaded.Project, loaded.Environment)
	options.DeploymentRoot = filepath.ToSlash(layout.Root)
	options.RuntimeSecretsRoot = filepath.ToSlash(layout.RuntimeSecrets)
	options.Ports = ports
	artifacts, err := Generate(loaded.Project, options)
	if err != nil {
		return DeployResult{}, err
	}
	revision := artifactRevision(artifacts)
	result := DeployResult{
		Project:         loaded.Project.Name,
		Environment:     gateway.EnvironmentKey(loaded.Environment),
		Revision:        revision,
		DryRun:          dryRun,
		Ports:           ports,
		RequiredSecrets: secrets,
		Layout:          layout,
		Steps:           deploymentPlan(layout, loaded.Project, loaded.Environment, needsCaddy, secrets, l.serviceManager().ReloadCommand("caddy")),
	}
	if dryRun {
		return result, nil
	}
	if current, exists, err := store.GetDeployment(ctx, loaded.Project.Name, gateway.EnvironmentKey(loaded.Environment)); err != nil {
		return DeployResult{}, fmt.Errorf("could not inspect current deployment: %w", err)
	} else if exists && current.Status == "running" && isBlueGreenEligible(loaded.Project, l.Runner) {
		return l.deployBlueGreen(ctx, loaded, layout, result, current, store)
	}

	if err := prepareDeploymentDirectories(layout, loaded.Project, needsCaddy); err != nil {
		return DeployResult{}, err
	}

	composeSnapshot, err := captureFile(layout.Compose)
	if err != nil {
		return DeployResult{}, fmt.Errorf("could not capture existing Compose artifact: %w", err)
	}
	if err := persistPrevious(layout.PreviousCompose, composeSnapshot); err != nil {
		return DeployResult{}, fmt.Errorf("could not preserve previous Compose artifact: %w", err)
	}
	if err := WriteArtifact(layout.Compose, artifacts.Compose, 0o640); err != nil {
		return DeployResult{}, fmt.Errorf("could not write Compose artifact: %w", err)
	}
	if _, err := l.runStep(ctx, "Validate generated Compose configuration", "docker", composeArgs(layout, loaded.Project.Name, loaded.Environment, "config", "--quiet")...); err != nil {
		restoreErr := restoreFile(layout.Compose, composeSnapshot)
		return DeployResult{}, errors.Join(fmt.Errorf("generated Compose configuration is invalid: %w", err), restoreErr)
	}
	if _, err := l.runStep(ctx, "Start and health-check project containers", "docker", composeArgs(layout, loaded.Project.Name, loaded.Environment,
		"up", "--detach", "--remove-orphans", "--wait", "--wait-timeout", fmt.Sprint(defaultDeployWaitSeconds))...); err != nil {
		rollbackErr := l.rollbackCompose(ctx, layout, loaded.Project.Name, loaded.Environment, composeSnapshot)
		return DeployResult{}, errors.Join(fmt.Errorf("project containers did not become healthy: %w", err), rollbackErr)
	}

	if needsCaddy {
		if err := l.applyCaddy(ctx, layout, artifacts.Caddy); err != nil {
			rollbackErr := l.rollbackCompose(ctx, layout, loaded.Project.Name, loaded.Environment, composeSnapshot)
			return DeployResult{}, errors.Join(err, rollbackErr)
		}
	}

	deployment := state.Deployment{
		Project:      loaded.Project.Name,
		Environment:  gateway.EnvironmentKey(loaded.Environment),
		Status:       "running",
		Revision:     revision,
		ManifestPath: loaded.Path,
		ComposePath:  layout.Compose,
		CaddyPath:    caddyStatePath(layout, len(artifacts.Caddy) > 0),
	}
	if err := store.PutDeployment(ctx, deployment); err != nil {
		return DeployResult{}, fmt.Errorf("project is running but deployment state could not be stored: %w", err)
	}
	for index := range result.Steps {
		result.Steps[index].Status = "completed"
	}
	return result, nil
}

func (l Lifecycle) Status(ctx context.Context, loaded manifest.LoadedProject) (StatusResult, error) {
	result := StatusResult{Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment)}
	if _, err := os.Stat(l.Paths.StateDB); os.IsNotExist(err) {
		return result, nil
	} else if err != nil {
		return StatusResult{}, fmt.Errorf("could not inspect state database: %w", err)
	}
	store, err := state.OpenReadOnly(ctx, l.Paths.StateDB)
	if err != nil {
		return StatusResult{}, err
	}
	defer store.Close()
	deployment, exists, err := store.GetDeployment(ctx, result.Project, result.Environment)
	if err != nil {
		return StatusResult{}, err
	}
	if !exists {
		return result, nil
	}
	result.Deployed = true
	result.Deployment = &deployment
	composeLayout := activeComposeLayout(l.Layout(result.Project, result.Environment), deployment)
	composeProject := activeComposeProjectName(composeLayout, result.Project, result.Environment)
	output, err := l.Runner.Run(ctx, "docker", composeArgsWithProjectName(composeLayout, composeProject, "ps", "--format", "json")...)
	if err != nil {
		return StatusResult{}, fmt.Errorf("could not inspect project containers: %w", err)
	}
	containers, err := parseComposePS(output)
	if err != nil {
		return StatusResult{}, err
	}
	result.Containers = containers
	return result, nil
}

func (l Lifecycle) Control(ctx context.Context, loaded manifest.LoadedProject, action string, dryRun bool) (ControlResult, error) {
	if action != "restart" && action != "stop" {
		return ControlResult{}, fmt.Errorf("project action must be restart or stop")
	}
	environment := gateway.EnvironmentKey(loaded.Environment)
	layout := l.Layout(loaded.Project.Name, environment)
	args := composeArgs(layout, loaded.Project.Name, environment, action)
	result := ControlResult{
		Project: loaded.Project.Name, Environment: environment, Action: action, DryRun: dryRun,
		Command: append([]string{"docker"}, args...),
	}
	if dryRun {
		return result, nil
	}
	if _, err := os.Stat(l.Paths.StateDB); os.IsNotExist(err) {
		return ControlResult{}, fmt.Errorf("project %s/%s is not deployed", result.Project, environment)
	} else if err != nil {
		return ControlResult{}, fmt.Errorf("could not inspect state database: %w", err)
	}
	store, err := state.Open(ctx, l.Paths.StateDB)
	if err != nil {
		return ControlResult{}, err
	}
	defer store.Close()
	if deployment, exists, err := store.GetDeployment(ctx, result.Project, environment); err != nil {
		return ControlResult{}, err
	} else if !exists {
		return ControlResult{}, fmt.Errorf("project %s/%s is not deployed", result.Project, environment)
	} else {
		activeLayout := activeComposeLayout(layout, deployment)
		args = composeArgsWithProjectName(activeLayout, activeComposeProjectName(activeLayout, result.Project, environment), action)
		result.Command = append([]string{"docker"}, args...)
	}
	if _, err := l.runStep(ctx, strings.ToUpper(action[:1])+action[1:]+" project containers", "docker", args...); err != nil {
		return ControlResult{}, fmt.Errorf("could not %s project containers: %w", action, err)
	}
	status := "running"
	if action == "stop" {
		status = "stopped"
	}
	if err := store.SetDeploymentStatus(ctx, result.Project, environment, status, ""); err != nil {
		return ControlResult{}, err
	}
	return result, nil
}

func (l Lifecycle) resolvePorts(ctx context.Context, project manifest.Project, environment string, dryRun bool) (map[string]int, *state.Store, error) {
	if dryRun {
		if _, err := os.Stat(l.Paths.StateDB); os.IsNotExist(err) {
			return PreviewPorts(project, environment), nil, nil
		} else if err != nil {
			return nil, nil, fmt.Errorf("could not inspect state database: %w", err)
		}
		store, err := state.OpenReadOnly(ctx, l.Paths.StateDB)
		if err != nil {
			return nil, nil, err
		}
		ports, err := PersistentPorts(ctx, store, project, environment, true)
		if err != nil {
			store.Close()
			return nil, nil, err
		}
		return ports, store, nil
	}
	store, err := state.Open(ctx, l.Paths.StateDB)
	if err != nil {
		return nil, nil, err
	}
	ports, err := PersistentPorts(ctx, store, project, environment, false)
	if err != nil {
		store.Close()
		return nil, nil, err
	}
	return ports, store, nil
}

func (l Lifecycle) checkPrerequisites(needsCaddy bool) error {
	if _, err := l.Runner.LookPath("docker"); err != nil {
		return fmt.Errorf("Docker is required for project deployment")
	}
	if !needsCaddy {
		return nil
	}
	if _, err := l.Runner.LookPath("caddy"); err != nil {
		return fmt.Errorf("Caddy is required for gateway deployment")
	}
	serviceCommand := l.serviceManager().VersionCommand()
	if _, err := l.Runner.LookPath(serviceCommand.Name); err != nil {
		return fmt.Errorf("%s is required for gateway deployment", serviceCommand.Name)
	}
	if info, err := os.Stat(l.Paths.CaddyFile); err != nil {
		return fmt.Errorf("could not access Caddyfile %s: %w", l.Paths.CaddyFile, err)
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("Caddyfile is not a regular file: %s", l.Paths.CaddyFile)
	}
	return nil
}

func (l Lifecycle) applyCaddy(ctx context.Context, layout DeploymentLayout, content []byte) error {
	snippetSnapshot, err := captureFile(layout.Caddy)
	if err != nil {
		return fmt.Errorf("could not capture existing Caddy project artifact: %w", err)
	}
	baseSnapshot, err := captureFile(l.Paths.CaddyFile)
	if err != nil {
		return fmt.Errorf("could not capture Caddyfile: %w", err)
	}
	if err := persistPrevious(layout.PreviousCaddy, snippetSnapshot); err != nil {
		return fmt.Errorf("could not preserve previous Caddy artifact: %w", err)
	}

	if len(content) == 0 {
		if err := os.Remove(layout.Caddy); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not remove obsolete Caddy project artifact: %w", err)
		}
	} else if err := WriteArtifact(layout.Caddy, content, caddyArtifactMode); err != nil {
		return fmt.Errorf("could not write Caddy project artifact: %w", err)
	}
	if err := ensureCaddyImport(l.Paths.CaddyFile, l.Paths.CaddyProjects); err != nil {
		_ = restoreFile(layout.Caddy, snippetSnapshot)
		return err
	}
	if _, err := l.runStep(ctx, "Validate Caddy configuration", "caddy", "validate", "--config", l.Paths.CaddyFile, "--adapter", "caddyfile"); err != nil {
		restoreErr := errors.Join(restoreFile(layout.Caddy, snippetSnapshot), restoreCaddyFile(l.Paths.CaddyFile, baseSnapshot))
		return errors.Join(fmt.Errorf("generated Caddy configuration is invalid: %w", err), restoreErr)
	}
	reload := l.serviceManager().ReloadCommand("caddy")
	if _, err := l.runStep(ctx, "Reload Caddy", reload.Name, reload.Args...); err != nil {
		restoreErr := errors.Join(restoreFile(layout.Caddy, snippetSnapshot), restoreCaddyFile(l.Paths.CaddyFile, baseSnapshot))
		if restoreErr == nil {
			_, restoreErr = l.runStep(ctx, "Restore Caddy", reload.Name, reload.Args...)
		}
		return errors.Join(fmt.Errorf("could not reload Caddy: %w", err), restoreErr)
	}
	return nil
}

func (l Lifecycle) rollbackCompose(ctx context.Context, layout DeploymentLayout, projectName, environment string, snapshot fileSnapshot) error {
	if snapshot.Exists {
		if err := restoreFile(layout.Compose, snapshot); err != nil {
			return fmt.Errorf("could not restore previous Compose artifact: %w", err)
		}
		if _, err := l.runStep(ctx, "Restore previous project containers", "docker", composeArgs(layout, projectName, environment,
			"up", "--detach", "--remove-orphans", "--wait", "--wait-timeout", fmt.Sprint(defaultDeployWaitSeconds))...); err != nil {
			return fmt.Errorf("could not restore previous project containers: %w", err)
		}
		return nil
	}
	_, downErr := l.runStep(ctx, "Remove project containers after failed deployment", "docker", composeArgs(layout, projectName, environment, "down", "--remove-orphans")...)
	restoreErr := restoreFile(layout.Compose, snapshot)
	if downErr != nil {
		downErr = fmt.Errorf("could not remove failed project containers: %w", downErr)
	}
	return errors.Join(downErr, restoreErr)
}

func isBlueGreenEligible(project manifest.Project, runner host.Runner) bool {
	dynamic, ok := runner.(host.DynamicPortRunner)
	if !ok || !dynamic.SupportsDynamicPorts() {
		return false
	}
	if len(project.Gateway.Routes) == 0 || len(project.Dependencies) > 0 {
		return false
	}
	for _, service := range project.Services {
		if len(service.Volumes) > 0 {
			return false
		}
	}
	return true
}

func slotLayout(layout DeploymentLayout, slot string) DeploymentLayout {
	root := filepath.Join(layout.Root, "slots", slot)
	layout.Root = root
	layout.Compose = filepath.Join(root, "compose.yaml")
	layout.PreviousCompose = filepath.Join(root, "compose.previous.yaml")
	return layout
}

func activeComposeLayout(layout DeploymentLayout, deployment state.Deployment) DeploymentLayout {
	if deployment.ComposePath == "" {
		return layout
	}
	candidate := filepath.Clean(deployment.ComposePath)
	baseRoot := filepath.Clean(layout.Root)
	for _, slot := range []string{"a", "b"} {
		if candidate == filepath.Join(baseRoot, "slots", slot, "compose.yaml") {
			return slotLayout(layout, slot)
		}
	}
	if candidate == filepath.Clean(layout.Compose) {
		return layout
	}
	// Keep compatibility with deployments created by older versions while
	// ensuring commands operate on the recorded active artifact.
	layout.Compose = candidate
	layout.PreviousCompose = candidate + ".previous"
	return layout
}

func activeComposeProjectName(layout DeploymentLayout, projectName, environment string) string {
	name := ComposeProjectName(projectName, environment)
	base := filepath.Base(filepath.Clean(layout.Root))
	if base == "a" || base == "b" {
		return name + "-slot-" + base
	}
	return name
}

func deploymentSlot(layout DeploymentLayout) string {
	base := filepath.Base(filepath.Clean(layout.Root))
	if base == "a" || base == "b" {
		return base
	}
	return ""
}

func composeArgsWithProjectName(layout DeploymentLayout, composeProject string, action ...string) []string {
	args := []string{"compose", "--project-name", composeProject, "--file", layout.Compose}
	return append(args, action...)
}

func clonePorts(ports map[string]int) map[string]int {
	result := make(map[string]int, len(ports))
	for key, port := range ports {
		result[key] = port
	}
	return result
}

func parsePublishedPort(output string) (int, error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, port, err := net.SplitHostPort(line); err == nil {
			value, parseErr := strconv.Atoi(port)
			if parseErr == nil && value > 0 && value <= 65535 {
				return value, nil
			}
		}
		if index := strings.LastIndex(line, ":"); index >= 0 {
			value, err := strconv.Atoi(strings.TrimSpace(line[index+1:]))
			if err == nil && value > 0 && value <= 65535 {
				return value, nil
			}
		}
	}
	return 0, fmt.Errorf("could not determine the published port from Docker output %q", strings.TrimSpace(output))
}

func (l Lifecycle) resolveDynamicPorts(ctx context.Context, layout DeploymentLayout, composeProject string, routes []manifest.Route) (map[string]int, error) {
	ports := make(map[string]int)
	for _, route := range routes {
		key := PortKey(route.Service, route.Port)
		if _, exists := ports[key]; exists {
			continue
		}
		output, err := l.runStep(ctx, "Resolve gateway port for "+route.Service, "docker",
			composeArgsWithProjectName(layout, composeProject, "port", route.Service, strconv.Itoa(route.Port))...)
		if err != nil {
			return nil, fmt.Errorf("could not resolve published port for %s:%d: %w", route.Service, route.Port, err)
		}
		port, err := parsePublishedPort(output)
		if err != nil {
			return nil, fmt.Errorf("could not resolve published port for %s:%d: %w", route.Service, route.Port, err)
		}
		ports[key] = port
	}
	return ports, nil
}

func (l Lifecycle) cleanupBlueGreenSlot(ctx context.Context, layout DeploymentLayout, composeProject string, snapshot fileSnapshot) error {
	_, downErr := l.runStep(ctx, "Remove failed replacement containers", "docker",
		composeArgsWithProjectName(layout, composeProject, "down", "--remove-orphans")...)
	restoreErr := restoreFile(layout.Compose, snapshot)
	if downErr != nil {
		downErr = fmt.Errorf("could not remove failed replacement containers: %w", downErr)
	}
	return errors.Join(downErr, restoreErr)
}

func (l Lifecycle) deployBlueGreen(ctx context.Context, loaded manifest.LoadedProject, layout DeploymentLayout, result DeployResult, current state.Deployment, store *state.Store) (DeployResult, error) {
	activeLayout := activeComposeLayout(layout, current)
	activeProject := activeComposeProjectName(activeLayout, loaded.Project.Name, loaded.Environment)
	targetSlot := "a"
	if filepath.Base(filepath.Clean(activeLayout.Root)) == "a" {
		targetSlot = "b"
	} else if filepath.Base(filepath.Clean(activeLayout.Root)) == "b" {
		targetSlot = "a"
	}
	targetLayout := slotLayout(layout, targetSlot)
	if err := prepareDeploymentDirectories(targetLayout, loaded.Project, true); err != nil {
		return DeployResult{}, err
	}
	if previous, err := captureFile(activeLayout.Compose); err != nil {
		return DeployResult{}, fmt.Errorf("could not capture active Compose artifact: %w", err)
	} else if err := persistPrevious(layout.PreviousCompose, previous); err != nil {
		return DeployResult{}, fmt.Errorf("could not preserve active Compose artifact: %w", err)
	}

	dynamicPorts := clonePorts(result.Ports)
	for key := range dynamicPorts {
		dynamicPorts[key] = 0
	}
	options := DefaultRenderOptions(loaded.Project, loaded.Environment)
	options.DeploymentRoot = filepath.ToSlash(targetLayout.Root)
	options.DataRoot = filepath.ToSlash(layout.Root)
	options.RuntimeSecretsRoot = filepath.ToSlash(layout.RuntimeSecrets)
	options.Ports = dynamicPorts
	replacement, err := Generate(loaded.Project, options)
	if err != nil {
		return DeployResult{}, err
	}
	snapshot, err := captureFile(targetLayout.Compose)
	if err != nil {
		return DeployResult{}, fmt.Errorf("could not capture replacement Compose artifact: %w", err)
	}
	if err := WriteArtifact(targetLayout.Compose, replacement.Compose, 0o640); err != nil {
		return DeployResult{}, fmt.Errorf("could not write replacement Compose artifact: %w", err)
	}
	replacementProject := ComposeProjectName(loaded.Project.Name, loaded.Environment) + "-slot-" + targetSlot
	composeArgs := func(action ...string) []string {
		return composeArgsWithProjectName(targetLayout, replacementProject, action...)
	}
	if _, err := l.runStep(ctx, "Validate replacement Compose configuration", "docker", composeArgs("config", "--quiet")...); err != nil {
		cleanupErr := l.cleanupBlueGreenSlot(ctx, targetLayout, replacementProject, snapshot)
		return DeployResult{}, errors.Join(fmt.Errorf("replacement Compose configuration is invalid: %w", err), cleanupErr)
	}
	if _, err := l.runStep(ctx, "Start and health-check replacement containers", "docker", composeArgs("up", "--detach", "--remove-orphans", "--wait", "--wait-timeout", fmt.Sprint(defaultDeployWaitSeconds))...); err != nil {
		cleanupErr := l.cleanupBlueGreenSlot(ctx, targetLayout, replacementProject, snapshot)
		return DeployResult{}, errors.Join(fmt.Errorf("replacement containers did not become healthy; automatic rollback kept the current deployment active: %w", err), cleanupErr)
	}
	actualPorts, err := l.resolveDynamicPorts(ctx, targetLayout, replacementProject, loaded.Project.Gateway.Routes)
	if err != nil {
		cleanupErr := l.cleanupBlueGreenSlot(ctx, targetLayout, replacementProject, snapshot)
		return DeployResult{}, errors.Join(fmt.Errorf("replacement gateway ports could not be resolved; automatic rollback kept the current deployment active: %w", err), cleanupErr)
	}
	// Persist the resolved port assignments in the slot artifact. This keeps
	// restart and explicit rollback consistent with the Caddy route that was
	// activated for this slot.
	options.Ports = actualPorts
	replacement, err = Generate(loaded.Project, options)
	if err != nil {
		cleanupErr := l.cleanupBlueGreenSlot(ctx, targetLayout, replacementProject, snapshot)
		return DeployResult{}, errors.Join(fmt.Errorf("could not finalize replacement Compose artifact: %w", err), cleanupErr)
	}
	if err := WriteArtifact(targetLayout.Compose, replacement.Compose, 0o640); err != nil {
		cleanupErr := l.cleanupBlueGreenSlot(ctx, targetLayout, replacementProject, snapshot)
		return DeployResult{}, errors.Join(fmt.Errorf("could not persist resolved replacement ports: %w", err), cleanupErr)
	}
	caddyOptions := options
	caddyOptions.Ports = actualPorts
	caddyContent, err := renderCaddy(loaded.Project, caddyOptions)
	if err != nil {
		cleanupErr := l.cleanupBlueGreenSlot(ctx, targetLayout, replacementProject, snapshot)
		return DeployResult{}, errors.Join(err, cleanupErr)
	}
	oldCaddy, err := captureFile(layout.Caddy)
	if err != nil {
		cleanupErr := l.cleanupBlueGreenSlot(ctx, targetLayout, replacementProject, snapshot)
		return DeployResult{}, errors.Join(fmt.Errorf("could not capture active Caddy artifact: %w", err), cleanupErr)
	}
	if err := l.applyCaddy(ctx, layout, caddyContent); err != nil {
		cleanupErr := l.cleanupBlueGreenSlot(ctx, targetLayout, replacementProject, snapshot)
		return DeployResult{}, errors.Join(fmt.Errorf("replacement gateway was not activated; automatic rollback kept the current deployment active: %w", err), cleanupErr)
	}
	result.Revision = artifactRevision(Artifacts{Compose: replacement.Compose, Caddy: caddyContent})
	deployment := state.Deployment{
		Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment), Status: "running",
		Revision: result.Revision, ManifestPath: loaded.Path, ComposePath: targetLayout.Compose, CaddyPath: caddyStatePath(layout, len(caddyContent) > 0),
	}
	if err := store.PutDeployment(ctx, deployment); err != nil {
		restoreErr := l.restoreCaddyAfterFailedSwitch(ctx, layout, oldCaddy)
		cleanupErr := l.cleanupBlueGreenSlot(ctx, targetLayout, replacementProject, snapshot)
		return DeployResult{}, errors.Join(fmt.Errorf("replacement is running but deployment state could not be stored: %w", err), restoreErr, cleanupErr)
	}
	if _, err := l.runStep(ctx, "Stop previous project containers", "docker", composeArgsWithProjectName(activeLayout, activeProject, "down", "--remove-orphans")...); err != nil {
		result.Layout = targetLayout
		result.Ports = actualPorts
		return result, fmt.Errorf("zero-downtime deployment succeeded, but previous containers could not be removed: %w", err)
	}
	result.Layout = targetLayout
	result.Ports = actualPorts
	for index := range result.Steps {
		result.Steps[index].Status = "completed"
	}
	return result, nil
}

func (l Lifecycle) restoreCaddyAfterFailedSwitch(ctx context.Context, layout DeploymentLayout, snapshot fileSnapshot) error {
	if err := restoreFile(layout.Caddy, snapshot); err != nil {
		return err
	}
	reload := l.serviceManager().ReloadCommand("caddy")
	if _, err := l.runStep(ctx, "Restore previous Caddy route", reload.Name, reload.Args...); err != nil {
		return fmt.Errorf("could not restore previous Caddy route: %w", err)
	}
	return nil
}

func composeArgs(layout DeploymentLayout, projectName, environment string, action ...string) []string {
	args := []string{"compose", "--project-name", ComposeProjectName(projectName, environment), "--file", layout.Compose}
	return append(args, action...)
}

func deploymentPlan(layout DeploymentLayout, project manifest.Project, environment string, needsCaddy bool, secrets []string, reload host.PackageCommand) []LifecycleStep {
	steps := []LifecycleStep{{Name: "allocate gateway ports", Status: "planned"}}
	steps = append(steps, LifecycleStep{Name: "prepare deployment directories", Path: layout.Root, Status: "planned"})
	for _, secret := range secrets {
		steps = append(steps, LifecycleStep{Name: "verify runtime secret", Path: filepath.Join(layout.RuntimeSecrets, secret), Status: "planned"})
	}
	steps = append(steps,
		LifecycleStep{Name: "write Compose artifact", Path: layout.Compose, Status: "planned"},
		LifecycleStep{Name: "validate Compose configuration", Command: append([]string{"docker"}, composeArgs(layout, project.Name, environment, "config", "--quiet")...), Status: "planned"},
		LifecycleStep{Name: "start and health-check containers", Command: append([]string{"docker"}, composeArgs(layout, project.Name, environment, "up", "--detach", "--remove-orphans", "--wait", "--wait-timeout", fmt.Sprint(defaultDeployWaitSeconds))...), Status: "planned"},
	)
	if needsCaddy {
		steps = append(steps,
			LifecycleStep{Name: "reconcile Caddy project artifact", Path: layout.Caddy, Status: "planned"},
			LifecycleStep{Name: "validate Caddy configuration", Status: "planned"},
			LifecycleStep{Name: "reload Caddy", Command: append([]string{reload.Name}, reload.Args...), Status: "planned"},
		)
	}
	steps = append(steps, LifecycleStep{Name: "store deployment state", Status: "planned"})
	return steps
}

func prepareDeploymentDirectories(layout DeploymentLayout, project manifest.Project, needsCaddy bool) error {
	directories := map[string]fs.FileMode{layout.Root: 0o750}
	if needsCaddy {
		// The Caddy service runs as an unprivileged user and must be able to
		// traverse this directory to read the base file and project snippets.
		directories[filepath.Dir(layout.Caddy)] = caddyDirectoryMode
	}
	for _, service := range project.Services {
		for _, volume := range service.Volumes {
			directories[filepath.Join(layout.Root, "data", volume.Name)] = 0o750
		}
	}
	for name, dependency := range project.Dependencies {
		if dependency.Type == "postgres" || (dependency.Type == "redis" && dependency.Persistence != "none") {
			directories[filepath.Join(layout.Root, "data", "dependencies", name)] = 0o750
		}
	}
	for directory, mode := range directories {
		if err := os.MkdirAll(directory, mode); err != nil {
			return fmt.Errorf("could not create deployment directory %s: %w", directory, err)
		}
		if err := os.Chmod(directory, mode); err != nil {
			return fmt.Errorf("could not secure deployment directory %s: %w", directory, err)
		}
	}
	return nil
}

func deploymentNeedsCaddy(layout DeploymentLayout, project manifest.Project) (bool, error) {
	if len(project.Gateway.Routes) > 0 {
		return true, nil
	}
	if _, err := os.Stat(layout.Caddy); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, fmt.Errorf("could not inspect Caddy project artifact: %w", err)
	}
}

func caddyStatePath(layout DeploymentLayout, needsCaddy bool) string {
	if !needsCaddy {
		return ""
	}
	return layout.Caddy
}

func requiredSecrets(project manifest.Project) []string {
	unique := make(map[string]bool)
	for _, service := range project.Services {
		for _, secret := range service.Secrets {
			unique[secret.Name] = true
		}
	}
	for _, dependency := range project.Dependencies {
		if dependency.PasswordSecret != "" {
			unique[dependency.PasswordSecret] = true
		}
	}
	secrets := make([]string, 0, len(unique))
	for secret := range unique {
		secrets = append(secrets, secret)
	}
	sort.Strings(secrets)
	return secrets
}

func requiredSecretSpecs(project manifest.Project) []secret.MaterializeSpec {
	specs := make(map[string]secret.MaterializeSpec)
	for _, service := range project.Services {
		for _, mount := range service.Secrets {
			mode, err := secret.ParseMode(mount.Mode)
			if err != nil {
				mode = 0o400
			}
			specs[mount.Name] = secret.MaterializeSpec{Name: mount.Name, Mode: mode, UID: mount.UID, GID: mount.GID}
		}
	}
	for _, dependency := range project.Dependencies {
		if dependency.PasswordSecret != "" {
			if _, exists := specs[dependency.PasswordSecret]; !exists {
				specs[dependency.PasswordSecret] = secret.MaterializeSpec{Name: dependency.PasswordSecret, Mode: 0o400}
			}
		}
	}
	names := make([]string, 0, len(specs))
	for name := range specs {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]secret.MaterializeSpec, 0, len(names))
	for _, name := range names {
		result = append(result, specs[name])
	}
	return result
}

func ensureRuntimeSecrets(root string, secrets []string) error {
	for _, secret := range secrets {
		path := filepath.Join(root, secret)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("runtime secret %s is not available at %s: %w", secret, path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("runtime secret is not a regular file: %s", path)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("runtime secret permissions are too broad at %s: expected no group or other access", path)
		}
	}
	return nil
}

func ensureCaddyImport(caddyFile, projectDirectory string) error {
	if err := ensureCaddyRuntimeAccess(caddyFile); err != nil {
		return err
	}
	content, err := os.ReadFile(caddyFile)
	if err != nil {
		return fmt.Errorf("could not read Caddyfile: %w", err)
	}
	pattern := "*.caddy"
	if filepath.Clean(projectDirectory) == filepath.Clean(filepath.Dir(caddyFile)) {
		pattern = "omurga-*.caddy"
	}
	importLine := "import " + filepath.ToSlash(filepath.Join(projectDirectory, pattern))
	lines := strings.Split(string(content), "\n")
	legacyImportLines := map[string]bool{
		"import /etc/omurga/caddy/projects/*.caddy": true,
		"import /etc/caddy/omurga/projects/*.caddy": true,
	}
	found := false
	removedLegacy := false
	updatedLines := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if legacyImportLines[trimmed] {
			removedLegacy = true
			continue
		}
		if trimmed == importLine {
			found = true
		}
		updatedLines = append(updatedLines, line)
	}
	if found && !removedLegacy {
		if len(updatedLines) == len(lines) {
			return nil
		}
	}
	updated := []byte(strings.Join(updatedLines, "\n"))
	if !found {
		if len(updated) > 0 && updated[len(updated)-1] != '\n' {
			updated = append(updated, '\n')
		}
		updated = append(updated, []byte("\n# Omurga managed project routes\n"+importLine+"\n")...)
	}
	if err := WriteArtifact(caddyFile, updated, caddyArtifactMode); err != nil {
		if removedLegacy {
			return fmt.Errorf("could not migrate the Omurga project import in Caddyfile: %w", err)
		}
		return fmt.Errorf("could not add the Omurga project import to Caddyfile: %w", err)
	}
	return nil
}

func ensureCaddyRuntimeAccess(caddyFile string) error {
	directory := filepath.Dir(caddyFile)
	if err := os.Chmod(directory, caddyDirectoryMode); err != nil {
		return fmt.Errorf("could not make Caddy configuration directory traversable at %s: %w", directory, err)
	}
	if err := os.Chmod(caddyFile, caddyArtifactMode); err != nil {
		return fmt.Errorf("could not make Caddyfile readable at %s: %w", caddyFile, err)
	}
	return nil
}

type fileSnapshot struct {
	Exists bool
	Data   []byte
	Mode   fs.FileMode
}

func captureFile(path string) (fileSnapshot, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fileSnapshot{}, nil
	}
	if err != nil {
		return fileSnapshot{}, err
	}
	if !info.Mode().IsRegular() {
		return fileSnapshot{}, fmt.Errorf("path is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	return fileSnapshot{Exists: true, Data: data, Mode: info.Mode().Perm()}, nil
}

func restoreFile(path string, snapshot fileSnapshot) error {
	if !snapshot.Exists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not remove newly created artifact %s: %w", path, err)
		}
		return nil
	}
	if err := WriteArtifact(path, snapshot.Data, snapshot.Mode); err != nil {
		return fmt.Errorf("could not restore artifact %s: %w", path, err)
	}
	return nil
}

func restoreCaddyFile(path string, snapshot fileSnapshot) error {
	if err := restoreFile(path, snapshot); err != nil {
		return err
	}
	if !snapshot.Exists {
		return nil
	}
	return ensureCaddyRuntimeAccess(path)
}

func persistPrevious(path string, snapshot fileSnapshot) error {
	if !snapshot.Exists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return WriteArtifact(path, snapshot.Data, snapshot.Mode)
}

func artifactRevision(artifacts Artifacts) string {
	hash := sha256.New()
	hash.Write(artifacts.Compose)
	hash.Write([]byte{0})
	hash.Write(artifacts.Caddy)
	return hex.EncodeToString(hash.Sum(nil))
}

func parseComposePS(output string) ([]map[string]any, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}
	var containers []map[string]any
	if err := json.Unmarshal([]byte(output), &containers); err == nil {
		return containers, nil
	}
	for _, line := range strings.Split(output, "\n") {
		var container map[string]any
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			return nil, fmt.Errorf("could not parse Docker Compose status output: %w", err)
		}
		containers = append(containers, container)
	}
	return containers, nil
}
