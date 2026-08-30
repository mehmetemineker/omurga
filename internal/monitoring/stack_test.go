package monitoring

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"omurga/internal/host"
)

func TestGenerateMonitoringBundle(t *testing.T) {
	paths := host.DefaultPaths(t.TempDir())
	bundle, err := Generate(paths, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	compose := string(bundle.Compose)
	for _, expected := range []string{PrometheusImage, GrafanaImage, NodeExporterImage, CadvisorImage, "127.0.0.1:9090:9090", "127.0.0.1:3000:3000", "grafana-admin-password", "omurga-monitoring"} {
		if !strings.Contains(compose, expected) {
			t.Fatalf("generated Compose is missing %q:\n%s", expected, compose)
		}
	}
	if !strings.Contains(string(bundle.PrometheusConfig), "node-exporter:9100") || !strings.Contains(string(bundle.PrometheusConfig), "cadvisor:8080") {
		t.Fatalf("generated Prometheus configuration is incomplete: %s", bundle.PrometheusConfig)
	}
	if !strings.Contains(string(bundle.GrafanaDatasource), "http://prometheus:9090") {
		t.Fatalf("generated Grafana datasource is incomplete: %s", bundle.GrafanaDatasource)
	}
}

func TestValidateOptions(t *testing.T) {
	options := DefaultOptions()
	if err := ValidateOptions(options); err != nil {
		t.Fatal(err)
	}
	options.BindAddress = "not-an-ip"
	if err := ValidateOptions(options); err == nil {
		t.Fatal("expected invalid bind address error")
	}
	options = DefaultOptions()
	options.PrometheusPort = options.GrafanaPort
	if err := ValidateOptions(options); err == nil {
		t.Fatal("expected duplicate port error")
	}
}

func TestEnsurePasswordIsIdempotentAndPrivate(t *testing.T) {
	path := t.TempDir() + "/grafana-admin-password"
	created, err := EnsurePassword(path)
	if err != nil || !created {
		t.Fatalf("EnsurePassword() = %v, %v", created, err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("password file mode = %o, want 600", info.Mode().Perm())
	}
	created, err = EnsurePassword(path)
	if err != nil || created {
		t.Fatalf("second EnsurePassword() = %v, %v", created, err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("second EnsurePassword() replaced the existing password")
	}
}
