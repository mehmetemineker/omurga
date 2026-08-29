package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	envstore "omurga/internal/environment"
	"omurga/internal/manifest"
)

func newEnvironmentCommand(opts *options) *cobra.Command {
	return newGroupCommand("env", "Manage project environments and non-secret values",
		newEnvironmentListCommand(opts),
		newEnvironmentShowCommand(opts),
		newEnvironmentSetCommand(opts),
		newEnvironmentUnsetCommand(opts),
	)
}

func loadBaseProject(args []string) (manifest.LoadedProject, error) {
	return loadProjectArgument(args, "")
}

func newEnvironmentListCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "list [project-directory-or-manifest]", Short: "List environments", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadBaseProject(args)
		if err != nil {
			return err
		}
		names, err := envstore.List(loaded.Path)
		if err != nil {
			return err
		}
		if opts.quiet {
			return nil
		}
		if opts.json {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(names)
		}
		for _, name := range names {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), name); err != nil {
				return err
			}
		}
		return nil
	}}
}

func newEnvironmentShowCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "show [environment] [project-directory-or-manifest]", Short: "Show an environment overlay", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadBaseProject(args[1:])
		if err != nil {
			return err
		}
		content, err := envstore.Read(loaded.Path, args[0])
		if err != nil {
			return err
		}
		if opts.quiet {
			return nil
		}
		if opts.json {
			var value any
			if err := yaml.Unmarshal(content, &value); err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(value)
		}
		_, err = cmd.OutOrStdout().Write(content)
		return err
	}}
}

func newEnvironmentSetCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "set [environment] [service] [key] [value] [project-directory-or-manifest]", Short: "Set a non-secret service environment value", Args: cobra.RangeArgs(4, 5), RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadBaseProject(args[4:])
		if err != nil {
			return err
		}
		if _, exists := loaded.Project.Services[args[1]]; !exists {
			return fmt.Errorf("service %s is not defined", args[1])
		}
		path, err := envstore.Path(loaded.Path, args[0])
		if err != nil {
			return err
		}
		if !opts.dryRun {
			path, err = envstore.Set(loaded.Path, args[0], args[1], args[2], args[3])
			if err != nil {
				return err
			}
			if _, err := manifest.Load(filepath.Dir(loaded.Path), args[0]); err != nil {
				return fmt.Errorf("environment was written but the merged manifest is invalid: %w", err)
			}
		}
		return writeEnvironmentMutation(cmd, opts, "set", args[0], args[1], args[2], path)
	}}
}

func newEnvironmentUnsetCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "unset [environment] [service] [key] [project-directory-or-manifest]", Short: "Remove a non-secret service environment value", Args: cobra.RangeArgs(3, 4), RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadBaseProject(args[3:])
		if err != nil {
			return err
		}
		path, err := envstore.Path(loaded.Path, args[0])
		if err != nil {
			return err
		}
		if !opts.dryRun {
			var removed bool
			path, removed, err = envstore.Unset(loaded.Path, args[0], args[1], args[2])
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("environment value %s.%s was not found", args[1], args[2])
			}
		}
		return writeEnvironmentMutation(cmd, opts, "unset", args[0], args[1], args[2], path)
	}}
}

func writeEnvironmentMutation(cmd *cobra.Command, opts *options, action, environment, service, key, path string) error {
	result := map[string]any{"action": action, "environment": environment, "service": service, "key": key, "path": path, "dryRun": opts.dryRun}
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	verb := action
	if opts.dryRun {
		verb = "would " + action
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s.%s in environment %s (%s)\n", verb, service, key, environment, path)
	return err
}
