package host

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type MonitorOptions struct {
	CPUWarningPercent      int
	CPUCriticalPercent     int
	MemoryWarningPercent   int
	MemoryCriticalPercent  int
	DiskWarningPercent     int
	DiskCriticalPercent    int
	CertificateWarningDays int
	Services               []string
	CertificateRoots       []string
}

// RunMonitor performs the checks used by scheduled alert monitoring. It does
// not send notifications; callers decide which channels should receive issues.
func RunMonitor(ctx context.Context, paths Paths, runner Runner, options MonitorOptions) []Check {
	options = normalizeMonitorOptions(options, paths)
	checks := make([]Check, 0, 6)
	checks = append(checks, monitorCPU(paths, options))
	checks = append(checks, monitorMemory(paths, options))
	checks = append(checks, monitorDisk(ctx, runner, paths.Root, options))
	checks = append(checks, monitorServices(ctx, runner, options.Services))
	checks = append(checks, monitorContainers(ctx, runner))
	checks = append(checks, monitorCertificates(options.CertificateRoots, options.CertificateWarningDays))
	return checks
}

func normalizeMonitorOptions(options MonitorOptions, paths Paths) MonitorOptions {
	if options.DiskWarningPercent == 0 {
		options.DiskWarningPercent = 80
	}
	if options.DiskCriticalPercent == 0 {
		options.DiskCriticalPercent = 90
	}
	if options.CPUWarningPercent == 0 {
		options.CPUWarningPercent = 80
	}
	if options.CPUCriticalPercent == 0 {
		options.CPUCriticalPercent = 95
	}
	if options.MemoryWarningPercent == 0 {
		options.MemoryWarningPercent = 80
	}
	if options.MemoryCriticalPercent == 0 {
		options.MemoryCriticalPercent = 90
	}
	if options.CertificateWarningDays == 0 {
		options.CertificateWarningDays = 30
	}
	if len(options.CertificateRoots) == 0 && paths.CaddyDataRoot != "" {
		options.CertificateRoots = []string{paths.CaddyDataRoot}
	}
	return options
}

func monitorCPU(paths Paths, options MonitorOptions) Check {
	root := paths.Root
	if root == "" {
		root = string(filepath.Separator)
	}
	content, err := os.ReadFile(filepath.Join(root, "proc", "loadavg"))
	if err != nil {
		return Check{Name: "cpu", Status: CheckWarning, Message: "could not read /proc/loadavg: " + err.Error()}
	}
	fields := strings.Fields(string(content))
	if len(fields) == 0 {
		return Check{Name: "cpu", Status: CheckWarning, Message: "could not parse /proc/loadavg"}
	}
	load, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return Check{Name: "cpu", Status: CheckWarning, Message: "could not parse /proc/loadavg"}
	}
	cores := runtime.NumCPU()
	if cores < 1 {
		cores = 1
	}
	percent := load / float64(cores) * 100
	status := CheckPass
	if percent >= float64(options.CPUCriticalPercent) {
		status = CheckCritical
	} else if percent >= float64(options.CPUWarningPercent) {
		status = CheckWarning
	}
	return Check{Name: "cpu", Status: status, Message: fmt.Sprintf("normalized 1-minute CPU load is %.1f%% (%d CPU(s))", percent, cores)}
}

func monitorMemory(paths Paths, options MonitorOptions) Check {
	root := paths.Root
	if root == "" {
		root = string(filepath.Separator)
	}
	content, err := os.ReadFile(filepath.Join(root, "proc", "meminfo"))
	if err != nil {
		return Check{Name: "memory", Status: CheckWarning, Message: "could not read /proc/meminfo: " + err.Error()}
	}
	values := map[string]uint64{}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr == nil {
			values[strings.TrimSuffix(fields[0], ":")] = value
		}
	}
	total, totalOK := values["MemTotal"]
	available, availableOK := values["MemAvailable"]
	if !totalOK || !availableOK || total == 0 || available > total {
		return Check{Name: "memory", Status: CheckWarning, Message: "could not parse /proc/meminfo"}
	}
	percent := float64(total-available) / float64(total) * 100
	status := CheckPass
	if percent >= float64(options.MemoryCriticalPercent) {
		status = CheckCritical
	} else if percent >= float64(options.MemoryWarningPercent) {
		status = CheckWarning
	}
	return Check{Name: "memory", Status: status, Message: fmt.Sprintf("memory usage is %.1f%%", percent)}
}

func monitorDisk(ctx context.Context, runner Runner, path string, options MonitorOptions) Check {
	if path == "" {
		path = string(filepath.Separator)
	}
	if _, err := runner.LookPath("df"); err != nil {
		return Check{Name: "disk", Status: CheckWarning, Message: "df is not available"}
	}
	output, err := runner.Run(ctx, "df", "-P", path)
	if err != nil {
		return Check{Name: "disk", Status: CheckWarning, Message: err.Error()}
	}
	percentage, err := parseDFPercentage(output)
	if err != nil {
		return Check{Name: "disk", Status: CheckWarning, Message: err.Error()}
	}
	status := CheckPass
	if percentage >= options.DiskCriticalPercent {
		status = CheckCritical
	} else if percentage >= options.DiskWarningPercent {
		status = CheckWarning
	}
	return Check{Name: "disk", Status: status, Message: fmt.Sprintf("disk usage is %d%%", percentage)}
}

func monitorServices(ctx context.Context, runner Runner, services []string) Check {
	if _, err := runner.LookPath("systemctl"); err != nil {
		return Check{Name: "services", Status: CheckWarning, Message: "systemctl is not available"}
	}
	output, err := runner.Run(ctx, "systemctl", "--failed", "--no-legend", "--no-pager")
	if err != nil {
		return Check{Name: "services", Status: CheckWarning, Message: err.Error()}
	}
	failed := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			failed = append(failed, fields[0])
		}
	}
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		if _, err := runner.Run(ctx, "systemctl", "is-active", "--quiet", service); err != nil {
			failed = append(failed, service+" (inactive)")
		}
	}
	if len(failed) > 0 {
		failed = uniqueStrings(failed)
		return Check{Name: "services", Status: CheckCritical, Message: "failed or inactive services: " + strings.Join(failed, ", ")}
	}
	return Check{Name: "services", Status: CheckPass, Message: "no failed or monitored inactive services"}
}

func monitorContainers(ctx context.Context, runner Runner) Check {
	if _, err := runner.LookPath("docker"); err != nil {
		return Check{Name: "containers", Status: CheckWarning, Message: "docker is not installed"}
	}
	output, err := runner.Run(ctx, "docker", "ps", "-a", "--filter", "label=dev.omurga.managed=true", "--format", "{{.Names}}\t{{.Status}}")
	if err != nil {
		return Check{Name: "containers", Status: CheckCritical, Message: "could not inspect managed containers: " + err.Error()}
	}
	problems := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimSpace(fields[0])
		status := strings.ToLower(strings.TrimSpace(fields[1]))
		if name == "" {
			continue
		}
		for _, marker := range []string{"unhealthy", "exited", "restarting", "dead", "created", "paused"} {
			if strings.Contains(status, marker) {
				problems = append(problems, name+" ("+status+")")
				break
			}
		}
	}
	if len(problems) > 0 {
		return Check{Name: "containers", Status: CheckCritical, Message: "unhealthy managed containers: " + strings.Join(problems, ", ")}
	}
	return Check{Name: "containers", Status: CheckPass, Message: "all managed containers are healthy or running"}
}

func monitorCertificates(roots []string, warningDays int) Check {
	files := make([]string, 0)
	for _, root := range roots {
		if root == "" {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry == nil || entry.IsDir() {
				return nil
			}
			extension := strings.ToLower(filepath.Ext(path))
			if extension == ".crt" || extension == ".pem" {
				files = append(files, path)
			}
			return nil
		})
	}
	files = uniqueStrings(files)
	if len(files) == 0 {
		return Check{Name: "certificates", Status: CheckPass, Message: "no Caddy certificates found"}
	}
	threshold := time.Duration(warningDays) * 24 * time.Hour
	var expired, expiring, unreadable []string
	var earliest time.Time
	for _, path := range files {
		certificate, err := readCertificate(path)
		if err != nil {
			unreadable = append(unreadable, path)
			continue
		}
		if earliest.IsZero() || certificate.NotAfter.Before(earliest) {
			earliest = certificate.NotAfter
		}
		remaining := time.Until(certificate.NotAfter)
		if remaining <= 0 {
			expired = append(expired, filepath.Base(path))
		} else if remaining <= threshold {
			expiring = append(expiring, fmt.Sprintf("%s (%s)", filepath.Base(path), formatRemaining(remaining)))
		}
	}
	if len(unreadable) > 0 {
		return Check{Name: "certificates", Status: CheckCritical, Message: "could not read certificate files: " + strings.Join(unreadable, ", ")}
	}
	if len(expired) > 0 {
		return Check{Name: "certificates", Status: CheckCritical, Message: "expired certificates: " + strings.Join(expired, ", ")}
	}
	if len(expiring) > 0 {
		return Check{Name: "certificates", Status: CheckWarning, Message: fmt.Sprintf("certificates expire within %d days: %s", warningDays, strings.Join(expiring, ", "))}
	}
	return Check{Name: "certificates", Status: CheckPass, Message: fmt.Sprintf("%d Caddy certificate(s) are valid; earliest expiry %s", len(files), earliest.Format(time.RFC3339))}
}

func readCertificate(path string) (*x509.Certificate, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	for len(content) > 0 {
		block, rest := pem.Decode(content)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			return x509.ParseCertificate(block.Bytes)
		}
		content = rest
	}
	return nil, fmt.Errorf("certificate is not PEM encoded")
}

func formatRemaining(value time.Duration) string {
	days := int(value / (24 * time.Hour))
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dh", int(value/time.Hour))
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
