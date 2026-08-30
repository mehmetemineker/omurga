package host

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const aptPeriodicConfig = `# Managed by Omurga. Do not edit.
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
`

const unattendedUpgradesConfig = `# Managed by Omurga. Do not edit.
# Security origins remain controlled by the distribution defaults.
Unattended-Upgrade::Automatic-Reboot "false";
Unattended-Upgrade::Remove-Unused-Dependencies "true";
Unattended-Upgrade::Package-Blacklist {
    "docker-ce";
    "docker-ce-cli";
    "containerd.io";
    "docker-buildx-plugin";
    "docker-compose-plugin";
    "caddy";
};
`

// InstallUnattendedUpgrades installs daily unattended APT updates using the
// distribution's configured security origins. Docker and Caddy are excluded
// so their service updates remain controlled by Omurga.
func (i Installer) InstallUnattendedUpgrades(ctx context.Context, release OSRelease, options InstallOptions) (InstallResult, error) {
	result := InstallResult{Component: "unattended-upgrades", DryRun: options.DryRun}
	if err := i.validate(); err != nil {
		return result, err
	}
	provider, err := i.providerFor(release)
	if err != nil {
		return result, err
	}
	packages := provider.PackageManager()
	services := provider.ServiceManager()
	configured, err := unattendedUpgradesConfigured(i.Paths)
	if err != nil {
		return result, err
	}
	updatesTimer := services.IsActiveCommand("apt-daily.timer")
	upgradeTimer := services.IsActiveCommand("apt-daily-upgrade.timer")
	healthy := packages.IsInstalled(ctx, i.Runner, "unattended-upgrades") && configured &&
		commandHealthy(ctx, i.Runner, updatesTimer.Name, updatesTimer.Args...) &&
		commandHealthy(ctx, i.Runner, upgradeTimer.Name, upgradeTimer.Args...)
	if healthy {
		result.AlreadyInstalled = true
		result.Steps = append(result.Steps, InstallStep{Name: "verify automatic security updates", Status: "unchanged"})
		return result, nil
	}

	if !packages.IsInstalled(ctx, i.Runner, "unattended-upgrades") {
		if err := i.runCommand(ctx, &result, options, "refresh package indexes", packages.RefreshCommand()); err != nil {
			return result, err
		}
		if err := i.runCommand(ctx, &result, options, "install unattended-upgrades", packages.InstallCommand("unattended-upgrades")); err != nil {
			return result, err
		}
	}

	changed, err := ensureFile(i.Paths.APTPeriodicConfig, []byte(aptPeriodicConfig), 0o644, options.DryRun)
	if err != nil {
		return result, fmt.Errorf("could not configure APT periodic updates: %w", err)
	}
	i.fileStep(&result, options, "configure APT periodic updates", i.Paths.APTPeriodicConfig, changed)
	changed, err = ensureFile(i.Paths.UnattendedUpgrades, []byte(unattendedUpgradesConfig), 0o644, options.DryRun)
	if err != nil {
		return result, fmt.Errorf("could not configure unattended-upgrades: %w", err)
	}
	i.fileStep(&result, options, "configure unattended-upgrades policy", i.Paths.UnattendedUpgrades, changed)

	if err := i.runCommand(ctx, &result, options, "enable APT package list updates", services.EnableNowCommand("apt-daily.timer")); err != nil {
		return result, err
	}
	if err := i.runCommand(ctx, &result, options, "enable automatic security updates", services.EnableNowCommand("apt-daily-upgrade.timer")); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "verify APT package list updates", updatesTimer.Name, updatesTimer.Args...); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "verify automatic security updates", upgradeTimer.Name, upgradeTimer.Args...); err != nil {
		return result, err
	}
	return result, nil
}

func unattendedUpgradesConfigured(paths Paths) (bool, error) {
	periodic, err := os.ReadFile(paths.APTPeriodicConfig)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	policy, err := os.ReadFile(paths.UnattendedUpgrades)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(string(periodic), `APT::Periodic::Update-Package-Lists "1";`) &&
		strings.Contains(string(periodic), `APT::Periodic::Unattended-Upgrade "1";`) &&
		strings.Contains(string(policy), `Unattended-Upgrade::Automatic-Reboot "false";`) &&
		strings.Contains(string(policy), `"docker-ce";`) &&
		strings.Contains(string(policy), `"caddy";`), nil
}
