package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"omurga/internal/gateway"
	"omurga/internal/host"
	"omurga/internal/manifest"
	"omurga/internal/state"
)

type LogOptions struct {
	Follow     bool
	Tail       string
	Since      string
	Timestamps bool
	Services   []string
}

type LogResult struct {
	Project     string   `json:"project"`
	Environment string   `json:"environment"`
	DryRun      bool     `json:"dryRun"`
	Command     []string `json:"command"`
}

type ExecOptions struct {
	TTY     bool
	Command []string
}

type ExecResult struct {
	Project     string   `json:"project"`
	Environment string   `json:"environment"`
	Service     string   `json:"service"`
	DryRun      bool     `json:"dryRun"`
	Command     []string `json:"command"`
}

type RollbackResult struct {
	Project      string          `json:"project"`
	Environment  string          `json:"environment"`
	FromRevision string          `json:"fromRevision"`
	ToRevision   string          `json:"toRevision"`
	DryRun       bool            `json:"dryRun"`
	Steps        []LifecycleStep `json:"steps"`
}

type DeleteOptions struct {
	DryRun    bool
	PurgeData bool
}

type DeleteResult struct {
	Project       string                    `json:"project"`
	Environment   string                    `json:"environment"`
	DryRun        bool                      `json:"dryRun"`
	DataPreserved bool                      `json:"dataPreserved"`
	DataPath      string                    `json:"dataPath"`
	State         state.DeleteProjectResult `json:"state"`
	Steps         []LifecycleStep           `json:"steps"`
}

func (l Lifecycle) Logs(ctx context.Context, loaded manifest.LoadedProject, options LogOptions, dryRun bool, stdout, stderr io.Writer) (LogResult, error) {
	environment := gateway.EnvironmentKey(loaded.Environment)
	layout := l.Layout(loaded.Project.Name, environment)
	if err := validateLogOptions(loaded.Project, options); err != nil {
		return LogResult{}, err
	}
	args := composeArgs(layout, loaded.Project.Name, environment, "logs")
	if options.Follow {
		args = append(args, "--follow")
	}
	if options.Tail != "" {
		args = append(args, "--tail", options.Tail)
	}
	if options.Since != "" {
		args = append(args, "--since", options.Since)
	}
	if options.Timestamps {
		args = append(args, "--timestamps")
	}
	args = append(args, options.Services...)
	result := LogResult{
		Project: loaded.Project.Name, Environment: environment, DryRun: dryRun,
		Command: append([]string{"docker"}, args...),
	}
	if dryRun {
		return result, nil
	}
	deployment, err := l.deployment(ctx, loaded.Project.Name, environment, true)
	if err != nil {
		return LogResult{}, err
	}
	activeLayout := activeComposeLayout(layout, deployment)
	args = composeArgsWithProjectName(activeLayout, activeComposeProjectName(activeLayout, loaded.Project.Name, environment), "logs")
	if options.Follow {
		args = append(args, "--follow")
	}
	if options.Tail != "" {
		args = append(args, "--tail", options.Tail)
	}
	if options.Since != "" {
		args = append(args, "--since", options.Since)
	}
	if options.Timestamps {
		args = append(args, "--timestamps")
	}
	args = append(args, options.Services...)
	result.Command = append([]string{"docker"}, args...)
	streamer, ok := l.Runner.(host.StreamRunner)
	if !ok {
		return LogResult{}, fmt.Errorf("project log streaming is not supported by the command runner")
	}
	if err := streamer.Stream(ctx, stdout, stderr, "docker", args...); err != nil {
		return LogResult{}, fmt.Errorf("could not stream project logs: %w", err)
	}
	return result, nil
}

func (l Lifecycle) CurrentDeployment(ctx context.Context, loaded manifest.LoadedProject) (state.Deployment, bool, error) {
	environment := gateway.EnvironmentKey(loaded.Environment)
	if _, err := os.Stat(l.Paths.StateDB); os.IsNotExist(err) {
		return state.Deployment{}, false, nil
	} else if err != nil {
		return state.Deployment{}, false, fmt.Errorf("could not inspect state database: %w", err)
	}
	store, err := state.OpenReadOnly(ctx, l.Paths.StateDB)
	if err != nil {
		return state.Deployment{}, false, err
	}
	defer store.Close()
	deployment, exists, err := store.GetDeployment(ctx, loaded.Project.Name, environment)
	if err != nil {
		return state.Deployment{}, false, err
	}
	return deployment, exists, nil
}

func (l Lifecycle) CurrentLayout(ctx context.Context, loaded manifest.LoadedProject) (DeploymentLayout, state.Deployment, bool, error) {
	layout := l.Layout(loaded.Project.Name, loaded.Environment)
	deployment, exists, err := l.CurrentDeployment(ctx, loaded)
	if err != nil {
		return DeploymentLayout{}, state.Deployment{}, false, err
	}
	if exists {
		layout = activeComposeLayout(layout, deployment)
	}
	return layout, deployment, exists, nil
}

func (l Lifecycle) Exec(ctx context.Context, loaded manifest.LoadedProject, service string, options ExecOptions, dryRun bool, stdin io.Reader, stdout, stderr io.Writer) (ExecResult, error) {
	environment := gateway.EnvironmentKey(loaded.Environment)
	if service == "" {
		return ExecResult{}, fmt.Errorf("project service is required")
	}
	if _, exists := loaded.Project.Services[service]; !exists {
		return ExecResult{}, fmt.Errorf("service %q is not declared in project %s", service, loaded.Project.Name)
	}
	if len(options.Command) == 0 {
		return ExecResult{}, fmt.Errorf("a command is required for project exec")
	}

	layout := l.Layout(loaded.Project.Name, environment)
	args := composeArgs(layout, loaded.Project.Name, environment, "exec")
	if options.TTY {
		args = append(args, "--interactive", "--tty")
	} else {
		args = append(args, "--no-TTY")
	}
	args = append(args, service)
	args = append(args, options.Command...)
	result := ExecResult{
		Project: loaded.Project.Name, Environment: environment, Service: service,
		DryRun: dryRun, Command: append([]string{"docker"}, args...),
	}
	if dryRun {
		return result, nil
	}

	deployment, err := l.deployment(ctx, loaded.Project.Name, environment, true)
	if err != nil {
		return ExecResult{}, err
	}
	activeLayout := activeComposeLayout(layout, deployment)
	args = composeArgsWithProjectName(activeLayout, activeComposeProjectName(activeLayout, loaded.Project.Name, environment), "exec")
	if options.TTY {
		args = append(args, "--interactive", "--tty")
	} else {
		args = append(args, "--no-TTY")
	}
	args = append(args, service)
	args = append(args, options.Command...)
	result.Command = append([]string{"docker"}, args...)
	runner, ok := l.Runner.(host.IORunner)
	if !ok {
		return ExecResult{}, fmt.Errorf("project exec is not supported by the command runner")
	}
	if err := runner.RunIO(ctx, stdin, stdout, stderr, "docker", args...); err != nil {
		return ExecResult{}, fmt.Errorf("could not execute command in project service: %w", err)
	}
	return result, nil
}

func (l Lifecycle) Rollback(ctx context.Context, loaded manifest.LoadedProject, dryRun bool) (RollbackResult, error) {
	environment := gateway.EnvironmentKey(loaded.Environment)
	layout := l.Layout(loaded.Project.Name, environment)
	deployment, err := l.deployment(ctx, loaded.Project.Name, environment, true)
	if err != nil {
		return RollbackResult{}, err
	}
	activeLayout := activeComposeLayout(layout, deployment)
	activeProject := activeComposeProjectName(activeLayout, loaded.Project.Name, environment)
	if activeSlot := deploymentSlot(activeLayout); activeSlot != "" {
		previousSlot := "a"
		if activeSlot == "a" {
			previousSlot = "b"
		}
		previousLayout := slotLayout(layout, previousSlot)
		if previousExists, existsErr := fileExistsAt(previousLayout.Compose); existsErr != nil {
			return RollbackResult{}, existsErr
		} else if previousExists {
			return l.rollbackBlueGreen(ctx, loaded, layout, activeLayout, previousLayout, activeProject, deployment, dryRun)
		}
	}
	currentCompose, err := captureFile(activeLayout.Compose)
	if err != nil || !currentCompose.Exists {
		return RollbackResult{}, fmt.Errorf("current Compose artifact is not available at %s", activeLayout.Compose)
	}
	previousCompose, err := captureFile(layout.PreviousCompose)
	if err != nil {
		return RollbackResult{}, fmt.Errorf("could not read previous Compose artifact: %w", err)
	}
	if !previousCompose.Exists {
		return RollbackResult{}, fmt.Errorf("no previous deployment is available for %s/%s", loaded.Project.Name, environment)
	}
	currentCaddy, err := captureFile(layout.Caddy)
	if err != nil {
		return RollbackResult{}, fmt.Errorf("could not read current Caddy artifact: %w", err)
	}
	previousCaddy, err := captureFile(layout.PreviousCaddy)
	if err != nil {
		return RollbackResult{}, fmt.Errorf("could not read previous Caddy artifact: %w", err)
	}
	targetRevision := artifactRevision(Artifacts{Compose: previousCompose.Data, Caddy: previousCaddy.Data})
	needsCaddy := currentCaddy.Exists || previousCaddy.Exists
	result := RollbackResult{
		Project: loaded.Project.Name, Environment: environment,
		FromRevision: deployment.Revision, ToRevision: targetRevision, DryRun: dryRun,
		Steps: rollbackPlan(layout, loaded.Project.Name, environment, needsCaddy, l.serviceManager().ReloadCommand("caddy")),
	}
	if dryRun {
		return result, nil
	}
	if err := l.checkPrerequisites(needsCaddy); err != nil {
		return RollbackResult{}, err
	}
	if err := persistPrevious(layout.PreviousCompose, currentCompose); err != nil {
		return RollbackResult{}, fmt.Errorf("could not preserve current Compose artifact before rollback: %w", err)
	}
	if err := restoreFile(activeLayout.Compose, previousCompose); err != nil {
		_ = restoreFile(layout.PreviousCompose, previousCompose)
		return RollbackResult{}, fmt.Errorf("could not activate previous Compose artifact: %w", err)
	}
	if _, err := l.runStep(ctx, "Validate previous Compose configuration", "docker", composeArgsWithProjectName(activeLayout, activeProject, "config", "--quiet")...); err != nil {
		restoreErr := errors.Join(restoreFile(activeLayout.Compose, currentCompose), restoreFile(layout.PreviousCompose, previousCompose))
		return RollbackResult{}, errors.Join(fmt.Errorf("previous Compose configuration is invalid: %w", err), restoreErr)
	}
	if _, err := l.runStep(ctx, "Start and health-check previous containers", "docker", composeArgsWithProjectName(activeLayout, activeProject,
		"up", "--detach", "--remove-orphans", "--wait", "--wait-timeout", fmt.Sprint(defaultDeployWaitSeconds))...); err != nil {
		restoreErr := errors.Join(l.restoreRollbackCompose(ctx, activeLayout, activeProject, currentCompose), restoreFile(layout.PreviousCompose, previousCompose))
		return RollbackResult{}, errors.Join(fmt.Errorf("previous project containers did not become healthy: %w", err), restoreErr)
	}
	if needsCaddy {
		if err := l.applyCaddy(ctx, layout, previousCaddy.Data); err != nil {
			restoreErr := errors.Join(
				l.restoreRollbackCompose(ctx, activeLayout, activeProject, currentCompose),
				restoreFile(layout.PreviousCompose, previousCompose),
				restoreFile(layout.PreviousCaddy, previousCaddy),
			)
			return RollbackResult{}, errors.Join(err, restoreErr)
		}
	}
	deployment.Status = "running"
	deployment.Revision = targetRevision
	deployment.ComposePath = activeLayout.Compose
	deployment.CaddyPath = caddyStatePath(layout, previousCaddy.Exists)
	deployment.LastError = ""
	store, err := state.Open(ctx, l.Paths.StateDB)
	if err != nil {
		return RollbackResult{}, fmt.Errorf("rollback completed but state database could not be opened: %w", err)
	}
	defer store.Close()
	if err := store.PutDeployment(ctx, deployment); err != nil {
		return RollbackResult{}, fmt.Errorf("rollback completed but deployment state could not be stored: %w", err)
	}
	for index := range result.Steps {
		result.Steps[index].Status = "completed"
	}
	return result, nil
}

func (l Lifecycle) rollbackBlueGreen(ctx context.Context, loaded manifest.LoadedProject, layout, activeLayout, previousLayout DeploymentLayout, activeProject string, deployment state.Deployment, dryRun bool) (RollbackResult, error) {
	previousProject := activeComposeProjectName(previousLayout, loaded.Project.Name, loaded.Environment)
	currentCompose, err := captureFile(activeLayout.Compose)
	if err != nil || !currentCompose.Exists {
		return RollbackResult{}, fmt.Errorf("current Compose artifact is not available at %s", activeLayout.Compose)
	}
	previousCompose, err := captureFile(previousLayout.Compose)
	if err != nil || !previousCompose.Exists {
		return RollbackResult{}, fmt.Errorf("previous slot Compose artifact is not available at %s", previousLayout.Compose)
	}
	currentCaddy, err := captureFile(layout.Caddy)
	if err != nil {
		return RollbackResult{}, fmt.Errorf("could not read current Caddy artifact: %w", err)
	}
	previousCaddy, err := captureFile(layout.PreviousCaddy)
	if err != nil {
		return RollbackResult{}, fmt.Errorf("could not read previous Caddy artifact: %w", err)
	}
	if !previousCaddy.Exists {
		return RollbackResult{}, fmt.Errorf("previous Caddy artifact is not available for blue-green rollback")
	}
	targetRevision := artifactRevision(Artifacts{Compose: previousCompose.Data, Caddy: previousCaddy.Data})
	result := RollbackResult{
		Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment),
		FromRevision: deployment.Revision, ToRevision: targetRevision, DryRun: dryRun,
		Steps: rollbackPlan(previousLayout, loaded.Project.Name, loaded.Environment, true, l.serviceManager().ReloadCommand("caddy")),
	}
	if dryRun {
		return result, nil
	}
	if err := l.checkPrerequisites(true); err != nil {
		return RollbackResult{}, err
	}
	if _, err := l.runStep(ctx, "Validate previous slot Compose configuration", "docker", composeArgsWithProjectName(previousLayout, previousProject, "config", "--quiet")...); err != nil {
		return RollbackResult{}, fmt.Errorf("previous slot Compose configuration is invalid: %w", err)
	}
	if _, err := l.runStep(ctx, "Start and health-check previous slot containers", "docker", composeArgsWithProjectName(previousLayout, previousProject,
		"up", "--detach", "--remove-orphans", "--wait", "--wait-timeout", fmt.Sprint(defaultDeployWaitSeconds))...); err != nil {
		_, _ = l.runStep(ctx, "Remove failed rollback slot containers", "docker", composeArgsWithProjectName(previousLayout, previousProject, "down", "--remove-orphans")...)
		return RollbackResult{}, fmt.Errorf("previous slot containers did not become healthy: %w", err)
	}
	if err := l.applyCaddy(ctx, layout, previousCaddy.Data); err != nil {
		_, _ = l.runStep(ctx, "Remove failed rollback slot containers", "docker", composeArgsWithProjectName(previousLayout, previousProject, "down", "--remove-orphans")...)
		return RollbackResult{}, err
	}
	deployment.Status = "running"
	deployment.Revision = targetRevision
	deployment.ComposePath = previousLayout.Compose
	deployment.CaddyPath = caddyStatePath(layout, true)
	deployment.LastError = ""
	store, err := state.Open(ctx, l.Paths.StateDB)
	if err != nil {
		restoreErr := l.restoreCaddyAfterFailedSwitch(ctx, layout, currentCaddy)
		_, cleanupErr := l.runStep(ctx, "Remove failed rollback slot containers", "docker", composeArgsWithProjectName(previousLayout, previousProject, "down", "--remove-orphans")...)
		return RollbackResult{}, errors.Join(fmt.Errorf("rollback completed but state database could not be opened: %w", err), restoreErr, cleanupErr)
	}
	if err := store.PutDeployment(ctx, deployment); err != nil {
		_ = store.Close()
		restoreErr := l.restoreCaddyAfterFailedSwitch(ctx, layout, currentCaddy)
		_, cleanupErr := l.runStep(ctx, "Remove failed rollback slot containers", "docker", composeArgsWithProjectName(previousLayout, previousProject, "down", "--remove-orphans")...)
		return RollbackResult{}, errors.Join(fmt.Errorf("rollback completed but deployment state could not be stored: %w", err), restoreErr, cleanupErr)
	}
	if err := store.Close(); err != nil {
		return RollbackResult{}, fmt.Errorf("rollback completed but state database could not be closed: %w", err)
	}
	if _, err := l.runStep(ctx, "Stop current project containers", "docker", composeArgsWithProjectName(activeLayout, activeProject, "down", "--remove-orphans")...); err != nil {
		return result, fmt.Errorf("rollback succeeded, but current containers could not be removed: %w", err)
	}
	for index := range result.Steps {
		result.Steps[index].Status = "completed"
	}
	return result, nil
}

func (l Lifecycle) Delete(ctx context.Context, loaded manifest.LoadedProject, options DeleteOptions) (DeleteResult, error) {
	environment := gateway.EnvironmentKey(loaded.Environment)
	layout := l.Layout(loaded.Project.Name, environment)
	dataPath := filepath.Join(layout.Root, "data")
	caddyExists, err := fileExistsAt(layout.Caddy)
	if err != nil {
		return DeleteResult{}, err
	}
	composeExists, err := fileExistsAt(layout.Compose)
	if err != nil {
		return DeleteResult{}, err
	}
	dataExists, err := directoryExistsAt(dataPath)
	if err != nil {
		return DeleteResult{}, err
	}
	result := DeleteResult{
		Project: loaded.Project.Name, Environment: environment, DryRun: options.DryRun,
		DataPreserved: !options.PurgeData, DataPath: dataPath,
		Steps: deletePlan(layout, loaded.Project.Name, environment, composeExists, caddyExists, options.PurgeData),
	}
	if options.DryRun {
		return result, nil
	}
	deployment, err := l.deployment(ctx, loaded.Project.Name, environment, false)
	if err != nil {
		return DeleteResult{}, err
	}
	if deployment.Project == "" && !composeExists && !caddyExists && !(options.PurgeData && dataExists) {
		return DeleteResult{}, fmt.Errorf("project %s/%s is not deployed", loaded.Project.Name, environment)
	}
	activeLayout := activeComposeLayout(layout, deployment)
	if deployment.Project != "" {
		composeExists, err = fileExistsAt(activeLayout.Compose)
		if err != nil {
			return DeleteResult{}, err
		}
		caddyExists, err = fileExistsAt(activeLayout.Caddy)
		if err != nil {
			return DeleteResult{}, err
		}
		result.Steps = deletePlan(activeLayout, loaded.Project.Name, environment, composeExists, caddyExists, options.PurgeData)
	}
	if composeExists {
		if _, err := l.Runner.LookPath("docker"); err != nil {
			return DeleteResult{}, fmt.Errorf("Docker is required for project deletion")
		}
	}
	if caddyExists {
		if err := l.checkPrerequisites(true); err != nil {
			return DeleteResult{}, err
		}
	}
	if composeExists {
		if _, err := l.runStep(ctx, "Remove project containers", "docker", composeArgsWithProjectName(activeLayout, activeComposeProjectName(activeLayout, loaded.Project.Name, environment), "down", "--remove-orphans")...); err != nil {
			return DeleteResult{}, fmt.Errorf("could not remove project containers: %w", err)
		}
	}
	if caddyExists {
		if err := l.applyCaddy(ctx, layout, nil); err != nil {
			return DeleteResult{}, err
		}
	}
	for _, path := range []string{layout.Compose, layout.PreviousCompose, layout.Caddy, layout.PreviousCaddy} {
		if err := removeFileIfExists(path); err != nil {
			return DeleteResult{}, err
		}
	}
	if err := safeRemoveAll(l.Paths.RuntimeSecrets, layout.RuntimeSecrets); err != nil {
		return DeleteResult{}, fmt.Errorf("could not remove runtime secrets: %w", err)
	}
	if options.PurgeData {
		if err := safeRemoveAll(layout.Root, dataPath); err != nil {
			return DeleteResult{}, fmt.Errorf("could not purge project data: %w", err)
		}
	}
	if err := safeRemoveAll(layout.Root, filepath.Join(layout.Root, "slots")); err != nil {
		return DeleteResult{}, fmt.Errorf("could not remove deployment slots: %w", err)
	}
	if _, err := os.Stat(l.Paths.StateDB); err == nil {
		store, err := state.Open(ctx, l.Paths.StateDB)
		if err != nil {
			return DeleteResult{}, err
		}
		deletion, deleteErr := store.DeleteProject(ctx, loaded.Project.Name, environment)
		closeErr := store.Close()
		if deleteErr != nil {
			return DeleteResult{}, deleteErr
		}
		if closeErr != nil {
			return DeleteResult{}, closeErr
		}
		result.State = deletion
	} else if !os.IsNotExist(err) {
		return DeleteResult{}, fmt.Errorf("could not inspect state database: %w", err)
	}
	_ = os.Remove(layout.Root)
	for index := range result.Steps {
		result.Steps[index].Status = "completed"
	}
	return result, nil
}

func (l Lifecycle) deployment(ctx context.Context, project, environment string, required bool) (state.Deployment, error) {
	if _, err := os.Stat(l.Paths.StateDB); os.IsNotExist(err) {
		if required {
			return state.Deployment{}, fmt.Errorf("project %s/%s is not deployed", project, environment)
		}
		return state.Deployment{}, nil
	} else if err != nil {
		return state.Deployment{}, fmt.Errorf("could not inspect state database: %w", err)
	}
	store, err := state.OpenReadOnly(ctx, l.Paths.StateDB)
	if err != nil {
		return state.Deployment{}, err
	}
	defer store.Close()
	deployment, exists, err := store.GetDeployment(ctx, project, environment)
	if err != nil {
		return state.Deployment{}, err
	}
	if !exists && required {
		return state.Deployment{}, fmt.Errorf("project %s/%s is not deployed", project, environment)
	}
	return deployment, nil
}

func (l Lifecycle) restoreRollbackCompose(ctx context.Context, layout DeploymentLayout, composeProject string, current fileSnapshot) error {
	if err := restoreFile(layout.Compose, current); err != nil {
		return err
	}
	if _, err := l.runStep(ctx, "Restore current project containers", "docker", composeArgsWithProjectName(layout, composeProject,
		"up", "--detach", "--remove-orphans", "--wait", "--wait-timeout", fmt.Sprint(defaultDeployWaitSeconds))...); err != nil {
		return fmt.Errorf("could not restore current project containers: %w", err)
	}
	return nil
}

func validateLogOptions(project manifest.Project, options LogOptions) error {
	if options.Tail != "" && options.Tail != "all" {
		value, err := strconv.Atoi(options.Tail)
		if err != nil || value < 0 {
			return fmt.Errorf("log tail must be a non-negative integer or all")
		}
	}
	for _, service := range options.Services {
		if _, exists := project.Services[service]; exists {
			continue
		}
		if _, exists := project.Dependencies[service]; !exists {
			return fmt.Errorf("log service %q is not defined by the project", service)
		}
	}
	return nil
}

func rollbackPlan(layout DeploymentLayout, project, environment string, needsCaddy bool, reload host.PackageCommand) []LifecycleStep {
	composeProject := activeComposeProjectName(layout, project, environment)
	steps := []LifecycleStep{
		{Name: "activate previous Compose artifact", Path: layout.PreviousCompose, Status: "planned"},
		{Name: "validate previous Compose configuration", Command: append([]string{"docker"}, composeArgsWithProjectName(layout, composeProject, "config", "--quiet")...), Status: "planned"},
		{Name: "start and health-check previous containers", Command: append([]string{"docker"}, composeArgsWithProjectName(layout, composeProject, "up", "--detach", "--remove-orphans", "--wait", "--wait-timeout", fmt.Sprint(defaultDeployWaitSeconds))...), Status: "planned"},
	}
	if needsCaddy {
		steps = append(steps,
			LifecycleStep{Name: "activate previous Caddy artifact", Path: layout.PreviousCaddy, Status: "planned"},
			LifecycleStep{Name: "validate Caddy configuration", Status: "planned"},
			LifecycleStep{Name: "reload Caddy", Command: append([]string{reload.Name}, reload.Args...), Status: "planned"},
		)
	}
	return append(steps, LifecycleStep{Name: "store rollback state", Status: "planned"})
}

func deletePlan(layout DeploymentLayout, project, environment string, hasCompose, hasCaddy, purgeData bool) []LifecycleStep {
	steps := make([]LifecycleStep, 0)
	if hasCompose {
		composeProject := activeComposeProjectName(layout, project, environment)
		steps = append(steps, LifecycleStep{
			Name: "remove project containers", Command: append([]string{"docker"}, composeArgsWithProjectName(layout, composeProject, "down", "--remove-orphans")...), Status: "planned",
		})
	}
	if hasCaddy {
		steps = append(steps,
			LifecycleStep{Name: "remove Caddy project route", Path: layout.Caddy, Status: "planned"},
			LifecycleStep{Name: "validate and reload Caddy", Status: "planned"},
		)
	}
	steps = append(steps,
		LifecycleStep{Name: "remove generated artifacts", Path: layout.Root, Status: "planned"},
		LifecycleStep{Name: "remove runtime secrets", Path: layout.RuntimeSecrets, Status: "planned"},
	)
	if purgeData {
		steps = append(steps, LifecycleStep{Name: "purge persistent project data", Path: filepath.Join(layout.Root, "data"), Status: "planned"})
	} else {
		steps = append(steps, LifecycleStep{Name: "preserve persistent project data", Path: filepath.Join(layout.Root, "data"), Status: "planned"})
	}
	return append(steps, LifecycleStep{Name: "remove deployment state and release gateway ports", Status: "planned"})
}

func fileExistsAt(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("path is not a regular file: %s", path)
	}
	return true, nil
}

func directoryExistsAt(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not inspect %s: %w", path, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("path is not a directory: %s", path)
	}
	return true, nil
}

func removeFileIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not remove generated artifact %s: %w", path, err)
	}
	return nil
}

func safeRemoveAll(base, target string) error {
	basePath, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	targetPath, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(basePath, targetPath)
	if err != nil {
		return err
	}
	if relative == "." || relative == "" || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to remove path outside the managed directory: %s", targetPath)
	}
	return os.RemoveAll(targetPath)
}
