package host

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
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
	Checks  []Check      `json:"checks"`
	Summary CheckSummary `json:"summary"`
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

	release, err := LoadOSRelease(paths.OSRelease)
	if err != nil {
		add(Check{Name: "operating-system", Status: CheckCritical, Message: fmt.Sprintf("could not read os-release: %v", err)})
	} else if err := ValidateSupportedUbuntu(release); err != nil {
		add(Check{Name: "operating-system", Status: CheckCritical, Message: err.Error()})
	} else {
		name := release.PrettyName
		if name == "" {
			name = "Ubuntu " + release.VersionID
		}
		add(Check{Name: "operating-system", Status: CheckPass, Message: name})
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

	checkCommand(ctx, runner, &report, "apt", "apt-get", []string{"--version"}, CheckWarning)
	checkCommand(ctx, runner, &report, "docker", "docker", []string{"info", "--format", "{{.ServerVersion}}"}, CheckCritical)
	checkCommand(ctx, runner, &report, "docker-compose", "docker", []string{"compose", "version", "--short"}, CheckCritical)
	checkCommand(ctx, runner, &report, "caddy", "caddy", []string{"version"}, CheckCritical)
	checkCommand(ctx, runner, &report, "docker-service", "systemctl", []string{"is-active", "--quiet", "docker"}, CheckCritical)
	checkCommand(ctx, runner, &report, "caddy-service", "systemctl", []string{"is-active", "--quiet", "caddy"}, CheckCritical)

	if _, err := os.Stat(paths.RebootRequired); err == nil {
		addToReport(&report, Check{Name: "reboot", Status: CheckWarning, Message: "a system reboot is required"})
	} else {
		addToReport(&report, Check{Name: "reboot", Status: CheckPass, Message: "no reboot is required"})
	}

	checkDisk(ctx, runner, paths.StateRoot, &report)
	return report
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
