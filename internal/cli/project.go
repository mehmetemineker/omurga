package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"omurga/internal/manifest"
	projectruntime "omurga/internal/project"
)

func newProjectCommand(opts *options) *cobra.Command {
	cmd := newGroupCommand("project", "Manage Docker projects",
		newProjectCreateCommand(opts),
		newProjectRenderCommand(opts),
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
	cmd := &cobra.Command{
		Use:   "validate [project-directory-or-manifest]",
		Short: "Validate a project manifest and merged environment overlay",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			loaded, err := manifest.Load(path, opts.environment)
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

	return cmd
}

func newProjectCreateCommand(opts *options) *cobra.Command {
	var parent string
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a project manifest scaffold",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := projectruntime.Create(args[0], parent, opts.dryRun)
			if err != nil {
				return err
			}
			if opts.quiet {
				return nil
			}
			if opts.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			action := "created"
			if result.DryRun {
				action = "would create"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s project %s at %s\n", action, result.Name, result.Directory)
			return err
		},
	}
	cmd.Flags().StringVar(&parent, "directory", ".", "parent directory for the new project")
	return cmd
}

func newProjectRenderCommand(opts *options) *cobra.Command {
	var kind string
	var output string
	cmd := &cobra.Command{
		Use:   "render [project-directory-or-manifest]",
		Short: "Render the generated Compose or Caddy configuration",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestPath := "."
			if len(args) == 1 {
				manifestPath = args[0]
			}
			if kind != "compose" && kind != "caddy" {
				return fmt.Errorf("kind must be compose or caddy")
			}

			loaded, err := manifest.Load(manifestPath, opts.environment)
			if err != nil {
				return err
			}
			artifacts, err := projectruntime.Generate(loaded.Project, projectruntime.DefaultRenderOptions(loaded.Project, loaded.Environment))
			if err != nil {
				return err
			}
			content := artifacts.Compose
			if kind == "caddy" {
				content = artifacts.Caddy
			}

			if opts.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					Name        string         `json:"name"`
					Environment string         `json:"environment,omitempty"`
					Kind        string         `json:"kind"`
					Content     string         `json:"content"`
					Ports       map[string]int `json:"ports"`
				}{loaded.Project.Name, loaded.Environment, kind, string(content), artifacts.Ports})
			}
			if output == "" || output == "-" {
				if opts.quiet {
					return nil
				}
				_, err := cmd.OutOrStdout().Write(content)
				return err
			}
			if opts.dryRun {
				if !opts.quiet {
					_, err := fmt.Fprintf(cmd.OutOrStdout(), "would write %s configuration to %s\n", kind, output)
					return err
				}
				return nil
			}
			if err := projectruntime.WriteArtifact(output, content, 0o640); err != nil {
				return err
			}
			if !opts.quiet {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "wrote %s configuration to %s\n", kind, output)
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "compose", "artifact kind: compose or caddy")
	cmd.Flags().StringVarP(&output, "output", "o", "-", "output path or - for stdout")
	return cmd
}
