package monitoring

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"omurga/internal/host"
)

const (
	PrometheusImage   = "prom/prometheus:v3.5.0"
	GrafanaImage      = "grafana/grafana:12.1.1"
	NodeExporterImage = "prom/node-exporter:v1.9.1"
	CadvisorImage     = "gcr.io/cadvisor/cadvisor:v0.53.0"
)

type Options struct {
	BindAddress              string
	PrometheusPort           int
	GrafanaPort              int
	GrafanaAdminPasswordFile string
}

type Bundle struct {
	Compose               []byte
	PrometheusConfig      []byte
	GrafanaDatasource     []byte
	ComposePath           string
	ConfigRoot            string
	PrometheusConfigPath  string
	GrafanaDatasourcePath string
	DataRoot              string
	PasswordFile          string
}

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
	Secrets  map[string]composeSecret  `yaml:"secrets"`
	Networks map[string]composeNetwork `yaml:"networks"`
}

type composeService struct {
	Image       string               `yaml:"image"`
	Restart     string               `yaml:"restart"`
	Command     []string             `yaml:"command,omitempty"`
	Environment map[string]string    `yaml:"environment,omitempty"`
	Volumes     []string             `yaml:"volumes,omitempty"`
	Ports       []string             `yaml:"ports,omitempty"`
	Networks    []string             `yaml:"networks"`
	Privileged  bool                 `yaml:"privileged,omitempty"`
	Secrets     []string             `yaml:"secrets,omitempty"`
	DependsOn   map[string]dependsOn `yaml:"depends_on,omitempty"`
	Labels      map[string]string    `yaml:"labels,omitempty"`
}

type dependsOn struct {
	Condition string `yaml:"condition"`
}

type composeSecret struct {
	File string `yaml:"file"`
}

type composeNetwork struct {
	Name string `yaml:"name"`
}

func DefaultOptions() Options {
	return Options{
		BindAddress:    "127.0.0.1",
		PrometheusPort: 9090,
		GrafanaPort:    3000,
	}
}

func ValidateOptions(options Options) error {
	if net.ParseIP(options.BindAddress) == nil {
		return fmt.Errorf("monitoring bind address %q is not a valid IP address", options.BindAddress)
	}
	if options.PrometheusPort < 1 || options.PrometheusPort > 65535 {
		return fmt.Errorf("Prometheus port must be between 1 and 65535")
	}
	if options.GrafanaPort < 1 || options.GrafanaPort > 65535 {
		return fmt.Errorf("Grafana port must be between 1 and 65535")
	}
	if options.PrometheusPort == options.GrafanaPort {
		return fmt.Errorf("Prometheus and Grafana ports must be different")
	}
	return nil
}

func Layout(paths host.Paths) (root, compose, configRoot, prometheusConfig, datasource, data, password string) {
	root = paths.MonitoringRoot
	compose = filepath.Join(root, "compose.yaml")
	configRoot = paths.MonitoringConfig
	prometheusConfig = filepath.Join(configRoot, "prometheus.yml")
	datasource = filepath.Join(configRoot, "grafana", "provisioning", "datasources", "datasource.yml")
	data = filepath.Join(root, "data")
	password = paths.MonitoringPassword
	return
}

func Generate(paths host.Paths, options Options) (Bundle, error) {
	if options.GrafanaAdminPasswordFile == "" {
		options.GrafanaAdminPasswordFile = paths.MonitoringPassword
	}
	if err := ValidateOptions(options); err != nil {
		return Bundle{}, err
	}
	_, composePath, configRoot, prometheusPath, datasourcePath, dataRoot, passwordPath := Layout(paths)
	prometheusData := filepath.Join(dataRoot, "prometheus")
	grafanaData := filepath.Join(dataRoot, "grafana")

	compose := composeFile{
		Services: map[string]composeService{
			"prometheus": {
				Image: PrometheusImage, Restart: "unless-stopped",
				Command: []string{
					"--config.file=/etc/prometheus/prometheus.yml",
					"--storage.tsdb.path=/prometheus",
					"--storage.tsdb.retention.time=15d",
				},
				Volumes: []string{
					filepath.ToSlash(prometheusPath) + ":/etc/prometheus/prometheus.yml:ro",
					filepath.ToSlash(prometheusData) + ":/prometheus",
				},
				Ports:    []string{publishPort(options.BindAddress, options.PrometheusPort, 9090)},
				Networks: []string{"omurga-monitoring"},
				DependsOn: map[string]dependsOn{
					"node-exporter": {Condition: "service_started"},
					"cadvisor":      {Condition: "service_started"},
				},
				Labels: managedLabels(),
			},
			"grafana": {
				Image: GrafanaImage, Restart: "unless-stopped",
				Environment: map[string]string{
					"GF_SECURITY_ADMIN_USER":           "admin",
					"GF_SECURITY_ADMIN_PASSWORD__FILE": "/run/secrets/grafana-admin-password",
					"GF_USERS_ALLOW_SIGN_UP":           "false",
				},
				Volumes: []string{
					filepath.ToSlash(grafanaData) + ":/var/lib/grafana",
					filepath.ToSlash(filepath.Dir(datasourcePath)) + ":/etc/grafana/provisioning/datasources:ro",
				},
				Ports:     []string{publishPort(options.BindAddress, options.GrafanaPort, 3000)},
				Networks:  []string{"omurga-monitoring"},
				Secrets:   []string{"grafana-admin-password"},
				DependsOn: map[string]dependsOn{"prometheus": {Condition: "service_started"}},
				Labels:    managedLabels(),
			},
			"node-exporter": {
				Image: NodeExporterImage, Restart: "unless-stopped",
				Command: []string{"--path.rootfs=/host"},
				Volumes: []string{
					"/proc:/host/proc:ro",
					"/sys:/host/sys:ro",
					"/:/host:ro,rslave",
				},
				Networks: []string{"omurga-monitoring"}, Labels: managedLabels(),
			},
			"cadvisor": {
				Image: CadvisorImage, Restart: "unless-stopped", Privileged: true,
				Volumes: []string{
					"/:/rootfs:ro",
					"/var/run:/var/run:ro",
					"/sys:/sys:ro",
					"/var/lib/docker:/var/lib/docker:ro",
					"/dev/disk:/dev/disk:ro",
				},
				Networks: []string{"omurga-monitoring"}, Labels: managedLabels(),
			},
		},
		Secrets: map[string]composeSecret{
			"grafana-admin-password": {File: options.GrafanaAdminPasswordFile},
		},
		Networks: map[string]composeNetwork{
			"omurga-monitoring": {Name: "omurga-monitoring"},
		},
	}
	composeData, err := yaml.Marshal(compose)
	if err != nil {
		return Bundle{}, fmt.Errorf("could not render monitoring Compose file: %w", err)
	}

	prometheusConfig := []byte("global:\n  scrape_interval: 15s\n  evaluation_interval: 15s\n\nscrape_configs:\n  - job_name: omurga-node\n    static_configs:\n      - targets: [node-exporter:9100]\n  - job_name: omurga-containers\n    static_configs:\n      - targets: [cadvisor:8080]\n")
	datasourceConfig := []byte("apiVersion: 1\n\ndatasources:\n  - name: Omurga Prometheus\n    type: prometheus\n    access: proxy\n    url: http://prometheus:9090\n    isDefault: true\n    editable: false\n")

	return Bundle{
		Compose: composeData, PrometheusConfig: prometheusConfig, GrafanaDatasource: datasourceConfig,
		ComposePath: composePath, ConfigRoot: configRoot, PrometheusConfigPath: prometheusPath,
		GrafanaDatasourcePath: datasourcePath, DataRoot: dataRoot, PasswordFile: passwordPath,
	}, nil
}

func managedLabels() map[string]string {
	return map[string]string{
		"dev.omurga.managed":    "true",
		"dev.omurga.monitoring": "true",
	}
}

func EnsurePassword(path string) (created bool, err error) {
	if info, statErr := os.Stat(path); statErr == nil {
		if info.IsDir() {
			return false, fmt.Errorf("Grafana admin password path is a directory: %s", path)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return false, fmt.Errorf("Grafana admin password file permissions are too broad: %s", path)
		}
		return false, nil
	} else if !os.IsNotExist(statErr) {
		return false, fmt.Errorf("could not inspect Grafana admin password file: %w", statErr)
	}

	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return false, fmt.Errorf("could not generate Grafana admin password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(value) + "\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, fmt.Errorf("could not create Grafana credential directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".omurga-grafana-password-")
	if err != nil {
		return false, fmt.Errorf("could not create temporary Grafana password file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return false, err
	}
	if _, err := temporary.WriteString(password); err != nil {
		temporary.Close()
		return false, err
	}
	if err := temporary.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return false, fmt.Errorf("could not create Grafana admin password file: %w", err)
	}
	return true, nil
}

func Port(value int) string { return strconv.Itoa(value) }

func publishPort(address string, hostPort, containerPort int) string {
	return net.JoinHostPort(address, strconv.Itoa(hostPort)) + ":" + strconv.Itoa(containerPort)
}

func Summary(options Options) string {
	return strings.Join([]string{
		"Prometheus: http://" + net.JoinHostPort(options.BindAddress, Port(options.PrometheusPort)),
		"Grafana: http://" + net.JoinHostPort(options.BindAddress, Port(options.GrafanaPort)),
	}, "\n")
}
