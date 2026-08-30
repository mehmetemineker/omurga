package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"omurga/internal/buildinfo"
	"omurga/internal/progress"
)

type options struct {
	host        string
	environment string
	json        bool
	quiet       bool
	dryRun      bool
	yes         bool
	progress    string
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
		Short:         "Manage Linux hosts and Docker projects",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.json && opts.quiet {
				return fmt.Errorf("--json and --quiet cannot be used together")
			}
			mode, err := progress.ParseMode(opts.progress)
			if err != nil {
				return err
			}
			if opts.json || opts.quiet || opts.dryRun {
				mode = progress.ModeOff
			}
			cmd.SetContext(progress.WithReporter(cmd.Context(), progress.New(cmd.ErrOrStderr(), mode)))
			return executeRemoteIfRequested(cmd, opts)
		},
	}

	cmd.PersistentFlags().StringVar(&opts.host, "host", "local", "host on which to run the operation")
	cmd.PersistentFlags().StringVar(&opts.environment, "env", "", "project environment overlay")
	cmd.PersistentFlags().BoolVar(&opts.json, "json", false, "write output as JSON")
	cmd.PersistentFlags().BoolVarP(&opts.quiet, "quiet", "q", false, "suppress successful output")
	cmd.PersistentFlags().BoolVar(&opts.dryRun, "dry-run", false, "show the plan without making changes")
	cmd.PersistentFlags().BoolVarP(&opts.yes, "yes", "y", false, "automatically accept safe confirmation prompts")
	cmd.PersistentFlags().StringVar(&opts.progress, "progress", "auto", "progress display: auto, tty, plain, or off")

	cmd.AddCommand(newVersionCommand(opts))
	cmd.AddCommand(newDoctorCommand(opts, "doctor"))
	cmd.AddCommand(newHostCommand(opts))
	cmd.AddCommand(newProjectCommand(opts))
	cmd.AddCommand(newEnvironmentCommand(opts))
	cmd.AddCommand(newSecretCommand(opts))
	cmd.AddCommand(newGatewayCommand(opts))
	cmd.AddCommand(newServiceCommand(opts))
	cmd.AddCommand(newPostgresCommand(opts))
	cmd.AddCommand(newRedisCommand(opts))
	cmd.AddCommand(newBackupCommand(opts))
	cmd.AddCommand(newRegistryCommand(opts))
	cmd.AddCommand(newAlertCommand(opts))

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

func newGroupCommand(use, short string, children ...*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short}
	cmd.AddCommand(children...)
	return cmd
}
