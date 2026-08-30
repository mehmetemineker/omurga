package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"omurga/internal/alert"
	"omurga/internal/gateway"
	"omurga/internal/host"
	"omurga/internal/manifest"
	"omurga/internal/progress"
)

func newAlertCommand(opts *options) *cobra.Command {
	return newGroupCommand("alert", "Manage email and Telegram alerts",
		newAlertStatusCommand(opts),
		newAlertTestCommand(opts),
		newAlertCheckCommand(opts),
		newAlertScheduleCommand(opts),
		newAlertUnscheduleCommand(opts),
	)
}

type alertCheckResult struct {
	Checks   []host.Check         `json:"checks"`
	New      []alert.MonitorIssue `json:"newIssues,omitempty"`
	Resolved []string             `json:"resolved,omitempty"`
	DryRun   bool                 `json:"dryRun"`
}

func newAlertCheckCommand(opts *options) *cobra.Command {
	var channel string
	cmd := &cobra.Command{Use: "check", Short: "Check host health and send alerts for new issues", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLocalHost(opts.host); err != nil {
			return err
		}
		paths := host.DefaultPaths("/")
		config, err := alert.Load(paths.AlertConfig)
		if err != nil {
			return err
		}
		if !opts.dryRun {
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "alert check"); err != nil {
				return err
			}
		}
		checks := host.RunMonitor(cmd.Context(), paths, host.ExecRunner{}, host.MonitorOptions{
			DiskWarningPercent: config.Monitor.DiskWarningPercent, DiskCriticalPercent: config.Monitor.DiskCriticalPercent,
			CertificateWarningDays: config.Monitor.CertificateWarningDays, Services: config.Monitor.Services,
			CertificateRoots: config.Monitor.CertificateRoots,
		})
		issues := make([]alert.MonitorIssue, 0)
		for _, check := range checks {
			if check.Status == host.CheckPass {
				continue
			}
			issues = append(issues, alert.MonitorIssue{Name: check.Name, Status: string(check.Status), Message: check.Message})
		}
		state := alert.MonitorState{Issues: map[string]string{}}
		if !opts.dryRun {
			state, err = alert.LoadMonitorState(paths.AlertState)
			if err != nil {
				return err
			}
		}
		delta := alert.CompareMonitorState(state, issues)
		result := alertCheckResult{Checks: checks, New: delta.NewIssues, Resolved: delta.Resolved, DryRun: opts.dryRun}
		if !opts.dryRun && (len(delta.NewIssues) > 0 || len(delta.Resolved) > 0) {
			message := formatMonitorAlert(delta.NewIssues, delta.Resolved)
			task := progress.FromContext(cmd.Context()).Start("Send host monitor alert")
			if err := alert.Send(cmd.Context(), config, channel, "Omurga host monitor", message); err != nil {
				task.Fail(err)
				return err
			}
			task.Complete()
			if err := alert.SaveMonitorState(paths.AlertState, delta.NextState); err != nil {
				return err
			}
		}
		if opts.quiet {
			return nil
		}
		if opts.json {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		for _, check := range checks {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s - %s\n", check.Status, check.Name, check.Message); err != nil {
				return err
			}
		}
		if len(delta.NewIssues) == 0 && len(delta.Resolved) == 0 {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "no new or resolved host alerts")
			return err
		}
		return nil
	}}
	cmd.Flags().StringVar(&channel, "channel", "all", "alert channel: all, telegram, or email")
	return cmd
}

func formatMonitorAlert(newIssues []alert.MonitorIssue, resolved []string) string {
	lines := []string{"Omurga host monitor detected a state change."}
	for _, issue := range newIssues {
		lines = append(lines, strings.ToUpper(issue.Status)+" "+issue.Name+": "+issue.Message)
	}
	for _, name := range resolved {
		lines = append(lines, "RESOLVED "+name)
	}
	return strings.Join(lines, "\n")
}

func newAlertScheduleCommand(opts *options) *cobra.Command {
	var schedule string
	cmd := &cobra.Command{Use: "schedule", Short: "Enable the systemd host alert monitor", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLocalHost(opts.host); err != nil {
			return err
		}
		paths := host.DefaultPaths("/")
		config, err := alert.Load(paths.AlertConfig)
		if err != nil {
			return err
		}
		if !config.Monitor.Enabled {
			return fmt.Errorf("alert monitoring is disabled; set monitor.enabled to true in %s", paths.AlertConfig)
		}
		if schedule == "" {
			schedule = config.Monitor.Schedule
		}
		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("could not determine the Omurga executable: %w", err)
		}
		pathsToWrite := alert.SchedulePaths(paths)
		result := map[string]any{"schedule": schedule, "paths": pathsToWrite, "dryRun": opts.dryRun}
		if !opts.dryRun {
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "alert schedule"); err != nil {
				return err
			}
			services, err := host.DetectServiceManager(paths.OSRelease)
			if err != nil {
				return err
			}
			if services.Name() != "systemd" {
				return fmt.Errorf("alert scheduling currently requires systemd; detected %s", services.Name())
			}
			if _, err := alert.WriteSchedule(paths, alert.Schedule{Executable: executable, Calendar: schedule}); err != nil {
				return err
			}
			daemonReload := services.DaemonReloadCommand()
			if _, err := (host.ExecRunner{}).Run(cmd.Context(), daemonReload.Name, daemonReload.Args...); err != nil {
				return err
			}
			enable := services.EnableNowCommand("omurga-alert-monitor.timer")
			if _, err := (host.ExecRunner{}).Run(cmd.Context(), enable.Name, enable.Args...); err != nil {
				return err
			}
		}
		if opts.quiet {
			return nil
		}
		if opts.json {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		verb := "enabled"
		if opts.dryRun {
			verb = "would enable"
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s host alert monitor on %s\n", verb, schedule)
		return err
	}}
	cmd.Flags().StringVar(&schedule, "schedule", "", "systemd calendar expression or HH:MM; defaults to monitor.schedule")
	return cmd
}

func newAlertUnscheduleCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "unschedule", Short: "Disable the systemd host alert monitor", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLocalHost(opts.host); err != nil {
			return err
		}
		paths := host.DefaultPaths("/")
		pathsToRemove := alert.SchedulePaths(paths)
		if !opts.dryRun {
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "alert unschedule"); err != nil {
				return err
			}
			services, err := host.DetectServiceManager(paths.OSRelease)
			if err != nil {
				return err
			}
			if services.Name() != "systemd" {
				return fmt.Errorf("alert scheduling currently requires systemd; detected %s", services.Name())
			}
			disable := services.DisableNowCommand("omurga-alert-monitor.timer")
			if _, err := (host.ExecRunner{}).Run(cmd.Context(), disable.Name, disable.Args...); err != nil {
				return err
			}
			for _, path := range pathsToRemove {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			daemonReload := services.DaemonReloadCommand()
			if _, err := (host.ExecRunner{}).Run(cmd.Context(), daemonReload.Name, daemonReload.Args...); err != nil {
				return err
			}
		}
		if opts.quiet {
			return nil
		}
		if opts.json {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"paths": pathsToRemove, "dryRun": opts.dryRun})
		}
		verb := "disabled"
		if opts.dryRun {
			verb = "would disable"
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s host alert monitor\n", verb)
		return err
	}}
}

func notifyProjectEvent(ctx context.Context, loaded manifest.LoadedProject, event string, cause error) error {
	enabled := false
	for _, configured := range loaded.Project.Alerts.On {
		if configured == event {
			enabled = true
			break
		}
	}
	if !enabled {
		return nil
	}
	config, err := alert.Load(host.DefaultPaths("/").AlertConfig)
	if err != nil {
		return fmt.Errorf("could not load alerts for %s: %w", event, err)
	}
	message := fmt.Sprintf("Omurga event: %s\nProject: %s\nEnvironment: %s\nError: %s", event, loaded.Project.Name, gateway.EnvironmentKey(loaded.Environment), strings.TrimSpace(cause.Error()))
	if err := alert.Send(ctx, config, "all", "Omurga: "+event, message); err != nil {
		return fmt.Errorf("could not send %s alert: %w", event, err)
	}
	return nil
}

func newAlertStatusCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show alert configuration without credentials", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLocalHost(opts.host); err != nil {
			return err
		}
		path := host.DefaultPaths("/").AlertConfig
		config, err := alert.Load(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("alert configuration does not exist at %s", path)
			}
			return err
		}
		if opts.quiet {
			return nil
		}
		if opts.json {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(config)
		}
		content, err := yaml.Marshal(config)
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(content)
		return err
	}}
}

func newAlertTestCommand(opts *options) *cobra.Command {
	var channel, message string
	cmd := &cobra.Command{Use: "test", Short: "Send a test alert", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLocalHost(opts.host); err != nil {
			return err
		}
		result := map[string]any{"channel": channel, "message": message, "dryRun": opts.dryRun}
		if !opts.dryRun {
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "alert test"); err != nil {
				return err
			}
			config, err := alert.Load(host.DefaultPaths("/").AlertConfig)
			if err != nil {
				return err
			}
			task := progress.FromContext(cmd.Context()).Start("Send test alert")
			if err := alert.Send(cmd.Context(), config, channel, "Omurga test alert", message); err != nil {
				task.Fail(err)
				return err
			}
			task.Complete()
		}
		if opts.quiet {
			return nil
		}
		if opts.json {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		verb := "sent"
		if opts.dryRun {
			verb = "would send"
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s test alert through %s\n", verb, channel)
		return err
	}}
	cmd.Flags().StringVar(&channel, "channel", "all", "alert channel: all, telegram, or email")
	cmd.Flags().StringVar(&message, "message", "Omurga alert delivery is working.", "test message")
	return cmd
}
