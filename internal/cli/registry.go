package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"omurga/internal/host"
	"omurga/internal/progress"
	registrystore "omurga/internal/registry"
)

func newRegistryCommand(opts *options) *cobra.Command {
	return newGroupCommand("registry", "Manage Docker registry profiles",
		newRegistryAddCommand(opts),
		newRegistryListCommand(opts),
		newRegistryLoginCommand(opts),
		newRegistryRemoveCommand(opts),
	)
}

func newRegistryAddCommand(opts *options) *cobra.Command {
	var username string
	cmd := &cobra.Command{Use: "add [name] [address]", Short: "Add or update a registry profile", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		profile := registrystore.Profile{Address: args[1], Username: username}
		if !opts.dryRun {
			store, err := registrystore.DefaultStore()
			if err != nil {
				return err
			}
			if err := store.Put(args[0], profile); err != nil {
				return err
			}
		}
		return writeRegistryResult(cmd, opts, args[0], profile, "added", "")
	}}
	cmd.Flags().StringVar(&username, "username", "", "registry username")
	return cmd
}

func newRegistryListCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List registry profiles", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		store, err := registrystore.DefaultStore()
		if err != nil {
			return err
		}
		names, profiles, err := store.List()
		if err != nil {
			return err
		}
		if opts.quiet {
			return nil
		}
		if opts.json {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(profiles)
		}
		for _, name := range names {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-20s %s\n", name, profiles[name].Address); err != nil {
				return err
			}
		}
		return nil
	}}
}

func newRegistryLoginCommand(opts *options) *cobra.Command {
	var passwordFile, username string
	cmd := &cobra.Command{Use: "login [name]", Short: "Authenticate Docker to a configured registry", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, err := registrystore.DefaultStore()
		if err != nil {
			return err
		}
		profile, exists, err := store.Get(args[0])
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("registry profile %s was not found", args[0])
		}
		if username == "" {
			username = profile.Username
		}
		if username == "" {
			return fmt.Errorf("registry username is required in the profile or --username")
		}
		if opts.dryRun {
			return writeRegistryResult(cmd, opts, args[0], profile, "login", "")
		}
		if passwordFile == "" {
			return fmt.Errorf("--password-file is required; use - to read from standard input")
		}
		password, err := readSecretValue(passwordFile, cmd.InOrStdin())
		if err != nil {
			return err
		}
		var output bytes.Buffer
		task := progress.FromContext(cmd.Context()).Start("Sign in to Docker registry")
		if err := (host.ExecRunner{}).RunIO(cmd.Context(), bytes.NewReader(password), &output, cmd.ErrOrStderr(), "docker", "login", profile.Address, "--username", username, "--password-stdin"); err != nil {
			task.Fail(err)
			return err
		}
		task.Complete()
		return writeRegistryResult(cmd, opts, args[0], profile, "login", strings.TrimSpace(output.String()))
	}}
	cmd.Flags().StringVar(&passwordFile, "password-file", "", "read the registry password from a file, or - for standard input")
	cmd.Flags().StringVar(&username, "username", "", "override the profile username")
	return cmd
}

func newRegistryRemoveCommand(opts *options) *cobra.Command {
	var logout bool
	cmd := &cobra.Command{Use: "remove [name]", Short: "Remove a registry profile", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, err := registrystore.DefaultStore()
		if err != nil {
			return err
		}
		profile, exists, err := store.Get(args[0])
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("registry profile %s was not found", args[0])
		}
		if !opts.dryRun {
			if logout {
				if _, err := (host.ExecRunner{}).Run(cmd.Context(), "docker", "logout", profile.Address); err != nil {
					return err
				}
			}
			if _, removed, err := store.Remove(args[0]); err != nil {
				return err
			} else if !removed {
				return fmt.Errorf("registry profile %s was not found", args[0])
			}
		}
		return writeRegistryResult(cmd, opts, args[0], profile, "removed", "")
	}}
	cmd.Flags().BoolVar(&logout, "logout", true, "also remove Docker's stored credentials")
	return cmd
}

func writeRegistryResult(cmd *cobra.Command, opts *options, name string, profile registrystore.Profile, action, output string) error {
	result := map[string]any{"name": name, "profile": profile, "action": action, "output": output, "dryRun": opts.dryRun}
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	if output != "" {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), output)
		return err
	}
	verb := action
	if opts.dryRun {
		verb = "would be " + action
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "registry profile %s %s\n", name, verb)
	return err
}
