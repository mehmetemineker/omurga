package host

import (
	"context"
	"fmt"
	"strings"
)

func (i Installer) InstallDocker(ctx context.Context, release OSRelease, options InstallOptions) (InstallResult, error) {
	result := InstallResult{Component: "docker", DryRun: options.DryRun}
	if err := i.validate(); err != nil {
		return result, err
	}
	provider, err := i.providerFor(release)
	if err != nil {
		return result, err
	}
	packages := provider.PackageManager()
	services := provider.ServiceManager()

	dockerRuntimeHealthy := packages.IsInstalled(ctx, i.Runner, "docker-ce") &&
		fileExists(i.Paths.DockerKey) && fileExists(i.Paths.DockerSource) &&
		commandHealthy(ctx, i.Runner, "docker", "info", "--format", "{{.ServerVersion}}") &&
		commandHealthy(ctx, i.Runner, "docker", "compose", "version", "--short")
	if dockerRuntimeHealthy {
		configured, err := dockerLogRotationConfigured(i.Paths.DockerDaemonConfig)
		if err != nil {
			return result, err
		}
		if configured {
			result.AlreadyInstalled = true
			result.Steps = append(result.Steps, InstallStep{Name: "verify Docker Engine, Compose, and log rotation", Status: "unchanged"})
			return result, nil
		}
		changed, err := ensureDockerLogRotation(i.Paths.DockerDaemonConfig, options.DryRun)
		if err != nil {
			return result, fmt.Errorf("could not configure Docker log rotation: %w", err)
		}
		i.fileStep(&result, options, "configure Docker log rotation", i.Paths.DockerDaemonConfig, changed)
		if err := i.runCommand(ctx, &result, options, "restart Docker to apply log rotation", services.RestartCommand("docker")); err != nil {
			return result, err
		}
		if err := i.runStep(ctx, &result, options, "verify Docker Engine", "docker", "info", "--format", "{{.ServerVersion}}"); err != nil {
			return result, err
		}
		if err := i.runStep(ctx, &result, options, "verify Docker Compose", "docker", "compose", "version", "--short"); err != nil {
			return result, err
		}
		return result, nil
	}

	architecture, err := packages.Architecture(ctx, i.Runner)
	if err != nil {
		return result, err
	}
	spec, err := provider.DockerSpec(release, architecture)
	if err != nil {
		return result, err
	}
	conflicts := make([]string, 0)
	for _, name := range spec.Conflicts {
		if packages.IsInstalled(ctx, i.Runner, name) {
			conflicts = append(conflicts, name)
		}
	}
	if len(conflicts) > 0 && !options.ReplaceDockerConflicts {
		return result, fmt.Errorf("conflicting Docker packages are installed: %s; rerun with --replace-conflicting-docker to remove them", strings.Join(conflicts, ", "))
	}

	if err := i.runCommand(ctx, &result, options, "refresh package indexes", packages.RefreshCommand()); err != nil {
		return result, err
	}
	if len(spec.Prerequisites) > 0 {
		if err := i.runCommand(ctx, &result, options, "install Docker repository prerequisites", packages.InstallCommand(spec.Prerequisites...)); err != nil {
			return result, err
		}
	}
	if len(conflicts) > 0 {
		if err := i.runCommand(ctx, &result, options, "remove conflicting Docker packages", packages.RemoveCommand(conflicts...)); err != nil {
			return result, err
		}
	}

	if err := i.configureRepositories(ctx, &result, options, spec.Repository); err != nil {
		return result, err
	}
	if err := i.runCommand(ctx, &result, options, "refresh Docker repository indexes", packages.RefreshCommand()); err != nil {
		return result, err
	}
	if err := i.runCommand(ctx, &result, options, "install Docker Engine and Compose", packages.InstallCommand(spec.Packages...)); err != nil {
		return result, err
	}
	changed, err := ensureDockerLogRotation(i.Paths.DockerDaemonConfig, options.DryRun)
	if err != nil {
		return result, fmt.Errorf("could not configure Docker log rotation: %w", err)
	}
	i.fileStep(&result, options, "configure Docker log rotation", i.Paths.DockerDaemonConfig, changed)
	if err := i.runCommand(ctx, &result, options, "enable and start Docker", services.EnableNowCommand("docker")); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "verify Docker Engine", "docker", "info", "--format", "{{.ServerVersion}}"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "verify Docker Compose", "docker", "compose", "version", "--short"); err != nil {
		return result, err
	}
	return result, nil
}
