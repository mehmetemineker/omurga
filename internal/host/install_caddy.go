package host

import (
	"context"
)

func (i Installer) InstallCaddy(ctx context.Context, release OSRelease, options InstallOptions) (InstallResult, error) {
	result := InstallResult{Component: "caddy", DryRun: options.DryRun}
	if err := i.validate(); err != nil {
		return result, err
	}
	provider, err := i.providerFor(release)
	if err != nil {
		return result, err
	}
	packages := provider.PackageManager()
	services := provider.ServiceManager()
	spec, err := provider.CaddySpec(release)
	if err != nil {
		return result, err
	}
	active := services.IsActiveCommand("caddy")
	if packages.IsInstalled(ctx, i.Runner, "caddy") &&
		fileExists(i.Paths.CaddyKey) && fileExists(i.Paths.CaddySource) &&
		commandHealthy(ctx, i.Runner, "caddy", "version") &&
		commandHealthy(ctx, i.Runner, active.Name, active.Args...) {
		result.AlreadyInstalled = true
		result.Steps = append(result.Steps, InstallStep{Name: "verify Caddy service", Status: "unchanged"})
		return result, nil
	}

	if err := i.runCommand(ctx, &result, options, "refresh package indexes", packages.RefreshCommand()); err != nil {
		return result, err
	}
	if len(spec.Prerequisites) > 0 {
		if err := i.runCommand(ctx, &result, options, "install Caddy repository prerequisites", packages.InstallCommand(spec.Prerequisites...)); err != nil {
			return result, err
		}
	}
	if err := i.configureRepositories(ctx, &result, options, spec.Repository); err != nil {
		return result, err
	}
	if err := i.runCommand(ctx, &result, options, "refresh Caddy repository indexes", packages.RefreshCommand()); err != nil {
		return result, err
	}
	if err := i.runCommand(ctx, &result, options, "install Caddy", packages.InstallCommand(spec.Packages...)); err != nil {
		return result, err
	}
	if err := i.runCommand(ctx, &result, options, "enable and start Caddy", services.EnableNowCommand("caddy")); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "verify Caddy", "caddy", "version"); err != nil {
		return result, err
	}
	if err := i.runCommand(ctx, &result, options, "verify Caddy service", services.IsActiveCommand("caddy")); err != nil {
		return result, err
	}
	return result, nil
}
