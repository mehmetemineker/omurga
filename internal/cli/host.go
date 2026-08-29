package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"omurga/internal/host"
)

func newHostCommand(opts *options) *cobra.Command {
	cmd := newGroupCommand("host", "Manage host connections and provisioning",
		newHostInitCommand(opts),
		newHostInstallCommand(opts),
		newHostDetectCommand(opts),
		newHostAddCommand(opts),
		newHostListCommand(opts),
		newHostShowCommand(opts),
		newHostRemoveCommand(opts),
		newDoctorCommand(opts, "status"),
		newHostUpdateCommand(opts),
		newDoctorCommand(opts, "doctor"),
	)
	return cmd
}

func newHostInitCommand(opts *options) *cobra.Command {
	var skipDocker bool
	var skipCaddy bool
	var skipRestic bool
	var replaceDockerConflicts bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Omurga and install Docker and Caddy on a supported Linux host",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}

			if !opts.dryRun {
				root, err := host.IsRoot(cmd.Context(), host.ExecRunner{})
				if err != nil {
					return fmt.Errorf("could not verify root privileges: %w", err)
				}
				if !root {
					return fmt.Errorf("host init requires root privileges; run it with sudo")
				}
			}

			paths := host.DefaultPaths("/")
			result, err := host.Initialize(paths, opts.dryRun)
			if err != nil {
				return err
			}

			installations, err := runHostInstallers(cmd.Context(), paths, result.OS, host.InstallOptions{
				DryRun:                 opts.dryRun,
				ReplaceDockerConflicts: replaceDockerConflicts,
			}, !skipDocker, !skipCaddy, !skipRestic)
			if err != nil {
				return err
			}
			return writeInitResult(cmd.OutOrStdout(), result, installations, opts)
		},
	}
	cmd.Flags().BoolVar(&skipDocker, "skip-docker", false, "do not install Docker")
	cmd.Flags().BoolVar(&skipCaddy, "skip-caddy", false, "do not install Caddy")
	cmd.Flags().BoolVar(&skipRestic, "skip-restic", false, "do not install Restic")
	cmd.Flags().BoolVar(&replaceDockerConflicts, "replace-conflicting-docker", false, "remove conflicting distribution Docker packages before installing Docker CE")
	return cmd
}

func newHostInstallCommand(opts *options) *cobra.Command {
	var replaceDockerConflicts bool
	cmd := &cobra.Command{
		Use:       "install [docker|caddy|restic|all]",
		Short:     "Install or repair host infrastructure components",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"docker", "caddy", "restic", "all"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			component := args[0]
			if component != "docker" && component != "caddy" && component != "restic" && component != "all" {
				return fmt.Errorf("component must be docker, caddy, restic, or all")
			}

			paths := host.DefaultPaths("/")
			release, _, _, err := host.DetectPlatform(paths.OSRelease)
			if err != nil {
				return err
			}
			if !opts.dryRun {
				root, err := host.IsRoot(cmd.Context(), host.ExecRunner{})
				if err != nil {
					return fmt.Errorf("could not verify root privileges: %w", err)
				}
				if !root {
					return fmt.Errorf("host install requires root privileges; run it with sudo")
				}
			}

			installations, err := runHostInstallers(cmd.Context(), paths, release, host.InstallOptions{
				DryRun:                 opts.dryRun,
				ReplaceDockerConflicts: replaceDockerConflicts,
			}, component == "docker" || component == "all", component == "caddy" || component == "all", component == "restic" || component == "all")
			if err != nil {
				return err
			}
			return writeInstallResults(cmd.OutOrStdout(), installations, opts)
		},
	}
	cmd.Flags().BoolVar(&replaceDockerConflicts, "replace-conflicting-docker", false, "remove conflicting distribution Docker packages before installing Docker CE")
	return cmd
}

func newHostUpdateCommand(opts *options) *cobra.Command {
	var full bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update package indexes and upgrade installed packages",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			paths := host.DefaultPaths("/")
			_, provider, _, err := host.DetectPlatform(paths.OSRelease)
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
					return fmt.Errorf("host update requires root privileges; run it with sudo")
				}
			}

			result, err := host.UpdatePackages(cmd.Context(), runner, provider, full, opts.dryRun)
			if err != nil {
				return err
			}
			return writeUpdateResult(cmd.OutOrStdout(), result, opts)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "use the provider's full distribution upgrade mode")
	return cmd
}

func newHostDetectCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Detect the Linux distribution and selected provider",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			release, _, platform, err := host.DetectPlatform(host.DefaultPaths("/").OSRelease)
			if err != nil {
				return err
			}
			result := struct {
				OS       host.OSRelease    `json:"os"`
				Platform host.PlatformInfo `json:"platform"`
			}{release, platform}
			if opts.quiet {
				return nil
			}
			if opts.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s (%s)\n  family: %s\n  package manager: %s\n  service manager: %s\n  support: %s\n", strings.ToUpper(platform.Distribution[:1])+platform.Distribution[1:], platform.Version, platform.Codename, platform.Family, platform.PackageManager, platform.ServiceManager, platform.SupportLevel)
			return err
		},
	}
}

func newDoctorCommand(opts *options, use string) *cobra.Command {
	short := "Run health checks on the active host"
	if use == "status" {
		short = "Show host status"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			report := host.RunDoctor(cmd.Context(), host.DefaultPaths("/"), host.ExecRunner{})
			if err := writeDoctorReport(cmd.OutOrStdout(), report, opts); err != nil {
				return err
			}
			if code := report.ExitCode(); code != 0 {
				return newSilentExitError(code)
			}
			return nil
		},
	}
}

func writeInitResult(writer io.Writer, result host.InitResult, installations []host.InstallResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(struct {
			Initialization host.InitResult      `json:"initialization"`
			Installations  []host.InstallResult `json:"installations"`
		}{result, installations})
	}

	mode := "initialized"
	if result.DryRun {
		mode = "initialization plan for"
	}
	if _, err := fmt.Fprintf(writer, "%s %s (%s/%s)\n", mode, result.OS.PrettyName, result.Platform.PackageManager, result.Platform.ServiceManager); err != nil {
		return err
	}
	for _, action := range result.Actions {
		status := "unchanged"
		if action.Changed && result.DryRun {
			status = "would create or update"
		} else if action.Changed {
			status = "created or updated"
		}
		if _, err := fmt.Fprintf(writer, "  %-24s %s (%s)\n", status, action.Path, action.ModeOct); err != nil {
			return err
		}
	}
	return writeInstallResults(writer, installations, &options{quiet: opts.quiet})
}

func writeInstallResults(writer io.Writer, results []host.InstallResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(struct {
			Installations []host.InstallResult `json:"installations"`
		}{results})
	}
	for _, result := range results {
		state := "completed"
		if result.AlreadyInstalled {
			state = "already installed"
		} else if result.DryRun {
			state = "installation plan"
		}
		if _, err := fmt.Fprintf(writer, "\n%s: %s\n", result.Component, state); err != nil {
			return err
		}
		for _, step := range result.Steps {
			detail := step.Path
			if len(step.Command) > 0 {
				detail = strings.Join(step.Command, " ")
			}
			if detail != "" {
				detail = " - " + detail
			}
			if _, err := fmt.Fprintf(writer, "  %-10s %s%s\n", step.Status, step.Name, detail); err != nil {
				return err
			}
		}
	}
	return nil
}

func runHostInstallers(ctx context.Context, paths host.Paths, release host.OSRelease, options host.InstallOptions, installDocker, installCaddy, installRestic bool) ([]host.InstallResult, error) {
	installer := host.NewInstaller(paths)
	results := make([]host.InstallResult, 0, 3)
	if installDocker {
		result, err := installer.InstallDocker(ctx, release, options)
		if err != nil {
			return results, fmt.Errorf("Docker installation failed: %w", err)
		}
		results = append(results, result)
	}
	if installCaddy {
		result, err := installer.InstallCaddy(ctx, release, options)
		if err != nil {
			return results, fmt.Errorf("Caddy installation failed: %w", err)
		}
		results = append(results, result)
	}
	if installRestic {
		result, err := installer.InstallRestic(ctx, release, options)
		if err != nil {
			return results, fmt.Errorf("Restic installation failed: %w", err)
		}
		results = append(results, result)
	}
	return results, nil
}

func writeUpdateResult(writer io.Writer, result host.UpdateResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	if result.DryRun {
		if _, err := fmt.Fprintf(writer, "%s %s update plan:\n", strings.ToUpper(result.PackageManager), result.Mode); err != nil {
			return err
		}
		for _, command := range result.Commands {
			if _, err := fmt.Fprintf(writer, "  %s %s\n", command.Name, strings.Join(command.Args, " ")); err != nil {
				return err
			}
		}
		return nil
	}
	_, err := fmt.Fprintf(writer, "%s %s update completed\n", strings.ToUpper(result.PackageManager), result.Mode)
	return err
}

func writeDoctorReport(writer io.Writer, report host.DoctorReport, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(report)
	}

	labels := map[host.CheckStatus]string{
		host.CheckPass:     "PASS",
		host.CheckWarning:  "WARN",
		host.CheckCritical: "FAIL",
	}
	for _, check := range report.Checks {
		if _, err := fmt.Fprintf(writer, "[%s] %-20s %s\n", labels[check.Status], check.Name, check.Message); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(writer, "\n%d passed, %d warnings, %d critical\n", report.Summary.Passed, report.Summary.Warnings, report.Summary.Critical)
	return err
}

func requireLocalHost(name string) error {
	if name != "" && name != "local" {
		return fmt.Errorf("remote host %q is not supported yet", name)
	}
	return nil
}
