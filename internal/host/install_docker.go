package host

import (
	"context"
	"fmt"
	"strings"
)

var dockerConflictingPackages = []string{
	"docker.io",
	"docker-compose",
	"docker-compose-v2",
	"docker-doc",
	"docker-buildx",
	"podman-docker",
	"containerd",
	"runc",
}

var dockerPackages = []string{
	"docker-ce",
	"docker-ce-cli",
	"containerd.io",
	"docker-buildx-plugin",
	"docker-compose-plugin",
}

func (i Installer) InstallDocker(ctx context.Context, release OSRelease, options InstallOptions) (InstallResult, error) {
	result := InstallResult{Component: "docker", DryRun: options.DryRun}
	if err := i.validate(); err != nil {
		return result, err
	}
	if err := ValidateSupportedUbuntu(release); err != nil {
		return result, err
	}
	if release.Codename == "" {
		return result, fmt.Errorf("Ubuntu codename is required to configure the Docker repository")
	}

	if packageInstalled(ctx, i.Runner, "docker-ce") &&
		fileExists(i.Paths.DockerKey) && fileExists(i.Paths.DockerSource) &&
		commandHealthy(ctx, i.Runner, "docker", "info", "--format", "{{.ServerVersion}}") &&
		commandHealthy(ctx, i.Runner, "docker", "compose", "version", "--short") {
		result.AlreadyInstalled = true
		result.Steps = append(result.Steps, InstallStep{Name: "verify Docker Engine and Compose", Status: "unchanged"})
		return result, nil
	}

	conflicts := make([]string, 0)
	for _, name := range dockerConflictingPackages {
		if packageInstalled(ctx, i.Runner, name) {
			conflicts = append(conflicts, name)
		}
	}
	if len(conflicts) > 0 && !options.ReplaceDockerConflicts {
		return result, fmt.Errorf("conflicting Docker packages are installed: %s; rerun with --replace-conflicting-docker to remove them", strings.Join(conflicts, ", "))
	}

	if err := i.runStep(ctx, &result, options, "refresh APT package indexes", "apt-get", "update"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "install Docker repository prerequisites", "apt-get", "install", "-y", "ca-certificates", "curl"); err != nil {
		return result, err
	}
	if len(conflicts) > 0 {
		args := append([]string{"remove", "-y"}, conflicts...)
		if err := i.runStep(ctx, &result, options, "remove conflicting Docker packages", "apt-get", args...); err != nil {
			return result, err
		}
	}

	architecture, err := i.Runner.Run(ctx, "dpkg", "--print-architecture")
	if err != nil {
		return result, fmt.Errorf("could not determine the Debian architecture: %w", err)
	}
	architecture = strings.TrimSpace(architecture)
	if architecture == "" {
		return result, fmt.Errorf("dpkg returned an empty architecture")
	}

	directoryChanged, err := ensureDirectory(i.Paths.APTKeyrings, 0o755, options.DryRun)
	if err != nil {
		return result, fmt.Errorf("could not prepare APT keyring directory: %w", err)
	}
	i.fileStep(&result, options, "prepare APT keyring directory", i.Paths.APTKeyrings, directoryChanged)

	if options.DryRun {
		i.fileStep(&result, options, "install Docker repository signing key", i.Paths.DockerKey, true)
	} else {
		key, err := i.Downloader.Download(ctx, dockerKeyURL)
		if err != nil {
			return result, fmt.Errorf("could not download Docker repository key: %w", err)
		}
		changed, err := ensureFile(i.Paths.DockerKey, key, 0o644, false)
		if err != nil {
			return result, fmt.Errorf("could not install Docker repository key: %w", err)
		}
		i.fileStep(&result, options, "install Docker repository signing key", i.Paths.DockerKey, changed)
	}

	source := fmt.Sprintf("Types: deb\nURIs: https://download.docker.com/linux/ubuntu\nSuites: %s\nComponents: stable\nArchitectures: %s\nSigned-By: /etc/apt/keyrings/docker.asc\n", release.Codename, architecture)
	sourceChanged, err := ensureFile(i.Paths.DockerSource, []byte(source), 0o644, options.DryRun)
	if err != nil {
		return result, fmt.Errorf("could not install Docker APT source: %w", err)
	}
	i.fileStep(&result, options, "install Docker APT source", i.Paths.DockerSource, sourceChanged)

	if err := i.runStep(ctx, &result, options, "refresh Docker repository indexes", "apt-get", "update"); err != nil {
		return result, err
	}
	installArgs := append([]string{"install", "-y"}, dockerPackages...)
	if err := i.runStep(ctx, &result, options, "install Docker Engine and Compose", "apt-get", installArgs...); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "enable and start Docker", "systemctl", "enable", "--now", "docker"); err != nil {
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
