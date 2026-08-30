package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"omurga/internal/gateway"
	"omurga/internal/host"
	"omurga/internal/manifest"
	"omurga/internal/progress"
	projectruntime "omurga/internal/project"
	"omurga/internal/state"
)

func newProjectCommand(opts *options) *cobra.Command {
	cmd := newGroupCommand("project", "Manage Docker projects",
		newProjectCreateCommand(opts),
		newProjectRenderCommand(opts),
		newProjectDeployCommand(opts),
		newProjectRepairCommand(opts),
		newProjectExecCommand(opts),
		newProjectShellCommand(opts),
		newProjectInspectCommand(opts),
		newProjectDiffCommand(opts),
		newProjectStatusCommand(opts),
		newProjectControlCommand(opts, "restart"),
		newProjectControlCommand(opts, "stop"),
		newProjectLogsCommand(opts),
		newProjectRollbackCommand(opts),
		newProjectDeleteCommand(opts),
		newProjectListCommand(opts),
		newProjectShowCommand(opts),
	)
	cmd.AddCommand(newProjectValidateCommand(opts))
	return cmd
}

func newProjectExecCommand(opts *options) *cobra.Command {
	return newProjectExecLikeCommand(opts, false)
}

func newProjectShellCommand(opts *options) *cobra.Command {
	return newProjectExecLikeCommand(opts, true)
}

func newProjectExecLikeCommand(opts *options, shell bool) *cobra.Command {
	use := "exec <project-directory-or-manifest> <service> [command...]"
	short := "Run a command in an active project service"
	if shell {
		use = "shell <project-directory-or-manifest> <service>"
		short = "Open an interactive shell in an active project service"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			loaded, err := loadProjectArgument([]string{args[0]}, opts.environment)
			if err != nil {
				return err
			}
			runner := host.ExecRunner{}
			if !opts.dryRun {
				if err := requireRoot(cmd.Context(), runner, "project "+map[bool]string{true: "shell", false: "exec"}[shell]); err != nil {
					return err
				}
			}
			command := args[2:]
			if shell {
				command = []string{"/bin/sh"}
			}
			lifecycle := projectruntime.NewLifecycle(host.DefaultPaths("/"), runner)
			result, err := lifecycle.Exec(cmd.Context(), loaded, args[1], projectruntime.ExecOptions{
				TTY: shell, Command: command,
			}, opts.dryRun, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if opts.dryRun {
				return writeExecResult(cmd.OutOrStdout(), result, opts)
			}
			return nil
		},
	}
}

type projectInspectService struct {
	Name        string `json:"name"`
	Image       string `json:"image"`
	Expose      []int  `json:"expose,omitempty"`
	Volumes     int    `json:"volumeCount"`
	Secrets     int    `json:"secretCount"`
	Healthcheck bool   `json:"healthcheck"`
}

type projectInspectResult struct {
	Project     string                          `json:"project"`
	Environment string                          `json:"environment"`
	Manifest    string                          `json:"manifest"`
	Deployed    bool                            `json:"deployed"`
	Deployment  *state.Deployment               `json:"deployment,omitempty"`
	Layout      projectruntime.DeploymentLayout `json:"layout"`
	Services    []projectInspectService         `json:"services"`
	Routes      []manifest.Route                `json:"routes,omitempty"`
}

func newProjectInspectCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect [project-directory-or-manifest]",
		Short: "Inspect project configuration and recorded deployment state",
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
			layout, deployment, deployed, err := lifecycle.CurrentLayout(cmd.Context(), loaded)
			if err != nil {
				return err
			}
			services := make([]projectInspectService, 0, len(loaded.Project.Services))
			serviceNames := make([]string, 0, len(loaded.Project.Services))
			for name := range loaded.Project.Services {
				serviceNames = append(serviceNames, name)
			}
			sort.Strings(serviceNames)
			for _, name := range serviceNames {
				service := loaded.Project.Services[name]
				services = append(services, projectInspectService{
					Name: name, Image: service.Image, Expose: service.Expose,
					Volumes: len(service.Volumes), Secrets: len(service.Secrets),
					Healthcheck: len(service.Healthcheck.Command) > 0,
				})
			}
			result := projectInspectResult{
				Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment),
				Manifest: loaded.Path, Deployed: deployed, Layout: layout,
				Services: services, Routes: loaded.Project.Gateway.Routes,
			}
			if deployed {
				result.Deployment = &deployment
			}
			return writeProjectInspect(cmd.OutOrStdout(), result, opts)
		},
	}
}

type projectDiffResult struct {
	Project         string `json:"project"`
	Environment     string `json:"environment"`
	Deployed        bool   `json:"deployed"`
	Changed         bool   `json:"changed"`
	DesiredRevision string `json:"desiredRevision"`
	CurrentRevision string `json:"currentRevision,omitempty"`
	Manifest        string `json:"manifest"`
}

func newProjectDiffCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "diff [project-directory-or-manifest]",
		Short: "Compare the desired project revision with the active deployment",
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
			desired, err := lifecycle.Deploy(cmd.Context(), loaded, true)
			if err != nil {
				return err
			}
			deployment, deployed, err := lifecycle.CurrentDeployment(cmd.Context(), loaded)
			if err != nil {
				return err
			}
			result := projectDiffResult{
				Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment),
				Deployed: deployed, Changed: !deployed || desired.Revision != deployment.Revision,
				DesiredRevision: desired.Revision, Manifest: loaded.Path,
			}
			if deployed {
				result.CurrentRevision = deployment.Revision
			}
			return writeProjectDiff(cmd.OutOrStdout(), result, opts)
		},
	}
}

func newProjectRepairCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "repair [project-directory-or-manifest]",
		Short: "Reconcile missing project containers and gateway state",
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
				if err := requireRoot(cmd.Context(), runner, "project repair"); err != nil {
					return err
				}
			}
			lifecycle, err := platformLifecycle(host.DefaultPaths("/"), runner, opts.dryRun, progress.FromContext(cmd.Context()))
			if err != nil {
				return err
			}
			result, err := lifecycle.Deploy(cmd.Context(), loaded, opts.dryRun)
			if err != nil {
				return err
			}
			return writeRepairResult(cmd.OutOrStdout(), result, opts)
		},
	}
}

func newProjectListCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List deployed projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			paths := host.DefaultPaths("/")
			deployments := []state.Deployment{}
			if _, err := os.Stat(paths.StateDB); err == nil {
				store, err := state.OpenReadOnly(cmd.Context(), paths.StateDB)
				if err != nil {
					return err
				}
				defer store.Close()
				deployments, err = store.ListDeployments(cmd.Context())
				if err != nil {
					return err
				}
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("could not inspect state database: %w", err)
			}
			if opts.quiet {
				return nil
			}
			if opts.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(deployments)
			}
			for _, deployment := range deployments {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-16s %-10s %.12s\n", deployment.Project, deployment.Environment, deployment.Status, deployment.Revision); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newProjectShowCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "show [project-directory-or-manifest]",
		Short: "Show the resolved project manifest",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loaded, err := loadProjectArgument(args, opts.environment)
			if err != nil {
				return err
			}
			if opts.quiet {
				return nil
			}
			result := struct {
				Manifest    string           `json:"manifest"`
				Environment string           `json:"environment"`
				Project     manifest.Project `json:"project"`
			}{loaded.Path, gateway.EnvironmentKey(loaded.Environment), loaded.Project}
			if opts.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			content, err := yaml.Marshal(loaded.Project)
			if err != nil {
				return fmt.Errorf("could not encode resolved project: %w", err)
			}
			_, err = cmd.OutOrStdout().Write(content)
			return err
		},
	}
}

func newProjectLogsCommand(opts *options) *cobra.Command {
	var follow bool
	var tail string
	var since string
	var timestamps bool
	var services []string
	cmd := &cobra.Command{
		Use:   "logs [project-directory-or-manifest]",
		Short: "Stream or inspect project service logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			if opts.json && !opts.dryRun {
				return fmt.Errorf("--json is supported for project logs only with --dry-run")
			}
			loaded, err := loadProjectArgument(args, opts.environment)
			if err != nil {
				return err
			}
			lifecycle := projectruntime.NewLifecycle(host.DefaultPaths("/"), host.ExecRunner{})
			result, err := lifecycle.Logs(cmd.Context(), loaded, projectruntime.LogOptions{
				Follow: follow, Tail: tail, Since: since, Timestamps: timestamps, Services: services,
			}, opts.dryRun, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if opts.dryRun {
				return writeLogPlan(cmd.OutOrStdout(), result, opts)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	cmd.Flags().StringVar(&tail, "tail", "100", "number of lines to show from the end of the logs, or all")
	cmd.Flags().StringVar(&since, "since", "", "show logs since a duration or timestamp accepted by Docker")
	cmd.Flags().BoolVarP(&timestamps, "timestamps", "t", false, "show timestamps")
	cmd.Flags().StringSliceVar(&services, "service", nil, "limit logs to one or more project services")
	return cmd
}

func newProjectRollbackCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "rollback [project-directory-or-manifest]",
		Short: "Switch to the previous healthy deployment artifacts",
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
				if err := requireRoot(cmd.Context(), runner, "project rollback"); err != nil {
					return err
				}
			}
			lifecycle, err := platformLifecycle(host.DefaultPaths("/"), runner, opts.dryRun, progress.FromContext(cmd.Context()))
			if err != nil {
				return err
			}
			result, err := lifecycle.Rollback(cmd.Context(), loaded, opts.dryRun)
			if err != nil {
				return err
			}
			return writeRollbackResult(cmd.OutOrStdout(), result, opts)
		},
	}
}

func newProjectDeleteCommand(opts *options) *cobra.Command {
	var purgeData bool
	cmd := &cobra.Command{
		Use:   "delete [project-directory-or-manifest]",
		Short: "Remove a deployed project while preserving persistent data by default",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			if purgeData && !opts.dryRun && !opts.yes {
				return fmt.Errorf("--purge-data requires --yes because persistent project data will be permanently deleted")
			}
			loaded, err := loadProjectArgument(args, opts.environment)
			if err != nil {
				return err
			}
			runner := host.ExecRunner{}
			if !opts.dryRun {
				if err := requireRoot(cmd.Context(), runner, "project delete"); err != nil {
					return err
				}
			}
			lifecycle, err := platformLifecycle(host.DefaultPaths("/"), runner, opts.dryRun, progress.FromContext(cmd.Context()))
			if err != nil {
				return err
			}
			result, err := lifecycle.Delete(cmd.Context(), loaded, projectruntime.DeleteOptions{
				DryRun: opts.dryRun, PurgeData: purgeData,
			})
			if err != nil {
				return err
			}
			return writeDeleteResult(cmd.OutOrStdout(), result, opts)
		},
	}
	cmd.Flags().BoolVar(&purgeData, "purge-data", false, "permanently remove persistent project data")
	return cmd
}

func requireRoot(ctx context.Context, runner host.Runner, operation string) error {
	root, err := host.IsRoot(ctx, runner)
	if err != nil {
		return fmt.Errorf("could not verify root privileges: %w", err)
	}
	if !root {
		return fmt.Errorf("%s requires root privileges; run it with sudo", operation)
	}
	return nil
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
			lifecycle, err := platformLifecycle(host.DefaultPaths("/"), runner, opts.dryRun, progress.FromContext(cmd.Context()))
			if err != nil {
				return err
			}
			result, err := lifecycle.Deploy(cmd.Context(), loaded, opts.dryRun)
			if err != nil {
				return errors.Join(err, notifyProjectEvent(cmd.Context(), loaded, "deploy-failed", err))
			}
			return writeDeployResult(cmd.OutOrStdout(), result, opts)
		},
	}
	return cmd
}

func platformLifecycle(paths host.Paths, runner host.Runner, dryRun bool, reporter *progress.Reporter) (projectruntime.Lifecycle, error) {
	lifecycle := projectruntime.NewLifecycle(paths, runner).WithProgress(reporter)
	if dryRun {
		return lifecycle, nil
	}
	services, err := host.DetectServiceManager(paths.OSRelease)
	if err != nil {
		return projectruntime.Lifecycle{}, err
	}
	return lifecycle.WithServiceManager(services), nil
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
			lifecycle := projectruntime.NewLifecycle(host.DefaultPaths("/"), runner).WithProgress(progress.FromContext(cmd.Context()))
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

func writeExecResult(writer io.Writer, result projectruntime.ExecResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	_, err := fmt.Fprintf(writer, "would run in %s/%s (%s): %s\n", result.Project, result.Environment, result.Service, strings.Join(result.Command, " "))
	return err
}

func writeProjectInspect(writer io.Writer, result projectInspectResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	if _, err := fmt.Fprintf(writer, "project %s/%s\n", result.Project, result.Environment); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  manifest   %s\n", result.Manifest); err != nil {
		return err
	}
	stateLabel := "not deployed"
	if result.Deployed && result.Deployment != nil {
		stateLabel = fmt.Sprintf("%s (revision %.12s)", result.Deployment.Status, result.Deployment.Revision)
	}
	if _, err := fmt.Fprintf(writer, "  deployment  %s\n", stateLabel); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  compose     %s\n  caddy       %s\n", result.Layout.Compose, result.Layout.Caddy); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(writer, "  services:"); err != nil {
		return err
	}
	for _, service := range result.Services {
		health := "no healthcheck"
		if service.Healthcheck {
			health = "healthcheck"
		}
		if _, err := fmt.Fprintf(writer, "    %-16s %-36s %s\n", service.Name, service.Image, health); err != nil {
			return err
		}
	}
	for _, route := range result.Routes {
		if _, err := fmt.Fprintf(writer, "  route       %s -> %s:%d\n", route.Domain, route.Service, route.Port); err != nil {
			return err
		}
	}
	return nil
}

func writeProjectDiff(writer io.Writer, result projectDiffResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	if !result.Deployed {
		_, err := fmt.Fprintf(writer, "project %s/%s is not deployed; deployment required\n", result.Project, result.Environment)
		return err
	}
	if !result.Changed {
		_, err := fmt.Fprintf(writer, "project %s/%s is up to date (revision %.12s)\n", result.Project, result.Environment, result.CurrentRevision)
		return err
	}
	_, err := fmt.Fprintf(writer, "project %s/%s has changes\n  current  %.12s\n  desired  %.12s\n", result.Project, result.Environment, result.CurrentRevision, result.DesiredRevision)
	return err
}

func writeRepairResult(writer io.Writer, result projectruntime.DeployResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	mode := "repaired"
	if result.DryRun {
		mode = "repair plan for"
	}
	if _, err := fmt.Fprintf(writer, "%s %s/%s (revision %.12s)\n", mode, result.Project, result.Environment, result.Revision); err != nil {
		return err
	}
	return writeLifecycleSteps(writer, result.Steps)
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

func writeLogPlan(writer io.Writer, result projectruntime.LogResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	_, err := fmt.Fprintf(writer, "would run: %s\n", strings.Join(result.Command, " "))
	return err
}

func writeRollbackResult(writer io.Writer, result projectruntime.RollbackResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	mode := "rolled back"
	if result.DryRun {
		mode = "rollback plan for"
	}
	if _, err := fmt.Fprintf(writer, "%s %s/%s (%.12s -> %.12s)\n", mode, result.Project, result.Environment, result.FromRevision, result.ToRevision); err != nil {
		return err
	}
	return writeLifecycleSteps(writer, result.Steps)
}

func writeDeleteResult(writer io.Writer, result projectruntime.DeleteResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	mode := "deleted"
	if result.DryRun {
		mode = "deletion plan for"
	}
	if _, err := fmt.Fprintf(writer, "%s %s/%s\n", mode, result.Project, result.Environment); err != nil {
		return err
	}
	if result.DataPreserved {
		if _, err := fmt.Fprintf(writer, "  persistent data will be preserved at %s\n", result.DataPath); err != nil {
			return err
		}
	}
	return writeLifecycleSteps(writer, result.Steps)
}

func writeLifecycleSteps(writer io.Writer, steps []projectruntime.LifecycleStep) error {
	for _, step := range steps {
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
