package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (i Installer) InstallCaddy(ctx context.Context, release OSRelease, options InstallOptions) (InstallResult, error) {
	result := InstallResult{Component: "caddy", DryRun: options.DryRun}
	if err := i.validate(); err != nil {
		return result, err
	}
	if err := ValidateSupportedUbuntu(release); err != nil {
		return result, err
	}

	if packageInstalled(ctx, i.Runner, "caddy") &&
		fileExists(i.Paths.CaddyKey) && fileExists(i.Paths.CaddySource) &&
		commandHealthy(ctx, i.Runner, "caddy", "version") &&
		commandHealthy(ctx, i.Runner, "systemctl", "is-active", "--quiet", "caddy") {
		result.AlreadyInstalled = true
		result.Steps = append(result.Steps, InstallStep{Name: "verify Caddy service", Status: "unchanged"})
		return result, nil
	}

	if err := i.runStep(ctx, &result, options, "refresh APT package indexes", "apt-get", "update"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "install Caddy repository prerequisites", "apt-get", "install", "-y", "debian-keyring", "debian-archive-keyring", "apt-transport-https", "curl", "gnupg"); err != nil {
		return result, err
	}

	keyDirectoryChanged, err := ensureDirectory(filepath.Dir(i.Paths.CaddyKey), 0o755, options.DryRun)
	if err != nil {
		return result, fmt.Errorf("could not prepare Caddy keyring directory: %w", err)
	}
	i.fileStep(&result, options, "prepare Caddy keyring directory", filepath.Dir(i.Paths.CaddyKey), keyDirectoryChanged)

	if options.DryRun {
		i.fileStep(&result, options, "install Caddy repository signing key", i.Paths.CaddyKey, true)
		i.fileStep(&result, options, "install Caddy APT source", i.Paths.CaddySource, true)
	} else {
		if err := i.installCaddyKey(ctx, &result, options); err != nil {
			return result, err
		}
		source, err := i.Downloader.Download(ctx, caddySourceURL)
		if err != nil {
			return result, fmt.Errorf("could not download Caddy APT source: %w", err)
		}
		if !strings.Contains(string(source), "dl.cloudsmith.io/public/caddy/stable") {
			return result, fmt.Errorf("downloaded Caddy APT source did not reference the stable repository")
		}
		changed, err := ensureFile(i.Paths.CaddySource, source, 0o644, false)
		if err != nil {
			return result, fmt.Errorf("could not install Caddy APT source: %w", err)
		}
		i.fileStep(&result, options, "install Caddy APT source", i.Paths.CaddySource, changed)
	}

	if err := i.runStep(ctx, &result, options, "refresh Caddy repository indexes", "apt-get", "update"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "install Caddy", "apt-get", "install", "-y", "caddy"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "enable and start Caddy", "systemctl", "enable", "--now", "caddy"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "verify Caddy", "caddy", "version"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "verify Caddy service", "systemctl", "is-active", "--quiet", "caddy"); err != nil {
		return result, err
	}
	return result, nil
}

func (i Installer) installCaddyKey(ctx context.Context, result *InstallResult, options InstallOptions) error {
	key, err := i.Downloader.Download(ctx, caddyKeyURL)
	if err != nil {
		return fmt.Errorf("could not download Caddy repository key: %w", err)
	}
	rawFile, err := os.CreateTemp("", "omurga-caddy-key-*.asc")
	if err != nil {
		return err
	}
	rawPath := rawFile.Name()
	defer os.Remove(rawPath)
	if _, err := rawFile.Write(key); err != nil {
		rawFile.Close()
		return err
	}
	if err := rawFile.Close(); err != nil {
		return err
	}

	binaryFile, err := os.CreateTemp("", "omurga-caddy-key-*.gpg")
	if err != nil {
		return err
	}
	binaryPath := binaryFile.Name()
	if err := binaryFile.Close(); err != nil {
		return err
	}
	defer os.Remove(binaryPath)

	if err := i.runStep(ctx, result, options, "dearmor Caddy repository key", "gpg", "--batch", "--yes", "--dearmor", "--output", binaryPath, rawPath); err != nil {
		return err
	}
	dearmored, err := os.ReadFile(binaryPath)
	if err != nil {
		return fmt.Errorf("could not read dearmored Caddy repository key: %w", err)
	}
	if len(dearmored) == 0 {
		return fmt.Errorf("gpg produced an empty Caddy repository key")
	}
	changed, err := ensureFile(i.Paths.CaddyKey, dearmored, 0o644, false)
	if err != nil {
		return fmt.Errorf("could not install Caddy repository key: %w", err)
	}
	i.fileStep(result, options, "install Caddy repository signing key", i.Paths.CaddyKey, changed)
	return nil
}
