package host

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	available map[string]bool
	outputs   map[string]string
	errors    map[string]error
	calls     []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	key := commandKey(name, args...)
	r.calls = append(r.calls, key)
	if err := r.errors[key]; err != nil {
		return r.outputs[key], err
	}
	if name == "gpg" {
		for index, arg := range args {
			if arg == "--output" && index+1 < len(args) {
				if err := os.WriteFile(args[index+1], []byte("dearmored-key"), 0o600); err != nil {
					return "", err
				}
			}
		}
	}
	if name == "df" {
		return "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/sda1 100000 20000 80000 20% /", nil
	}
	return r.outputs[key], nil
}

func (r *fakeRunner) LookPath(name string) (string, error) {
	if r.available[name] {
		return "/usr/bin/" + name, nil
	}
	return "", fmt.Errorf("not found")
}

func TestUpdatePackagesUsesSafeUpgradeByDefault(t *testing.T) {
	runner := &fakeRunner{outputs: map[string]string{}, errors: map[string]error{}}
	result, err := UpdatePackages(context.Background(), runner, NewUbuntuProvider(), false, false)
	if err != nil {
		t.Fatalf("UpdatePackages() error = %v", err)
	}
	if result.Mode != "safe" || len(runner.calls) != 2 {
		t.Fatalf("unexpected update result: %#v, calls: %#v", result, runner.calls)
	}
	if !strings.Contains(runner.calls[1], " upgrade") || strings.Contains(runner.calls[1], "full-upgrade") {
		t.Fatalf("safe update used the wrong command: %s", runner.calls[1])
	}
}

func TestUpdatePackagesDryRunDoesNotExecuteCommands(t *testing.T) {
	runner := &fakeRunner{}
	result, err := UpdatePackages(context.Background(), runner, NewUbuntuProvider(), true, true)
	if err != nil {
		t.Fatalf("UpdatePackages(dryRun) error = %v", err)
	}
	if len(runner.calls) != 0 || !result.DryRun || result.Mode != "full" {
		t.Fatalf("unexpected dry-run result: %#v, calls: %#v", result, runner.calls)
	}
}

func TestDoctorReportsHealthyInitializedHost(t *testing.T) {
	paths := createInitializedPaths(t)
	runner := healthyRunner()

	report := RunDoctor(context.Background(), paths, runner)
	if report.ExitCode() != 0 {
		t.Fatalf("expected healthy report, got: %#v", report)
	}
}

func TestDoctorReportsMissingDockerAsCritical(t *testing.T) {
	paths := createInitializedPaths(t)
	runner := healthyRunner()
	delete(runner.available, "docker")

	report := RunDoctor(context.Background(), paths, runner)
	if report.ExitCode() != 2 || report.Summary.Critical == 0 {
		t.Fatalf("expected critical report, got: %#v", report)
	}
}

func TestDoctorReportsInactiveUFW(t *testing.T) {
	paths := createInitializedPaths(t)
	runner := healthyRunner()
	runner.outputs[commandKey("ufw", "status", "verbose")] = "Status: inactive"

	report := RunDoctor(context.Background(), paths, runner)
	for _, check := range report.Checks {
		if check.Name == "ufw" {
			if check.Status != CheckWarning || !strings.Contains(check.Message, "inactive") {
				t.Fatalf("unexpected UFW check: %#v", check)
			}
			return
		}
	}
	t.Fatal("UFW check was not reported")
}

func TestDoctorReportsCaddyServiceConfigAccessFailure(t *testing.T) {
	paths := createInitializedPaths(t)
	runner := healthyRunner()
	command := commandKey("runuser", "-u", "caddy", "--", "caddy", "validate", "--config", paths.CaddyFile, "--adapter", "caddyfile")
	runner.errors[command] = errors.New("permission denied")

	report := RunDoctor(context.Background(), paths, runner)
	if report.ExitCode() != 2 {
		t.Fatalf("expected critical report, got: %#v", report)
	}
	for _, check := range report.Checks {
		if check.Name == "caddy-service-config" {
			if check.Status != CheckCritical || !strings.Contains(check.Message, "permission denied") {
				t.Fatalf("unexpected Caddy service config check: %#v", check)
			}
			return
		}
	}
	t.Fatal("Caddy service config check was not reported")
}

func TestRunMonitorReportsFailedServices(t *testing.T) {
	paths := DefaultPaths(t.TempDir())
	runner := healthyRunner()
	runner.outputs[commandKey("systemctl", "--failed", "--no-legend", "--no-pager")] = "caddy.service loaded failed failed Caddy"

	checks := RunMonitor(context.Background(), paths, runner, MonitorOptions{})
	for _, check := range checks {
		if check.Name == "services" {
			if check.Status != CheckCritical || !strings.Contains(check.Message, "caddy.service") {
				t.Fatalf("unexpected service monitor result: %#v", check)
			}
			return
		}
	}
	t.Fatal("service monitor check was not reported")
}

func TestMonitorReportsExpiringCertificate(t *testing.T) {
	root := t.TempDir()
	certificatePath := filepath.Join(root, "example.crt")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "example.com"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certificatePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}

	check := monitorCertificates([]string{root}, 30)
	if check.Status != CheckWarning || !strings.Contains(check.Message, "example.crt") {
		t.Fatalf("unexpected certificate monitor result: %#v", check)
	}
}

func TestMonitorReportsCPUAndMemoryThresholds(t *testing.T) {
	root := t.TempDir()
	proc := filepath.Join(root, "proc")
	if err := os.MkdirAll(proc, 0o755); err != nil {
		t.Fatal(err)
	}
	load := float64(runtime.NumCPU()) * 0.85
	if err := os.WriteFile(filepath.Join(proc, "loadavg"), []byte(strconv.FormatFloat(load, 'f', 2, 64)+" 0.00 0.00 1/100 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proc, "meminfo"), []byte("MemTotal:       1000 kB\nMemAvailable:    100 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := DefaultPaths(root)
	options := MonitorOptions{CPUWarningPercent: 80, CPUCriticalPercent: 95, MemoryWarningPercent: 80, MemoryCriticalPercent: 90}
	cpu := monitorCPU(paths, options)
	if cpu.Status != CheckWarning {
		t.Fatalf("unexpected CPU monitor result: %#v", cpu)
	}
	memory := monitorMemory(paths, options)
	if memory.Status != CheckCritical {
		t.Fatalf("unexpected memory monitor result: %#v", memory)
	}
}

func TestMonitorReportsUnhealthyManagedContainer(t *testing.T) {
	runner := healthyRunner()
	runner.outputs[commandKey("docker", "ps", "-a", "--filter", "label=dev.omurga.managed=true", "--format", "{{.Names}}\t{{.Status}}")] = "omurga-demo-app-1\tExited (1) 2 minutes ago"
	check := monitorContainers(context.Background(), runner)
	if check.Status != CheckCritical || !strings.Contains(check.Message, "omurga-demo-app-1") {
		t.Fatalf("unexpected container monitor result: %#v", check)
	}
}

func healthyRunner() *fakeRunner {
	return &fakeRunner{
		available: map[string]bool{
			"apt-get":            true,
			"docker":             true,
			"caddy":              true,
			"ufw":                true,
			"unattended-upgrade": true,
			"fail2ban-client":    true,
			"df":                 true,
			"runuser":            true,
			"systemctl":          true,
		},
		outputs: map[string]string{
			commandKey("id", "-u"):                                                     "0",
			commandKey("apt-get", "--version"):                                         "apt 2.4.14",
			commandKey("docker", "info", "--format", "{{.ServerVersion}}"):             "27.5.1",
			commandKey("docker", "compose", "version", "--short"):                      "2.32.4",
			commandKey("caddy", "version"):                                             "v2.9.1",
			commandKey("ufw", "status", "verbose"):                                     "Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n22/tcp ALLOW IN Anywhere\n80/tcp ALLOW IN Anywhere\n443/tcp ALLOW IN Anywhere",
			commandKey("systemctl", "is-active", "--quiet", "apt-daily.timer"):         "",
			commandKey("systemctl", "is-active", "--quiet", "apt-daily-upgrade.timer"): "",
			commandKey("fail2ban-client", "status", "sshd"):                            "Status for the jail: sshd",
			commandKey("systemctl", "is-active", "--quiet", "docker"):                  "",
			commandKey("systemctl", "is-active", "--quiet", "caddy"):                   "",
			commandKey("systemctl", "is-active", "--quiet", "fail2ban"):                "",
		},
		errors: map[string]error{},
	}
}

func createInitializedPaths(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	paths := DefaultPaths(root)
	if err := os.MkdirAll(filepath.Dir(paths.OSRelease), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.OSRelease, []byte("ID=ubuntu\nVERSION_ID=24.04\nVERSION_CODENAME=noble\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, directory := range paths.ManagedDirectories() {
		if err := os.MkdirAll(directory.Path, directory.Mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(paths.APTPeriodicConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.APTPeriodicConfig, []byte(aptPeriodicConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.UnattendedUpgrades, []byte(unattendedUpgradesConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	return paths
}

func commandKey(name string, args ...string) string {
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}

func TestMergedEnvironmentReplacesExistingValues(t *testing.T) {
	actual := mergedEnvironment([]string{"PATH=/bin", "AWS_ACCESS_KEY_ID=old"}, map[string]string{"AWS_ACCESS_KEY_ID": "new"})
	joined := strings.Join(actual, "\n")
	if strings.Contains(joined, "AWS_ACCESS_KEY_ID=old") || !strings.Contains(joined, "AWS_ACCESS_KEY_ID=new") {
		t.Fatalf("unexpected merged environment: %#v", actual)
	}
}
