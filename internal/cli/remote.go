package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"omurga/internal/remote"
)

func executeRemoteIfRequested(cmd *cobra.Command, opts *options) error {
	if opts.host == "" || opts.host == "local" || isLocalProfileCommand(cmd.CommandPath()) {
		return nil
	}
	store, err := remote.DefaultStore()
	if err != nil {
		return err
	}
	profile, exists, err := store.Get(opts.host)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("host profile %s was not found", opts.host)
	}
	arguments := remote.RemoveHostFlag(os.Args[1:])
	tty := strings.HasSuffix(cmd.CommandPath(), " shell")
	if strings.HasSuffix(cmd.CommandPath(), " project logs") {
		follow, _ := cmd.Flags().GetBool("follow")
		tty = follow
	}
	result, err := remote.Execute(cmd.Context(), profile, arguments, tty, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	return newSilentExitError(result.Code)
}

func isLocalProfileCommand(path string) bool {
	for _, suffix := range []string{" host add", " host list", " host show", " host remove"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func newHostAddCommand(opts *options) *cobra.Command {
	var profile remote.Profile
	cmd := &cobra.Command{Use: "add [name] [address]", Short: "Add or update a remote SSH host profile", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		profile.Address = args[1]
		if profile.Port == 0 {
			profile.Port = 22
		}
		store, err := remote.DefaultStore()
		if err != nil {
			return err
		}
		if !opts.dryRun {
			if err := store.Put(args[0], profile); err != nil {
				return err
			}
		}
		return writeHostProfile(cmd, opts, args[0], profile, "added")
	}}
	cmd.Flags().StringVar(&profile.User, "user", "", "SSH user")
	cmd.Flags().IntVar(&profile.Port, "port", 22, "SSH port")
	cmd.Flags().StringVar(&profile.Identity, "identity", "", "SSH private key path")
	cmd.Flags().StringVar(&profile.OmurgaPath, "omurga-path", "omurga", "Omurga executable path on the remote host")
	cmd.Flags().BoolVar(&profile.Sudo, "sudo", true, "run remote Omurga through non-interactive sudo")
	return cmd
}

func newHostListCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List configured remote host profiles", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		store, err := remote.DefaultStore()
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
			profile := profiles[name]
			target := profile.Address
			if profile.User != "" {
				target = profile.User + "@" + target
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-20s %s:%d\n", name, target, profile.Port); err != nil {
				return err
			}
		}
		return nil
	}}
}

func newHostShowCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "show [name]", Short: "Show a remote host profile", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, err := remote.DefaultStore()
		if err != nil {
			return err
		}
		profile, exists, err := store.Get(args[0])
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("host profile %s was not found", args[0])
		}
		return writeHostProfile(cmd, opts, args[0], profile, "")
	}}
}

func newHostRemoveCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "remove [name]", Short: "Remove a remote host profile", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, err := remote.DefaultStore()
		if err != nil {
			return err
		}
		if opts.dryRun {
			return writeHostProfile(cmd, opts, args[0], remote.Profile{}, "removed")
		}
		removed, err := store.Remove(args[0])
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("host profile %s was not found", args[0])
		}
		return writeHostProfile(cmd, opts, args[0], remote.Profile{}, "removed")
	}}
}

func writeHostProfile(cmd *cobra.Command, opts *options, name string, profile remote.Profile, action string) error {
	result := struct {
		Name    string         `json:"name"`
		Profile remote.Profile `json:"profile,omitempty"`
		Action  string         `json:"action,omitempty"`
		DryRun  bool           `json:"dryRun,omitempty"`
	}{name, profile, action, opts.dryRun}
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	if action == "" {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s@%s:%d (binary %s, sudo %t)\n", name, profile.User, profile.Address, profile.Port, profile.OmurgaPath, profile.Sudo)
		return err
	}
	verb := action
	if opts.dryRun {
		verb = "would be " + action
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "host profile %s %s\n", name, verb)
	return err
}
