package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"omurga/internal/host"
	"omurga/internal/monitoring"
	"omurga/internal/progress"
	projectruntime "omurga/internal/project"
)

type monitoringResult struct {
	Action          string   `json:"action"`
	ComposePath     string   `json:"composePath"`
	DataRoot        string   `json:"dataRoot"`
	PasswordFile    string   `json:"passwordFile"`
	Command         []string `json:"command,omitempty"`
	Output          string   `json:"output,omitempty"`
	PasswordCreated bool     `json:"passwordCreated,omitempty"`
	BindAddress     string   `json:"bindAddress,omitempty"`
	PrometheusPort  int      `json:"prometheusPort,omitempty"`
	GrafanaPort     int      `json:"grafanaPort,omitempty"`
	DryRun          bool     `json:"dryRun,omitempty"`
}

func newMonitoringCommand(opts *options) *cobra.Command {
	return newGroupCommand("monitoring", "Manage the Prometheus and Grafana monitoring stack",
		newMonitoringInstallCommand(opts),
		newMonitoringStatusCommand(opts),
		newMonitoringRemoveCommand(opts),
	)
}

func newMonitoringInstallCommand(opts *options) *cobra.Command {
	settings := monitoring.DefaultOptions()
	var passwordFile string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install or update Prometheus, Grafana, Node Exporter, and cAdvisor",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			paths := host.DefaultPaths("/")
			if passwordFile != "" {
				absolutePasswordFile, err := filepath.Abs(passwordFile)
				if err != nil {
					return fmt.Errorf("could not resolve Grafana password file: %w", err)
				}
				settings.GrafanaAdminPasswordFile = absolutePasswordFile
			}
			bundle, err := monitoring.Generate(paths, settings)
			if err != nil {
				return err
			}
			command := []string{"docker", "compose", "--project-name", "omurga-monitoring", "--file", bundle.ComposePath, "up", "--detach", "--wait"}
			result := monitoringResult{Action: "install", ComposePath: bundle.ComposePath, DataRoot: bundle.DataRoot, PasswordFile: settings.GrafanaAdminPasswordFile, BindAddress: settings.BindAddress, PrometheusPort: settings.PrometheusPort, GrafanaPort: settings.GrafanaPort, Command: command, DryRun: opts.dryRun}
			if opts.dryRun {
				return writeMonitoringResult(cmd.OutOrStdout(), result, opts)
			}
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "monitoring install"); err != nil {
				return err
			}
			created, err := monitoring.EnsurePassword(settings.GrafanaAdminPasswordFile)
			if err != nil {
				return err
			}
			result.PasswordCreated = created
			if err := os.MkdirAll(bundle.DataRoot, 0o750); err != nil {
				return fmt.Errorf("could not create monitoring data directory: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(bundle.GrafanaDatasourcePath), 0o700); err != nil {
				return fmt.Errorf("could not create Grafana provisioning directory: %w", err)
			}
			if err := projectruntime.WriteArtifact(bundle.ComposePath, bundle.Compose, 0o640); err != nil {
				return err
			}
			if err := projectruntime.WriteArtifact(bundle.PrometheusConfigPath, bundle.PrometheusConfig, 0o640); err != nil {
				return err
			}
			if err := projectruntime.WriteArtifact(bundle.GrafanaDatasourcePath, bundle.GrafanaDatasource, 0o640); err != nil {
				return err
			}
			task := progress.FromContext(cmd.Context()).Start("Pull and start monitoring containers")
			output, err := (host.ExecRunner{}).Run(cmd.Context(), command[0], command[1:]...)
			if err != nil {
				task.Fail(err)
				return err
			}
			task.Complete()
			result.Output = output
			return writeMonitoringResult(cmd.OutOrStdout(), result, opts)
		},
	}
	cmd.Flags().StringVar(&settings.BindAddress, "bind-address", settings.BindAddress, "IP address on which Prometheus and Grafana listen")
	cmd.Flags().IntVar(&settings.PrometheusPort, "prometheus-port", settings.PrometheusPort, "host port for Prometheus")
	cmd.Flags().IntVar(&settings.GrafanaPort, "grafana-port", settings.GrafanaPort, "host port for Grafana")
	cmd.Flags().StringVar(&passwordFile, "grafana-admin-password-file", "", "root-only file containing the Grafana admin password")
	return cmd
}

func newMonitoringStatusCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show monitoring container status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			_, compose, _, _, _, _, _ := monitoring.Layout(host.DefaultPaths("/"))
			command := []string{"docker", "compose", "--project-name", "omurga-monitoring", "--file", compose, "ps"}
			if opts.dryRun {
				return writeMonitoringResult(cmd.OutOrStdout(), monitoringResult{Action: "status", ComposePath: compose, Command: command, DryRun: true}, opts)
			}
			task := progress.FromContext(cmd.Context()).Start("Read monitoring container status")
			output, err := (host.ExecRunner{}).Run(cmd.Context(), command[0], command[1:]...)
			if err != nil {
				task.Fail(err)
				return err
			}
			task.Complete()
			return writeMonitoringResult(cmd.OutOrStdout(), monitoringResult{Action: "status", ComposePath: compose, Command: command, Output: output}, opts)
		},
	}
}

func newMonitoringRemoveCommand(opts *options) *cobra.Command {
	var purgeData bool
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Stop and remove the monitoring stack while preserving data by default",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			paths := host.DefaultPaths("/")
			root, compose, configRoot, prometheusConfig, datasource, dataRoot, _ := monitoring.Layout(paths)
			command := []string{"docker", "compose", "--project-name", "omurga-monitoring", "--file", compose, "down", "--remove-orphans"}
			if opts.dryRun {
				return writeMonitoringResult(cmd.OutOrStdout(), monitoringResult{Action: "remove", ComposePath: compose, DataRoot: dataRoot, Command: command, DryRun: true}, opts)
			}
			if purgeData && !opts.yes {
				return fmt.Errorf("monitoring remove --purge-data requires --yes because persistent data will be permanently deleted")
			}
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "monitoring remove"); err != nil {
				return err
			}
			if _, err := os.Stat(compose); err != nil {
				return fmt.Errorf("monitoring stack was not found")
			}
			if _, err := (host.ExecRunner{}).Run(cmd.Context(), command[0], command[1:]...); err != nil {
				return err
			}
			for _, file := range []string{compose, prometheusConfig, datasource} {
				if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("could not remove generated monitoring file %s: %w", file, err)
				}
			}
			_ = os.Remove(filepath.Dir(datasource))
			_ = os.Remove(filepath.Dir(filepath.Dir(datasource)))
			if purgeData {
				if err := removeWithin(root, dataRoot); err != nil {
					return err
				}
			}
			_ = os.Remove(root)
			_ = os.Remove(configRoot)
			return writeMonitoringResult(cmd.OutOrStdout(), monitoringResult{Action: "remove", ComposePath: compose, DataRoot: dataRoot, Command: command}, opts)
		},
	}
	cmd.Flags().BoolVar(&purgeData, "purge-data", false, "permanently remove monitoring data")
	return cmd
}

func removeWithin(root, target string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(absRoot, absTarget)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return fmt.Errorf("refusing to purge monitoring data outside the monitoring root")
	}
	if err := os.RemoveAll(absTarget); err != nil {
		return fmt.Errorf("could not remove monitoring data: %w", err)
	}
	return nil
}

func writeMonitoringResult(writer io.Writer, result monitoringResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	if result.Output != "" {
		if _, err := fmt.Fprintln(writer, result.Output); err != nil {
			return err
		}
	}
	if result.DryRun {
		_, err := fmt.Fprintf(writer, "would run: %s\n", strings.Join(result.Command, " "))
		return err
	}
	options := monitoring.DefaultOptions()
	if result.BindAddress != "" {
		options.BindAddress = result.BindAddress
	}
	if result.PrometheusPort != 0 {
		options.PrometheusPort = result.PrometheusPort
	}
	if result.GrafanaPort != 0 {
		options.GrafanaPort = result.GrafanaPort
	}
	if result.Action == "install" {
		_, err := fmt.Fprintf(writer, "monitoring %s completed\n%s\nGrafana credentials: %s\n", result.Action, monitoring.Summary(options), result.PasswordFile)
		return err
	}
	_, err := fmt.Fprintf(writer, "monitoring %s completed\n", result.Action)
	return err
}
