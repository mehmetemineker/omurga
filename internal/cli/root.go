package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"omurga/internal/buildinfo"
)

type options struct {
	host        string
	environment string
	json        bool
	quiet       bool
	dryRun      bool
	yes         bool
}

func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return NewRootCommand().ExecuteContext(ctx)
}

func NewRootCommand() *cobra.Command {
	opts := &options{}

	cmd := &cobra.Command{
		Use:           "omurga",
		Short:         "Manage Ubuntu hosts and Docker projects",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.json && opts.quiet {
				return fmt.Errorf("--json and --quiet cannot be used together")
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&opts.host, "host", "local", "host on which to run the operation")
	cmd.PersistentFlags().StringVar(&opts.environment, "env", "", "project environment overlay")
	cmd.PersistentFlags().BoolVar(&opts.json, "json", false, "write output as JSON")
	cmd.PersistentFlags().BoolVarP(&opts.quiet, "quiet", "q", false, "suppress successful output")
	cmd.PersistentFlags().BoolVar(&opts.dryRun, "dry-run", false, "show the plan without making changes")
	cmd.PersistentFlags().BoolVarP(&opts.yes, "yes", "y", false, "automatically accept safe confirmation prompts")

	cmd.AddCommand(newVersionCommand(opts))
	cmd.AddCommand(newDoctorCommand(opts, "doctor"))
	cmd.AddCommand(newHostCommand(opts))
	cmd.AddCommand(newProjectCommand(opts))
	cmd.AddCommand(newEnvironmentCommand())
	cmd.AddCommand(newSecretCommand())
	cmd.AddCommand(newGatewayCommand())
	cmd.AddCommand(newServiceCommand())
	cmd.AddCommand(newPostgresCommand())
	cmd.AddCommand(newRedisCommand())
	cmd.AddCommand(newBackupCommand())
	cmd.AddCommand(newRegistryCommand())
	cmd.AddCommand(newAlertCommand())

	return cmd
}

func newVersionCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info := struct {
				Version string `json:"version"`
				Commit  string `json:"commit"`
				Date    string `json:"date"`
			}{buildinfo.Version, buildinfo.Commit, buildinfo.Date}

			if opts.quiet {
				return nil
			}
			if opts.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(info)
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "omurga %s (commit: %s, built: %s)\n", info.Version, info.Commit, info.Date)
			return err
		},
	}
}

func newPendingCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s is not implemented yet", cmd.CommandPath())
		},
	}
}

func newGroupCommand(use, short string, children ...*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short}
	cmd.AddCommand(children...)
	return cmd
}

func pending(use, short string) *cobra.Command {
	return newPendingCommand(use, short)
}

func newEnvironmentCommand() *cobra.Command {
	return newGroupCommand("env", "Manage project environments and non-secret values",
		pending("list", "List environments"),
		pending("show", "Show an environment"),
		pending("set", "Set a non-secret environment value"),
		pending("unset", "Remove an environment value"),
	)
}

func newSecretCommand() *cobra.Command {
	return newGroupCommand("secret", "Manage encrypted project secret files",
		pending("set", "Set a secret value"),
		pending("list", "List secret names"),
		pending("remove", "Remove a secret value"),
		pending("rotate", "Rotate a secret value"),
	)
}

func newGatewayCommand() *cobra.Command {
	return newGroupCommand("gateway", "Manage the Caddy gateway configuration",
		pending("list", "List gateway routes"),
		pending("status", "Show Caddy status"),
		pending("validate", "Validate the Caddy configuration"),
		pending("reload", "Reload the Caddy configuration"),
	)
}

func newServiceCommand() *cobra.Command {
	return newGroupCommand("service", "Manage shared Docker services",
		pending("catalog", "Show the built-in service catalog"),
		pending("install", "Install a shared service"),
		pending("list", "List shared services"),
		pending("status", "Show shared service status"),
		pending("remove", "Remove a shared service"),
	)
}

func newPostgresCommand() *cobra.Command {
	return newGroupCommand("postgres", "Manage PostgreSQL instances and databases",
		pending("status", "Show PostgreSQL status"),
		pending("databases", "List databases"),
		pending("create-db", "Create a database"),
		pending("create-user", "Create a PostgreSQL user"),
		pending("shell", "Open a PostgreSQL shell"),
		pending("backup", "Create a PostgreSQL backup"),
		pending("restore", "Restore a PostgreSQL backup"),
	)
}

func newRedisCommand() *cobra.Command {
	return newGroupCommand("redis", "Manage Redis instances",
		pending("status", "Show Redis status"),
		pending("stats", "Show Redis statistics"),
		pending("shell", "Open a Redis shell"),
		pending("backup", "Create a Redis backup"),
		pending("flush", "Flush Redis data"),
	)
}

func newBackupCommand() *cobra.Command {
	return newGroupCommand("backup", "Manage backups and restores",
		pending("create", "Create a backup"),
		pending("list", "List backups"),
		pending("show", "Show backup details"),
		pending("check", "Check backup integrity"),
		pending("restore", "Restore a backup"),
		pending("prune", "Remove backups outside the retention policy"),
		pending("schedule", "Enable scheduled backups"),
		pending("unschedule", "Disable scheduled backups"),
	)
}

func newRegistryCommand() *cobra.Command {
	return newGroupCommand("registry", "Manage Docker registries",
		pending("add", "Add a registry"),
		pending("list", "List registries"),
		pending("login", "Store registry credentials"),
		pending("remove", "Remove a registry"),
	)
}

func newAlertCommand() *cobra.Command {
	return newGroupCommand("alert", "Manage email and Telegram alerts",
		pending("status", "Show alert configuration"),
		pending("test", "Send a test alert"),
	)
}
