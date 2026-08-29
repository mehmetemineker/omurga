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
)

func newAlertCommand(opts *options) *cobra.Command {
	return newGroupCommand("alert", "Manage email and Telegram alerts",
		newAlertStatusCommand(opts),
		newAlertTestCommand(opts),
	)
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
			if err := alert.Send(cmd.Context(), config, channel, "Omurga test alert", message); err != nil {
				return err
			}
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
