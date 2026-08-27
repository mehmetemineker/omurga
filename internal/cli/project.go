package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"omurga/internal/manifest"
)

func newProjectCommand(opts *options) *cobra.Command {
	cmd := newGroupCommand("project", "Manage Docker projects",
		pending("create", "Create a project manifest"),
		pending("list", "List projects"),
		pending("show", "Show project details"),
		pending("deploy", "Reconcile a project to its desired state"),
		pending("status", "Show project status"),
		pending("logs", "Show project logs"),
		pending("restart", "Restart project services"),
		pending("stop", "Stop project services"),
		pending("rollback", "Roll back to the previous deployment"),
		pending("delete", "Remove a project"),
	)
	cmd.AddCommand(newProjectValidateCommand(opts))
	return cmd
}

func newProjectValidateCommand(opts *options) *cobra.Command {
	var environment string

	cmd := &cobra.Command{
		Use:   "validate [project-directory-or-manifest]",
		Short: "Validate a project manifest and merged environment overlay",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			loaded, err := manifest.Load(path, environment)
			if err != nil {
				return err
			}
			if opts.quiet {
				return nil
			}

			result := struct {
				Valid       bool   `json:"valid"`
				Name        string `json:"name"`
				Environment string `json:"environment,omitempty"`
				Manifest    string `json:"manifest"`
			}{true, loaded.Project.Name, loaded.Environment, loaded.Path}

			if opts.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}

			environmentLabel := ""
			if loaded.Environment != "" {
				environmentLabel = fmt.Sprintf(" (%s)", loaded.Environment)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "manifest is valid: %s%s\n", loaded.Project.Name, environmentLabel)
			return err
		},
	}

	cmd.Flags().StringVar(&environment, "env", "", "environment overlay to merge")
	return cmd
}
