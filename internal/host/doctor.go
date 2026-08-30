package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"omurga/internal/state"
)

type CheckStatus string

const (
	CheckPass     CheckStatus = "pass"
	CheckWarning  CheckStatus = "warning"
	CheckCritical CheckStatus = "critical"
)

type Check struct {
	Name    string      `json:"name"`
	Status  CheckStatus `json:"status"`
	Message string      `json:"message"`
}

type CheckSummary struct {
	Passed   int `json:"passed"`
	Warnings int `json:"warnings"`
	Critical int `json:"critical"`
}

type DoctorReport struct {
	Platform *PlatformInfo `json:"platform,omitempty"`
	Checks   []Check       `json:"checks"`
	Summary  CheckSummary  `json:"summary"`
}

func (r DoctorReport) ExitCode() int {
	if r.Summary.Critical > 0 {
		return 2
	}
	if r.Summary.Warnings > 0 {
		return 1
	}
	return 0
}

func RunDoctor(ctx context.Context, paths Paths, runner Runner) DoctorReport {
	report := DoctorReport{}
	add := func(check Check) {
		report.Checks = append(report.Checks, check)
		switch check.Status {
		case CheckPass:
			report.Summary.Passed++
		case CheckWarning:
			report.Summary.Warnings++
		case CheckCritical:
			report.Summary.Critical++
		}
	}

	release, provider, platform, err := DetectPlatform(paths.OSRelease)
	if err != nil {
		add(Check{Name: "operating-system", Status: CheckCritical, Message: err.Error()})
	} else {
		report.Platform = &platform
		name := release.PrettyName
		if name == "" {
			name = distributionLabel(release.ID) + " " + release.VersionID
		}
		add(Check{Name: "operating-system", Status: CheckPass, Message: fmt.Sprintf("%s (%s/%s, %s support)", name, platform.PackageManager, platform.ServiceManager, platform.SupportLevel)})
	}

	root, err := IsRoot(ctx, runner)
	if err != nil {
		add(Check{Name: "privileges", Status: CheckWarning, Message: "could not determine the effective user"})
	} else if !root {
		add(Check{Name: "privileges", Status: CheckWarning, Message: "host changes require root or sudo"})
	} else {
		add(Check{Name: "privileges", Status: CheckPass, Message: "running as root"})
	}

	missingDirectories := make([]string, 0)
	for _, directory := range paths.ManagedDirectories() {
		if info, err := os.Stat(directory.Path); err != nil || !info.IsDir() {
			missingDirectories = append(missingDirectories, directory.Path)
		}
	}
	if len(missingDirectories) > 0 {
		add(Check{Name: "managed-directories", Status: CheckWarning, Message: fmt.Sprintf("%d managed directories are missing; run host init", len(missingDirectories))})
	} else {
		add(Check{Name: "managed-directories", Status: CheckPass, Message: "all managed directories exist"})
	}

	if provider != nil {
		packageVersion := provider.PackageManager().VersionCommand()
		checkCommand(ctx, runner, &report, "package-manager", packageVersion.Name, packageVersion.Args, CheckWarning)
		serviceVersion := provider.ServiceManager().VersionCommand()
		checkCommand(ctx, runner, &report, "service-manager", serviceVersion.Name, serviceVersion.Args, CheckCritical)
		checkUnattendedUpgrades(ctx, paths, runner, provider.ServiceManager(), &report)
	}
	checkCommand(ctx, runner, &report, "docker", "docker", []string{"info", "--format", "{{.ServerVersion}}"}, CheckCritical)
	checkCommand(ctx, runner, &report, "docker-compose", "docker", []string{"compose", "version", "--short"}, CheckCritical)
	checkCommand(ctx, runner, &report, "caddy", "caddy", []string{"version"}, CheckCritical)
	checkUFW(ctx, runner, &report)
	checkCommand(ctx, runner, &report, "fail2ban", "fail2ban-client", []string{"status", "sshd"}, CheckWarning)
	if provider != nil {
		dockerService := provider.ServiceManager().IsActiveCommand("docker")
		checkCommand(ctx, runner, &report, "docker-service", dockerService.Name, dockerService.Args, CheckCritical)
		caddyService := provider.ServiceManager().IsActiveCommand("caddy")
		checkCommand(ctx, runner, &report, "caddy-service", caddyService.Name, caddyService.Args, CheckCritical)
		fail2banService := provider.ServiceManager().IsActiveCommand("fail2ban")
		checkCommand(ctx, runner, &report, "fail2ban-service", fail2banService.Name, fail2banService.Args, CheckWarning)
	}
	checkCommand(ctx, runner, &report, "caddy-config", "caddy", []string{"validate", "--config", paths.CaddyFile, "--adapter", "caddyfile"}, CheckCritical)
	checkCaddyServiceConfig(ctx, paths, runner, root, &report)
	checkUnhealthyContainers(ctx, runner, &report)

	if _, err := os.Stat(paths.RebootRequired); err == nil {
		addToReport(&report, Check{Name: "reboot", Status: CheckWarning, Message: "a system reboot is required"})
	} else {
		addToReport(&report, Check{Name: "reboot", Status: CheckPass, Message: "no reboot is required"})
	}

	checkDisk(ctx, runner, paths.StateRoot, &report)
	checkInodes(ctx, runner, paths.StateRoot, &report)
	checkSecretPermissions(paths, &report)
	checkStateDatabase(ctx, paths, &report)
	var services ServiceManager
	if provider != nil {
		services = provider.ServiceManager()
	}
	checkBackupTimers(ctx, paths, runner, services, &report)
	return report
}

func checkUFW(ctx context.Context, runner Runner, report *DoctorReport) {
	if _, err := runner.LookPath("ufw"); err != nil {
		addToReport(report, Check{Name: "ufw", Status: CheckWarning, Message: "ufw is not installed"})
		return
	}
	output, err := runner.Run(ctx, "ufw", "status", "verbose")
	if err != nil {
		addToReport(report, Check{Name: "ufw", Status: CheckWarning, Message: err.Error()})
		return
	}
	if !strings.Contains(output, "Status: active") {
		addToReport(report, Check{Name: "ufw", Status: CheckWarning, Message: "UFW is installed but inactive"})
		return
	}
	addToReport(report, Check{Name: "ufw", Status: CheckPass, Message: firstLine(output)})
}

func checkUnattendedUpgrades(ctx context.Context, paths Paths, runner Runner, services ServiceManager, report *DoctorReport) {
	if _, err := runner.LookPath("unattended-upgrade"); err != nil {
		addToReport(report, Check{Name: "automatic-updates", Status: CheckWarning, Message: "unattended-upgrades is not installed"})
		return
	}
	configured, err := unattendedUpgradesConfigured(paths)
	if err != nil {
		addToReport(report, Check{Name: "automatic-updates", Status: CheckWarning, Message: err.Error()})
		return
	}
	if !configured {
		addToReport(report, Check{Name: "automatic-updates", Status: CheckWarning, Message: "unattended-upgrades is installed but its Omurga policy is not configured"})
		return
	}
	if services == nil {
		addToReport(report, Check{Name: "automatic-updates", Status: CheckWarning, Message: "service manager is unavailable"})
		return
	}
	for _, timer := range []string{"apt-daily.timer", "apt-daily-upgrade.timer"} {
		command := services.IsActiveCommand(timer)
		if !commandHealthy(ctx, runner, command.Name, command.Args...) {
			addToReport(report, Check{Name: "automatic-updates", Status: CheckWarning, Message: timer + " is not active"})
			return
		}
	}
	addToReport(report, Check{Name: "automatic-updates", Status: CheckPass, Message: "daily security updates are enabled; automatic reboot is disabled"})
}

func checkCaddyServiceConfig(ctx context.Context, paths Paths, runner Runner, root bool, report *DoctorReport) {
	if _, err := runner.LookPath("caddy"); err != nil {
		return
	}
	if !root {
		addToReport(report, Check{
			Name:    "caddy-service-config",
			Status:  CheckWarning,
			Message: "run doctor with sudo to verify Caddy service-account access to its configuration",
		})
		return
	}
	if _, err := runner.LookPath("runuser"); err != nil {
		addToReport(report, Check{
			Name:    "caddy-service-config",
			Status:  CheckWarning,
			Message: "runuser is unavailable; could not verify Caddy service-account access",
		})
		return
	}
	_, err := runner.Run(ctx, "runuser", "-u", "caddy", "--", "caddy", "validate", "--config", paths.CaddyFile, "--adapter", "caddyfile")
	if err != nil {
		addToReport(report, Check{
			Name:    "caddy-service-config",
			Status:  CheckCritical,
			Message: "Caddy service account cannot validate its configuration; check /etc/caddy directory traversal, Caddyfile/snippet readability, and Omurga imports: " + err.Error(),
		})
		return
	}
	addToReport(report, Check{
		Name:    "caddy-service-config",
		Status:  CheckPass,
		Message: "Caddy service account can read and validate the configured routes",
	})
}

func checkUnhealthyContainers(ctx context.Context, runner Runner, report *DoctorReport) {
	if _, err := runner.LookPath("docker"); err != nil {
		return
	}
	output, err := runner.Run(ctx, "docker", "ps", "--filter", "health=unhealthy", "--format", "{{.Names}}")
	if err != nil {
		addToReport(report, Check{Name: "container-health", Status: CheckWarning, Message: err.Error()})
		return
	}
	if strings.TrimSpace(output) != "" {
		addToReport(report, Check{Name: "container-health", Status: CheckCritical, Message: "unhealthy containers: " + strings.Join(strings.Fields(output), ", ")})
		return
	}
	addToReport(report, Check{Name: "container-health", Status: CheckPass, Message: "no unhealthy containers"})
}

func checkCommand(ctx context.Context, runner Runner, report *DoctorReport, checkName, command string, args []string, missingStatus CheckStatus) {
	if _, err := runner.LookPath(command); err != nil {
		addToReport(report, Check{Name: checkName, Status: missingStatus, Message: command + " is not installed"})
		return
	}
	output, err := runner.Run(ctx, command, args...)
	if err != nil {
		addToReport(report, Check{Name: checkName, Status: missingStatus, Message: err.Error()})
		return
	}
	if output == "" {
		output = "available"
	}
	addToReport(report, Check{Name: checkName, Status: CheckPass, Message: firstLine(output)})
}

func checkDisk(ctx context.Context, runner Runner, path string, report *DoctorReport) {
	if _, err := runner.LookPath("df"); err != nil {
		addToReport(report, Check{Name: "disk", Status: CheckWarning, Message: "df is not available"})
		return
	}
	output, err := runner.Run(ctx, "df", "-P", path)
	if err != nil {
		addToReport(report, Check{Name: "disk", Status: CheckWarning, Message: err.Error()})
		return
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		addToReport(report, Check{Name: "disk", Status: CheckWarning, Message: "could not parse disk usage"})
		return
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 5 {
		addToReport(report, Check{Name: "disk", Status: CheckWarning, Message: "could not parse disk usage"})
		return
	}
	percentageText := strings.TrimSuffix(fields[4], "%")
	percentage, err := strconv.Atoi(percentageText)
	if err != nil {
		addToReport(report, Check{Name: "disk", Status: CheckWarning, Message: "could not parse disk usage"})
		return
	}

	status := CheckPass
	if percentage >= 90 {
		status = CheckCritical
	} else if percentage >= 80 {
		status = CheckWarning
	}
	addToReport(report, Check{Name: "disk", Status: status, Message: fmt.Sprintf("disk usage is %d%%", percentage)})
}

func checkInodes(ctx context.Context, runner Runner, path string, report *DoctorReport) {
	if _, err := runner.LookPath("df"); err != nil {
		addToReport(report, Check{Name: "inodes", Status: CheckWarning, Message: "df is not available"})
		return
	}
	output, err := runner.Run(ctx, "df", "-Pi", path)
	if err != nil {
		addToReport(report, Check{Name: "inodes", Status: CheckWarning, Message: err.Error()})
		return
	}
	percentage, err := parseDFPercentage(output)
	if err != nil {
		addToReport(report, Check{Name: "inodes", Status: CheckWarning, Message: err.Error()})
		return
	}
	status := CheckPass
	if percentage >= 90 {
		status = CheckCritical
	} else if percentage >= 80 {
		status = CheckWarning
	}
	addToReport(report, Check{Name: "inodes", Status: status, Message: fmt.Sprintf("inode usage is %d%%", percentage)})
}

func parseDFPercentage(output string) (int, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("could not parse filesystem usage")
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 5 {
		return 0, fmt.Errorf("could not parse filesystem usage")
	}
	value, err := strconv.Atoi(strings.TrimSuffix(fields[4], "%"))
	if err != nil {
		return 0, fmt.Errorf("could not parse filesystem usage")
	}
	return value, nil
}

func checkSecretPermissions(paths Paths, report *DoctorReport) {
	problems := []string{}
	for _, root := range []string{paths.Secrets, paths.Keys, paths.RuntimeSecrets, paths.MonitoringConfig} {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				if !os.IsNotExist(err) {
					problems = append(problems, path)
				}
				return nil
			}
			info, err := entry.Info()
			if err != nil || (entry.IsDir() && info.Mode().Perm()&0o077 != 0) || (!entry.IsDir() && info.Mode().Perm()&0o077 != 0) {
				problems = append(problems, path)
			}
			return nil
		})
	}
	if len(problems) > 0 {
		addToReport(report, Check{Name: "secret-permissions", Status: CheckCritical, Message: fmt.Sprintf("%d secret or key paths have unsafe permissions", len(problems))})
		return
	}
	addToReport(report, Check{Name: "secret-permissions", Status: CheckPass, Message: "secret and key permissions are restricted"})
}

func checkStateDatabase(ctx context.Context, paths Paths, report *DoctorReport) {
	if _, err := os.Stat(paths.StateDB); os.IsNotExist(err) {
		addToReport(report, Check{Name: "state-database", Status: CheckPass, Message: "state database has not been created yet"})
		return
	} else if err != nil {
		addToReport(report, Check{Name: "state-database", Status: CheckCritical, Message: err.Error()})
		return
	}
	store, err := state.OpenReadOnly(ctx, paths.StateDB)
	if err == nil {
		defer store.Close()
		err = store.IntegrityCheck(ctx)
	}
	if err != nil {
		addToReport(report, Check{Name: "state-database", Status: CheckCritical, Message: err.Error()})
		return
	}
	addToReport(report, Check{Name: "state-database", Status: CheckPass, Message: "SQLite integrity check passed"})
}

func checkBackupTimers(ctx context.Context, paths Paths, runner Runner, services ServiceManager, report *DoctorReport) {
	units, _ := filepath.Glob(filepath.Join(paths.SystemdUnits, "omurga-backup-*.timer"))
	if len(units) == 0 {
		addToReport(report, Check{Name: "backup-timers", Status: CheckPass, Message: "no scheduled backups configured"})
		return
	}
	if services == nil {
		addToReport(report, Check{Name: "backup-timers", Status: CheckCritical, Message: "service manager is unavailable"})
		return
	}
	if _, err := runner.LookPath("restic"); err != nil {
		addToReport(report, Check{Name: "restic", Status: CheckCritical, Message: "restic is required by scheduled backups"})
		return
	}
	command := services.ListTimersCommand("omurga-backup-*.timer")
	output, err := runner.Run(ctx, command.Name, command.Args...)
	if err != nil || strings.TrimSpace(output) == "" {
		addToReport(report, Check{Name: "backup-timers", Status: CheckCritical, Message: "scheduled backup timers are not active"})
		return
	}
	addToReport(report, Check{Name: "backup-timers", Status: CheckPass, Message: fmt.Sprintf("%d backup timer unit(s) configured", len(units))})
}

func addToReport(report *DoctorReport, check Check) {
	report.Checks = append(report.Checks, check)
	switch check.Status {
	case CheckPass:
		report.Summary.Passed++
	case CheckWarning:
		report.Summary.Warnings++
	case CheckCritical:
		report.Summary.Critical++
	}
}

func firstLine(value string) string {
	if line, _, found := strings.Cut(strings.TrimSpace(value), "\n"); found {
		return line
	}
	return strings.TrimSpace(value)
}
