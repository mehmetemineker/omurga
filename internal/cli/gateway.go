package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"omurga/internal/host"
	"omurga/internal/progress"
)

type gatewayResult struct {
	Action  string   `json:"action"`
	Status  string   `json:"status,omitempty"`
	Routes  []string `json:"routes,omitempty"`
	Command []string `json:"command,omitempty"`
	Output  string   `json:"output,omitempty"`
	DryRun  bool     `json:"dryRun,omitempty"`
}

func newGatewayCommand(opts *options) *cobra.Command {
	return newGroupCommand("gateway", "Manage the Caddy gateway configuration",
		newGatewayListCommand(opts),
		newGatewayActionCommand(opts, "status"),
		newGatewayActionCommand(opts, "validate"),
		newGatewayActionCommand(opts, "reload"),
	)
}

func newGatewayListCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List managed gateway route files", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLocalHost(opts.host); err != nil {
			return err
		}
		entries, err := os.ReadDir(host.DefaultPaths("/").CaddyProjects)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		var routes []string
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "omurga-") && filepath.Ext(entry.Name()) == ".caddy" && !strings.HasSuffix(entry.Name(), ".previous") {
				routes = append(routes, entry.Name())
			}
		}
		sort.Strings(routes)
		return writeGatewayResult(cmd, opts, gatewayResult{Action: "list", Routes: routes})
	}}
}

func newGatewayActionCommand(opts *options, action string) *cobra.Command {
	short := map[string]string{"status": "Show Caddy service status", "validate": "Validate the complete Caddy configuration", "reload": "Validate and reload Caddy"}[action]
	return &cobra.Command{Use: action, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLocalHost(opts.host); err != nil {
			return err
		}
		paths := host.DefaultPaths("/")
		result := gatewayResult{Action: action, DryRun: opts.dryRun}
		runner := host.ExecRunner{}
		services := host.NewSystemdServiceManager()
		if detected, err := host.DetectServiceManager(paths.OSRelease); err == nil {
			services = detected
		} else if !opts.dryRun {
			return err
		}
		if action == "status" {
			status := services.IsActiveCommand("caddy")
			result.Command = append([]string{status.Name}, status.Args...)
			if !opts.dryRun {
				output, err := runner.Run(cmd.Context(), status.Name, status.Args...)
				if err != nil {
					return err
				}
				result.Status, result.Output = strings.TrimSpace(output), output
			}
			return writeGatewayResult(cmd, opts, result)
		}
		result.Command = []string{"caddy", "validate", "--config", paths.CaddyFile, "--adapter", "caddyfile"}
		if opts.dryRun {
			return writeGatewayResult(cmd, opts, result)
		}
		if action == "reload" {
			if err := requireRoot(cmd.Context(), runner, "gateway reload"); err != nil {
				return err
			}
		}
		task := progress.FromContext(cmd.Context()).Start("Validate Caddy configuration")
		output, err := runner.Run(cmd.Context(), "caddy", "validate", "--config", paths.CaddyFile, "--adapter", "caddyfile")
		if err != nil {
			task.Fail(err)
			return err
		}
		task.Complete()
		result.Output = output
		if action == "reload" {
			reload := services.ReloadCommand("caddy")
			result.Command = append([]string{reload.Name}, reload.Args...)
			task = progress.FromContext(cmd.Context()).Start("Reload Caddy")
			output, err = runner.Run(cmd.Context(), reload.Name, reload.Args...)
			if err != nil {
				task.Fail(err)
				return err
			}
			task.Complete()
			result.Output = output
		}
		return writeGatewayResult(cmd, opts, result)
	}}
}

func writeGatewayResult(cmd *cobra.Command, opts *options, result gatewayResult) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	if result.Action == "list" {
		for _, route := range result.Routes {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), route); err != nil {
				return err
			}
		}
		return nil
	}
	if result.DryRun {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "would run: %s\n", strings.Join(result.Command, " "))
		return err
	}
	if result.Output != "" {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), result.Output)
		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "gateway %s completed\n", result.Action)
	return err
}
