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

type ResourceMetric struct {
	Name  string
	Value float64
	Unit  string
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

func RunResourceMetrics(ctx context.Context, paths Paths, runner Runner) []ResourceMetric {
	metrics := make([]ResourceMetric, 0, 3)
	if value, err := readCPUMetric(paths); err == nil {
		metrics = append(metrics, ResourceMetric{Name: "host.cpu", Value: value, Unit: "percent"})
	}
	if value, err := readMemoryMetric(paths); err == nil {
		metrics = append(metrics, ResourceMetric{Name: "host.memory", Value: value, Unit: "percent"})
	}
	if value, err := readDiskMetric(ctx, runner, paths.Root); err == nil {
		metrics = append(metrics, ResourceMetric{Name: "host.disk", Value: float64(value), Unit: "percent"})
	}
	metrics = append(metrics, readContainerResourceMetrics(ctx, runner)...)
	return metrics
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
	percent, err := readCPUMetric(paths)
	if err != nil {
		return Check{Name: "cpu", Status: CheckWarning, Message: err.Error()}
	}
	status := CheckPass
	if percent >= float64(options.CPUCriticalPercent) {
		status = CheckCritical
	} else if percent >= float64(options.CPUWarningPercent) {
		status = CheckWarning
	}
	return Check{Name: "cpu", Status: status, Message: fmt.Sprintf("normalized 1-minute CPU load is %.1f%% (%d CPU(s))", percent, runtime.NumCPU())}
}

func monitorMemory(paths Paths, options MonitorOptions) Check {
	percent, err := readMemoryMetric(paths)
	if err != nil {
		return Check{Name: "memory", Status: CheckWarning, Message: err.Error()}
	}
	status := CheckPass
	if percent >= float64(options.MemoryCriticalPercent) {
		status = CheckCritical
	} else if percent >= float64(options.MemoryWarningPercent) {
		status = CheckWarning
	}
	return Check{Name: "memory", Status: status, Message: fmt.Sprintf("memory usage is %.1f%%", percent)}
}

func readCPUMetric(paths Paths) (float64, error) {
	root := paths.Root
	if root == "" {
		root = string(filepath.Separator)
	}
	content, err := os.ReadFile(filepath.Join(root, "proc", "loadavg"))
	if err != nil {
		return 0, fmt.Errorf("could not read /proc/loadavg: %w", err)
	}
	fields := strings.Fields(string(content))
	if len(fields) == 0 {
		return 0, fmt.Errorf("could not parse /proc/loadavg")
	}
	load, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("could not parse /proc/loadavg")
	}
	cores := runtime.NumCPU()
	if cores < 1 {
		cores = 1
	}
	return load / float64(cores) * 100, nil
}

func readMemoryMetric(paths Paths) (float64, error) {
	root := paths.Root
	if root == "" {
		root = string(filepath.Separator)
	}
	content, err := os.ReadFile(filepath.Join(root, "proc", "meminfo"))
	if err != nil {
		return 0, fmt.Errorf("could not read /proc/meminfo: %w", err)
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
		return 0, fmt.Errorf("could not parse /proc/meminfo")
	}
	return float64(total-available) / float64(total) * 100, nil
}

func readDiskMetric(ctx context.Context, runner Runner, path string) (int, error) {
	if path == "" {
		path = string(filepath.Separator)
	}
	if _, err := runner.LookPath("df"); err != nil {
		return 0, fmt.Errorf("df is not available")
	}
	output, err := runner.Run(ctx, "df", "-P", path)
	if err != nil {
		return 0, err
	}
	return parseDFPercentage(output)
}

func monitorDisk(ctx context.Context, runner Runner, path string, options MonitorOptions) Check {
	percentage, err := readDiskMetric(ctx, runner, path)
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

func readContainerResourceMetrics(ctx context.Context, runner Runner) []ResourceMetric {
	if _, err := runner.LookPath("docker"); err != nil {
		return nil
	}
	output, err := runner.Run(ctx, "docker", "stats", "--no-stream", "--filter", "label=dev.omurga.managed=true", "--format", "{{.Name}}\t{{.CPUPerc}}\t{{.MemPerc}}")
	if err != nil {
		return nil
	}
	metrics := make([]ResourceMetric, 0)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || strings.TrimSpace(fields[0]) == "" {
			continue
		}
		cpu, cpuErr := parseResourcePercentage(fields[1])
		memory, memoryErr := parseResourcePercentage(fields[2])
		if cpuErr == nil {
			metrics = append(metrics, ResourceMetric{Name: "container." + strings.TrimSpace(fields[0]) + ".cpu", Value: cpu, Unit: "percent"})
		}
		if memoryErr == nil {
			metrics = append(metrics, ResourceMetric{Name: "container." + strings.TrimSpace(fields[0]) + ".memory", Value: memory, Unit: "percent"})
		}
	}
	return metrics
}

func parseResourcePercentage(value string) (float64, error) {
	value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
	return strconv.ParseFloat(value, 64)
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
		// systemctl may print a summary such as "0 loaded units listed."
		// Failed unit names always have a unit suffix (for example .service).
		if len(fields) > 0 && strings.Contains(fields[0], ".") {
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
