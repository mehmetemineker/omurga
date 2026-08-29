package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"omurga/internal/gateway"
	"omurga/internal/host"
	"omurga/internal/manifest"
	"omurga/internal/secret"
)

const maxSecretSize = 1024 * 1024

type secretResult struct {
	Project     string   `json:"project"`
	Environment string   `json:"environment"`
	Name        string   `json:"name,omitempty"`
	Names       []string `json:"names,omitempty"`
	Action      string   `json:"action"`
	DryRun      bool     `json:"dryRun,omitempty"`
}

func newSecretCommand(opts *options) *cobra.Command {
	return newGroupCommand("secret", "Manage encrypted project secret files",
		newSecretWriteCommand(opts, "set"),
		newSecretListCommand(opts),
		newSecretRemoveCommand(opts),
		newSecretWriteCommand(opts, "rotate"),
	)
}

func newSecretWriteCommand(opts *options, action string) *cobra.Command {
	var source string
	short := "Set a secret value"
	if action == "rotate" {
		short = "Replace a secret value"
	}
	cmd := &cobra.Command{
		Use:   action + " [name] [project-directory-or-manifest]",
		Short: short,
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			loaded, err := loadSecretProject(args[1:], opts.environment)
			if err != nil {
				return err
			}
			result := secretResult{Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment), Name: args[0], Action: action, DryRun: opts.dryRun}
			if opts.dryRun {
				return writeSecretResult(cmd.OutOrStdout(), result, opts)
			}
			if source == "" {
				return fmt.Errorf("--file is required; use --file - to read the value from standard input")
			}
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "secret "+action); err != nil {
				return err
			}
			value, err := readSecretValue(source, cmd.InOrStdin())
			if err != nil {
				return err
			}
			if err := secret.NewManager(host.DefaultPaths("/")).Set(result.Project, result.Environment, result.Name, value); err != nil {
				return err
			}
			return writeSecretResult(cmd.OutOrStdout(), result, opts)
		},
	}
	cmd.Flags().StringVar(&source, "file", "", "read the secret value from a file, or - for standard input")
	return cmd
}

func newSecretListCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list [project-directory-or-manifest]",
		Short: "List secret names without revealing values",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			loaded, err := loadSecretProject(args, opts.environment)
			if err != nil {
				return err
			}
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "secret list"); err != nil {
				return err
			}
			environment := gateway.EnvironmentKey(loaded.Environment)
			names, err := secret.NewManager(host.DefaultPaths("/")).List(loaded.Project.Name, environment)
			if err != nil {
				return err
			}
			return writeSecretResult(cmd.OutOrStdout(), secretResult{Project: loaded.Project.Name, Environment: environment, Names: names, Action: "listed"}, opts)
		},
	}
}

func newSecretRemoveCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "remove [name] [project-directory-or-manifest]",
		Short: "Remove a secret value",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			loaded, err := loadSecretProject(args[1:], opts.environment)
			if err != nil {
				return err
			}
			result := secretResult{Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment), Name: args[0], Action: "removed", DryRun: opts.dryRun}
			if opts.dryRun {
				return writeSecretResult(cmd.OutOrStdout(), result, opts)
			}
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "secret remove"); err != nil {
				return err
			}
			removed, err := secret.NewManager(host.DefaultPaths("/")).Remove(result.Project, result.Environment, result.Name)
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("secret %s was not found", result.Name)
			}
			return writeSecretResult(cmd.OutOrStdout(), result, opts)
		},
	}
}

func loadSecretProject(args []string, environment string) (manifest.LoadedProject, error) {
	return loadProjectArgument(args, environment)
}

func readSecretValue(source string, stdin io.Reader) ([]byte, error) {
	var reader io.Reader
	var file *os.File
	if source == "-" {
		reader = stdin
	} else {
		opened, err := os.Open(source)
		if err != nil {
			return nil, fmt.Errorf("could not open secret source: %w", err)
		}
		file = opened
		defer file.Close()
		reader = file
	}
	value, err := io.ReadAll(io.LimitReader(reader, maxSecretSize+1))
	if err != nil {
		return nil, fmt.Errorf("could not read secret value: %w", err)
	}
	if len(value) > maxSecretSize {
		return nil, fmt.Errorf("secret value exceeds the %d-byte limit", maxSecretSize)
	}
	if len(value) == 0 {
		return nil, fmt.Errorf("secret value cannot be empty")
	}
	return value, nil
}

func writeSecretResult(writer io.Writer, result secretResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	if result.Action == "listed" {
		if _, err := fmt.Fprintf(writer, "secrets for %s/%s:\n", result.Project, result.Environment); err != nil {
			return err
		}
		for _, name := range result.Names {
			if _, err := fmt.Fprintf(writer, "  %s\n", name); err != nil {
				return err
			}
		}
		return nil
	}
	verb := result.Action
	if result.DryRun {
		verb = "would be " + result.Action
	}
	_, err := fmt.Fprintf(writer, "secret %s %s for %s/%s\n", result.Name, verb, result.Project, result.Environment)
	return err
}
