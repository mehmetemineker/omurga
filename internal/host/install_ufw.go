package host

import (
	"context"
	"fmt"
	"strings"
)

const defaultUFWSSHPort = 22

// InstallUFW installs and configures UFW with a conservative host firewall.
// Only SSH, HTTP, and HTTPS are opened by default. Application ports must be
// explicitly allowed by the operator.
func (i Installer) InstallUFW(ctx context.Context, release OSRelease, options InstallOptions) (InstallResult, error) {
	result := InstallResult{Component: "ufw", DryRun: options.DryRun}
	if err := i.validate(); err != nil {
		return result, err
	}
	sshPort := options.UFWSSHPort
	if sshPort == 0 {
		sshPort = defaultUFWSSHPort
	}
	if err := validateUFWSSHPort(sshPort); err != nil {
		return result, err
	}
	provider, err := i.providerFor(release)
	if err != nil {
		return result, err
	}
	packages := provider.PackageManager()

	if packages.IsInstalled(ctx, i.Runner, "ufw") && ufwConfigured(ctx, i.Runner, sshPort) {
		result.AlreadyInstalled = true
		result.Steps = append(result.Steps, InstallStep{Name: "verify UFW firewall", Status: "unchanged"})
		return result, nil
	}

	if !packages.IsInstalled(ctx, i.Runner, "ufw") {
		if err := i.runCommand(ctx, &result, options, "refresh package indexes", packages.RefreshCommand()); err != nil {
			return result, err
		}
		if err := i.runCommand(ctx, &result, options, "install UFW", packages.InstallCommand("ufw")); err != nil {
			return result, err
		}
	}

	if err := i.runStep(ctx, &result, options, "set default incoming policy", "ufw", "default", "deny", "incoming"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "set default outgoing policy", "ufw", "default", "allow", "outgoing"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "allow SSH", "ufw", "allow", fmt.Sprintf("%d/tcp", sshPort)); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "allow HTTP", "ufw", "allow", "80/tcp"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "allow HTTPS", "ufw", "allow", "443/tcp"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "enable UFW", "ufw", "--force", "enable"); err != nil {
		return result, err
	}
	if err := i.runStep(ctx, &result, options, "verify UFW firewall", "ufw", "status", "verbose"); err != nil {
		return result, err
	}
	if !options.DryRun && !ufwConfigured(ctx, i.Runner, sshPort) {
		return result, fmt.Errorf("UFW did not reach the expected active state")
	}
	return result, nil
}

func ufwConfigured(ctx context.Context, runner Runner, sshPort int) bool {
	if _, err := runner.LookPath("ufw"); err != nil {
		return false
	}
	output, err := runner.Run(ctx, "ufw", "status", "verbose")
	if err != nil {
		return false
	}
	required := []string{
		"Status: active",
		"Default: deny (incoming)",
		"allow (outgoing)",
		fmt.Sprintf("%d/tcp", sshPort),
		"80/tcp",
		"443/tcp",
	}
	for _, value := range required {
		if !strings.Contains(output, value) {
			return false
		}
	}
	return true
}

func validateUFWSSHPort(value int) error {
	if value < 1 || value > 65535 {
		return fmt.Errorf("UFW SSH port must be between 1 and 65535: %d", value)
	}
	return nil
}
