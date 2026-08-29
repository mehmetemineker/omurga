package host

import "context"

// InstallRestic installs the backup engine used by Omurga backup commands.
func (i Installer) InstallRestic(ctx context.Context, release OSRelease, options InstallOptions) (InstallResult, error) {
	result := InstallResult{Component: "restic", DryRun: options.DryRun}
	if err := i.validate(); err != nil {
		return result, err
	}
	provider, err := i.providerFor(release)
	if err != nil {
		return result, err
	}
	packages := provider.PackageManager()
	if packages.IsInstalled(ctx, i.Runner, "restic") && commandHealthy(ctx, i.Runner, "restic", "version") {
		result.AlreadyInstalled = true
		result.Steps = append(result.Steps, InstallStep{Name: "verify Restic", Status: "unchanged"})
		return result, nil
	}
	if err := i.runCommand(ctx, &result, options, "refresh package indexes", packages.RefreshCommand()); err != nil {
		return result, err
	}
	if err := i.runCommand(ctx, &result, options, "install Restic", packages.InstallCommand("restic")); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "verify Restic", "restic", "version"); err != nil {
		return result, err
	}
	return result, nil
}
