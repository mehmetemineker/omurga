package host

import (
	"context"
	"fmt"
	"os"
	"runtime"
)

const fail2banSSHJail = `# Managed by Omurga. Do not edit.
[sshd]
enabled = true
backend = systemd
maxretry = 5
findtime = 10m
bantime = 1h
`

// InstallFail2ban installs Fail2ban and enables a conservative SSH jail.
func (i Installer) InstallFail2ban(ctx context.Context, release OSRelease, options InstallOptions) (InstallResult, error) {
	result := InstallResult{Component: "fail2ban", DryRun: options.DryRun}
	if err := i.validate(); err != nil {
		return result, err
	}
	provider, err := i.providerFor(release)
	if err != nil {
		return result, err
	}
	packages := provider.PackageManager()
	services := provider.ServiceManager()
	active := services.IsActiveCommand("fail2ban")
	configured := false
	if content, readErr := os.ReadFile(i.Paths.Fail2banJail); readErr == nil && string(content) == fail2banSSHJail {
		configured = true
		if runtime.GOOS != "windows" {
			if info, statErr := os.Stat(i.Paths.Fail2banJail); statErr != nil || info.Mode().Perm() != 0o644 {
				configured = false
			}
		}
	}
	healthy := packages.IsInstalled(ctx, i.Runner, "fail2ban") && configured &&
		commandHealthy(ctx, i.Runner, active.Name, active.Args...) &&
		commandHealthy(ctx, i.Runner, "fail2ban-client", "status", "sshd")
	if healthy {
		result.AlreadyInstalled = true
		result.Steps = append(result.Steps, InstallStep{Name: "verify Fail2ban SSH jail", Status: "unchanged"})
		return result, nil
	}

	if !packages.IsInstalled(ctx, i.Runner, "fail2ban") {
		if err := i.runCommand(ctx, &result, options, "refresh package indexes", packages.RefreshCommand()); err != nil {
			return result, err
		}
		if err := i.runCommand(ctx, &result, options, "install Fail2ban", packages.InstallCommand("fail2ban")); err != nil {
			return result, err
		}
	}
	changed, err := ensureFile(i.Paths.Fail2banJail, []byte(fail2banSSHJail), 0o644, options.DryRun)
	if err != nil {
		return result, fmt.Errorf("could not configure Fail2ban SSH jail: %w", err)
	}
	i.fileStep(&result, options, "configure Fail2ban SSH jail", i.Paths.Fail2banJail, changed)
	if err := i.runCommand(ctx, &result, options, "enable and start Fail2ban", services.EnableNowCommand("fail2ban")); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "reload Fail2ban jails", "fail2ban-client", "reload"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "verify Fail2ban SSH jail", "fail2ban-client", "status", "sshd"); err != nil {
		return result, err
	}
	return result, nil
}
