package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"omurga/internal/host"
	"omurga/internal/manifest"
	projectruntime "omurga/internal/project"
)

func newProjectCommand(opts *options) *cobra.Command {
	cmd := newGroupCommand("project", "Manage Docker projects",
		newProjectCreateCommand(opts),
		newProjectRenderCommand(opts),
		newProjectDeployCommand(opts),
		newProjectStatusCommand(opts),
		newProjectControlCommand(opts, "restart"),
		newProjectControlCommand(opts, "stop"),
		pending("list", "List projects"),
		pending("show", "Show project details"),
		pending("logs", "Show project logs"),
		pending("rollback", "Roll back to the previous deployment"),
		pending("delete", "Remove a project"),
	)
	cmd.AddCommand(newProjectValidateCommand(opts))
	return cmd
}

func newProjectDeployCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [project-directory-or-manifest]",
		Short: "Reconcile a project to its desired state",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			loaded, err := loadProjectArgument(args, opts.environment)
			if err != nil {
				return err
			}
			runner := host.ExecRunner{}
			if !opts.dryRun {
				root, err := host.IsRoot(cmd.Context(), runner)
				if err != nil {
					return fmt.Errorf("could not verify root privileges: %w", err)
				}
				if !root {
					return fmt.Errorf("project deploy requires root privileges; run it with sudo")
				}
			}
			lifecycle := projectruntime.NewLifecycle(host.DefaultPaths("/"), runner)
			result, err := lifecycle.Deploy(cmd.Context(), loaded, opts.dryRun)
			if err != nil {
				return err
			}
			return writeDeployResult(cmd.OutOrStdout(), result, opts)
		},
	}
	return cmd
}

func newProjectStatusCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status [project-directory-or-manifest]",
		Short: "Show project deployment and container status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			loaded, err := loadProjectArgument(args, opts.environment)
			if err != nil {
				return err
			}
			lifecycle := projectruntime.NewLifecycle(host.DefaultPaths("/"), host.ExecRunner{})
			result, err := lifecycle.Status(cmd.Context(), loaded)
			if err != nil {
				return err
			}
			return writeProjectStatus(cmd.OutOrStdout(), result, opts)
		},
	}
}

func newProjectControlCommand(opts *options, action string) *cobra.Command {
	short := "Restart project services"
	if action == "stop" {
		short = "Stop project services without removing containers"
	}
	return &cobra.Command{
		Use:   action + " [project-directory-or-manifest]",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			loaded, err := loadProjectArgument(args, opts.environment)
			if err != nil {
				return err
			}
			runner := host.ExecRunner{}
			if !opts.dryRun {
				root, err := host.IsRoot(cmd.Context(), runner)
				if err != nil {
					return fmt.Errorf("could not verify root privileges: %w", err)
				}
				if !root {
					return fmt.Errorf("project %s requires root privileges; run it with sudo", action)
				}
			}
			lifecycle := projectruntime.NewLifecycle(host.DefaultPaths("/"), runner)
			result, err := lifecycle.Control(cmd.Context(), loaded, action, opts.dryRun)
			if err != nil {
				return err
			}
			return writeControlResult(cmd.OutOrStdout(), result, opts)
		},
	}
}

func loadProjectArgument(args []string, environment string) (manifest.LoadedProject, error) {
	path := "."
	if len(args) == 1 {
		path = args[0]
	}
	return manifest.Load(path, environment)
}

func writeDeployResult(writer io.Writer, result projectruntime.DeployResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	mode := "deployed"
	if result.DryRun {
		mode = "deployment plan for"
	}
	if _, err := fmt.Fprintf(writer, "%s %s/%s (revision %.12s)\n", mode, result.Project, result.Environment, result.Revision); err != nil {
		return err
	}
	portKeys := make([]string, 0, len(result.Ports))
	for key := range result.Ports {
		portKeys = append(portKeys, key)
	}
	sort.Strings(portKeys)
	for _, key := range portKeys {
		if _, err := fmt.Fprintf(writer, "  port      %-24s 127.0.0.1:%d\n", key, result.Ports[key]); err != nil {
			return err
		}
	}
	for _, step := range result.Steps {
		detail := step.Path
		if len(step.Command) > 0 {
			detail = strings.Join(step.Command, " ")
		}
		if detail != "" {
			detail = " - " + detail
		}
		if _, err := fmt.Fprintf(writer, "  %-9s %s%s\n", step.Status, step.Name, detail); err != nil {
			return err
		}
	}
	return nil
}

func writeProjectStatus(writer io.Writer, result projectruntime.StatusResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	if !result.Deployed {
		_, err := fmt.Fprintf(writer, "project %s/%s is not deployed\n", result.Project, result.Environment)
		return err
	}
	if _, err := fmt.Fprintf(writer, "project %s/%s is %s (revision %.12s)\n", result.Project, result.Environment, result.Deployment.Status, result.Deployment.Revision); err != nil {
		return err
	}
	for _, container := range result.Containers {
		name := mapString(container, "Name", "Service")
		state := mapString(container, "State", "Status")
		health := mapString(container, "Health")
		if health != "" {
			state += ", " + health
		}
		if _, err := fmt.Fprintf(writer, "  %-28s %s\n", name, state); err != nil {
			return err
		}
	}
	return nil
}

func writeControlResult(writer io.Writer, result projectruntime.ControlResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	verb := result.Action + "ed"
	if result.Action == "stop" {
		verb = "stopped"
	}
	if result.DryRun {
		_, err := fmt.Fprintf(writer, "would run: %s\n", strings.Join(result.Command, " "))
		return err
	}
	_, err := fmt.Fprintf(writer, "%s project %s/%s\n", verb, result.Project, result.Environment)
	return err
}

func mapString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
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
