package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	backupengine "omurga/internal/backup"
	"omurga/internal/gateway"
	"omurga/internal/host"
	"omurga/internal/manifest"
	"omurga/internal/progress"
	projectruntime "omurga/internal/project"
	"omurga/internal/secret"
)

type backupFlags struct {
	repository      string
	passwordFile    string
	environmentFile string
}

type backupResult struct {
	Project     string   `json:"project"`
	Environment string   `json:"environment"`
	Action      string   `json:"action"`
	Command     []string `json:"command,omitempty"`
	Output      string   `json:"output,omitempty"`
	Paths       []string `json:"paths,omitempty"`
	DryRun      bool     `json:"dryRun,omitempty"`
}

func newBackupCommand(opts *options) *cobra.Command {
	return newGroupCommand("backup", "Manage Restic backups and restores",
		newBackupCreateCommand(opts),
		newBackupRepositoryCommand(opts, "list"),
		newBackupRepositoryCommand(opts, "show"),
		newBackupRepositoryCommand(opts, "check"),
		newBackupRestoreCommand(opts),
		newBackupPruneCommand(opts),
		newBackupScheduleCommand(opts),
		newBackupUnscheduleCommand(opts),
	)
}

func addBackupFlags(cmd *cobra.Command, flags *backupFlags) {
	cmd.Flags().StringVar(&flags.repository, "repository", "", "Restic repository URI; defaults to backup.destination")
	cmd.Flags().StringVar(&flags.passwordFile, "password-file", "", "Restic repository password file")
	cmd.Flags().StringVar(&flags.environmentFile, "environment-file", "", "root-only environment file for backend credentials")
}

func resolveBackup(loaded manifest.LoadedProject, flags backupFlags) (backupengine.Manager, error) {
	repository := flags.repository
	if repository == "" {
		repository = loaded.Project.Backup.Destination
	}
	if repository == "" {
		return backupengine.Manager{}, fmt.Errorf("backup repository is required; set backup.destination or --repository")
	}
	passwordFile := flags.passwordFile
	if passwordFile == "" {
		name := safeUnitName(repository)
		passwordFile = filepath.Join(host.DefaultPaths("/").BackupConfig, name+".password")
	}
	return backupengine.Manager{Runner: host.ExecRunner{}, Repository: repository, PasswordFile: passwordFile, EnvironmentFile: flags.environmentFile}, nil
}

func newBackupCreateCommand(opts *options) *cobra.Command {
	var flags backupFlags
	var initialize bool
	cmd := &cobra.Command{Use: "create [project-directory-or-manifest]", Short: "Create database dumps and a Restic snapshot", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLocalHost(opts.host); err != nil {
			return err
		}
		loaded, err := loadProjectArgument(args, opts.environment)
		if err != nil {
			return err
		}
		manager, err := resolveBackup(loaded, flags)
		if err != nil {
			return err
		}
		manager.Progress = progress.FromContext(cmd.Context())
		environment := gateway.EnvironmentKey(loaded.Environment)
		paths := host.DefaultPaths("/")
		layout := projectruntime.NewLifecycle(paths, host.ExecRunner{}).Layout(loaded.Project.Name, environment)
		planned := existingBackupPaths(loaded, paths, layout)
		command := manager.Command("backup", append([]string{"--tag", "omurga", "--tag", "project=" + loaded.Project.Name, "--tag", "environment=" + environment}, planned...)...)
		result := backupResult{Project: loaded.Project.Name, Environment: environment, Action: "create", Command: append([]string{"restic"}, command...), Paths: planned, DryRun: opts.dryRun}
		if opts.dryRun {
			return writeBackupResult(cmd.OutOrStdout(), result, opts)
		}
		if err := requireRoot(cmd.Context(), host.ExecRunner{}, "backup create"); err != nil {
			return err
		}
		if _, err := (host.ExecRunner{}).LookPath("restic"); err != nil {
			return fmt.Errorf("Restic is required for backups")
		}
		if initialize {
			if _, err := manager.Run(cmd.Context(), "init"); err != nil {
				return fmt.Errorf("could not initialize Restic repository: %w", err)
			}
		}
		staging, err := prepareBackupStaging(cmd, loaded, layout)
		if err != nil {
			return err
		}
		if staging != "" {
			defer os.RemoveAll(staging)
			planned = append(planned, staging)
		}
		output, err := manager.Run(cmd.Context(), "backup", append([]string{"--tag", "omurga", "--tag", "project=" + loaded.Project.Name, "--tag", "environment=" + environment}, planned...)...)
		if err != nil {
			backupErr := fmt.Errorf("backup did not reach the Restic repository: %w", err)
			return errors.Join(backupErr, notifyProjectEvent(cmd.Context(), loaded, "backup-failed", backupErr))
		}
		result.Output = output
		result.Paths = planned
		return writeBackupResult(cmd.OutOrStdout(), result, opts)
	}}
	addBackupFlags(cmd, &flags)
	cmd.Flags().BoolVar(&initialize, "init", false, "initialize an empty Restic repository before backing up")
	return cmd
}

func existingBackupPaths(loaded manifest.LoadedProject, paths host.Paths, layout projectruntime.DeploymentLayout) []string {
	candidates := []string{loaded.Path, layout.Compose, layout.Caddy, filepath.Join(layout.Root, "data"), paths.StateDB}
	if secretPath, err := secret.NewManager(paths).StorePath(loaded.Project.Name, gateway.EnvironmentKey(loaded.Environment)); err == nil {
		candidates = append(candidates, secretPath)
	}
	var result []string
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			result = append(result, candidate)
		}
	}
	return result
}

func prepareBackupStaging(cmd *cobra.Command, loaded manifest.LoadedProject, layout projectruntime.DeploymentLayout) (string, error) {
	if len(loaded.Project.Backup.Include.Postgres) == 0 && len(loaded.Project.Backup.Include.Redis) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(host.DefaultPaths("/").BackupStaging, 0o700); err != nil {
		return "", fmt.Errorf("could not create backup staging root: %w", err)
	}
	root, err := os.MkdirTemp(host.DefaultPaths("/").BackupStaging, loaded.Project.Name+"-*")
	if err != nil {
		return "", fmt.Errorf("could not create backup staging directory: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return "", err
	}
	compose := []string{"compose", "--project-name", projectruntime.ComposeProjectName(loaded.Project.Name, gateway.EnvironmentKey(loaded.Environment)), "--file", layout.Compose}
	for _, name := range loaded.Project.Backup.Include.Postgres {
		dependency, exists := loaded.Project.Dependencies[name]
		if !exists || dependency.Type != "postgres" || dependency.Mode == "shared" {
			return root, fmt.Errorf("backup PostgreSQL selection %s is not a project-scoped instance", name)
		}
		path := filepath.Join(root, name+".dump")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return root, err
		}
		args := append(compose, "exec", "-T", name, "pg_dump", "-U", dependency.User, "-d", dependency.Database, "--format=custom")
		task := progress.FromContext(cmd.Context()).Start("Create PostgreSQL dump for " + name)
		runErr := (host.ExecRunner{}).RunIO(cmd.Context(), nil, file, cmd.ErrOrStderr(), "docker", args...)
		closeErr := file.Close()
		if runErr != nil || closeErr != nil {
			task.Fail(errors.Join(runErr, closeErr))
			return root, errors.Join(runErr, closeErr)
		}
		task.Complete()
	}
	for _, name := range loaded.Project.Backup.Include.Redis {
		dependency, exists := loaded.Project.Dependencies[name]
		if !exists || dependency.Type != "redis" || dependency.Mode == "shared" {
			return root, fmt.Errorf("backup Redis selection %s is not a project-scoped instance", name)
		}
		task := progress.FromContext(cmd.Context()).Start("Create Redis snapshot for " + name)
		if _, err := (host.ExecRunner{}).Run(cmd.Context(), "docker", append(compose, "exec", "-T", name, "redis-cli", "SAVE")...); err != nil {
			task.Fail(err)
			return root, err
		}
		if _, err := (host.ExecRunner{}).Run(cmd.Context(), "docker", append(compose, "cp", name+":/data/dump.rdb", filepath.Join(root, name+".rdb"))...); err != nil {
			task.Fail(err)
			return root, err
		}
		task.Complete()
	}
	return root, nil
}

func newBackupRepositoryCommand(opts *options, action string) *cobra.Command {
	var flags backupFlags
	use := action + " [project-directory-or-manifest]"
	if action == "show" {
		use = "show [snapshot] [project-directory-or-manifest]"
	}
	cmd := &cobra.Command{Use: use, Short: map[string]string{"list": "List Restic snapshots", "show": "Show snapshot details", "check": "Check backup repository integrity"}[action], Args: func(cmd *cobra.Command, args []string) error {
		if action == "show" {
			return cobra.RangeArgs(1, 2)(cmd, args)
		}
		return cobra.MaximumNArgs(1)(cmd, args)
	}, RunE: func(cmd *cobra.Command, args []string) error {
		projectArgs := args
		if action == "show" {
			projectArgs = args[1:]
		}
		loaded, err := loadProjectArgument(projectArgs, opts.environment)
		if err != nil {
			return err
		}
		manager, err := resolveBackup(loaded, flags)
		if err != nil {
			return err
		}
		manager.Progress = progress.FromContext(cmd.Context())
		resticAction := "snapshots"
		arguments := []string{"--tag", "project=" + loaded.Project.Name, "--tag", "environment=" + gateway.EnvironmentKey(loaded.Environment), "--json"}
		if action == "show" {
			arguments = []string{args[0], "--json"}
		}
		if action == "check" {
			resticAction, arguments = "check", nil
		}
		result := backupResult{Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment), Action: action, Command: append([]string{"restic"}, manager.Command(resticAction, arguments...)...), DryRun: opts.dryRun}
		if !opts.dryRun {
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "backup "+action); err != nil {
				return err
			}
			result.Output, err = manager.Run(cmd.Context(), resticAction, arguments...)
			if err != nil {
				return err
			}
		}
		return writeBackupResult(cmd.OutOrStdout(), result, opts)
	}}
	addBackupFlags(cmd, &flags)
	return cmd
}

func newBackupRestoreCommand(opts *options) *cobra.Command {
	var flags backupFlags
	var target string
	cmd := &cobra.Command{Use: "restore [snapshot] [project-directory-or-manifest]", Short: "Restore a snapshot into a staging target", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadProjectArgument(args[1:], opts.environment)
		if err != nil {
			return err
		}
		manager, err := resolveBackup(loaded, flags)
		if err != nil {
			return err
		}
		manager.Progress = progress.FromContext(cmd.Context())
		if target == "" {
			target = filepath.Join(host.DefaultPaths("/").BackupStaging, "restore-"+loaded.Project.Name+"-"+time.Now().UTC().Format("20060102T150405Z"))
		}
		arguments := []string{args[0], "--target", target}
		result := backupResult{Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment), Action: "restore", Command: append([]string{"restic"}, manager.Command("restore", arguments...)...), Paths: []string{target}, DryRun: opts.dryRun}
		if !opts.dryRun {
			if !opts.yes {
				return fmt.Errorf("backup restore requires --yes because existing files in the target may be replaced")
			}
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "backup restore"); err != nil {
				return err
			}
			result.Output, err = manager.Run(cmd.Context(), "restore", arguments...)
			if err != nil {
				return err
			}
		}
		return writeBackupResult(cmd.OutOrStdout(), result, opts)
	}}
	addBackupFlags(cmd, &flags)
	cmd.Flags().StringVar(&target, "target", "", "restore target directory")
	return cmd
}

func newBackupPruneCommand(opts *options) *cobra.Command {
	var flags backupFlags
	cmd := &cobra.Command{Use: "prune [project-directory-or-manifest]", Short: "Apply the retention policy and prune unused data", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadProjectArgument(args, opts.environment)
		if err != nil {
			return err
		}
		manager, err := resolveBackup(loaded, flags)
		if err != nil {
			return err
		}
		manager.Progress = progress.FromContext(cmd.Context())
		retention := loaded.Project.Backup.Retention
		if retention.Daily == 0 && retention.Weekly == 0 && retention.Monthly == 0 {
			retention.Daily, retention.Weekly, retention.Monthly = 7, 4, 6
		}
		arguments := backupengine.RetentionArguments(retention.Daily, retention.Weekly, retention.Monthly)
		arguments = append(arguments, "--tag", "project="+loaded.Project.Name, "--tag", "environment="+gateway.EnvironmentKey(loaded.Environment), "--prune")
		result := backupResult{Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment), Action: "prune", Command: append([]string{"restic"}, manager.Command("forget", arguments...)...), DryRun: opts.dryRun}
		if !opts.dryRun {
			if !opts.yes {
				return fmt.Errorf("backup prune requires --yes because snapshots and repository data may be permanently removed")
			}
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "backup prune"); err != nil {
				return err
			}
			result.Output, err = manager.Run(cmd.Context(), "forget", arguments...)
			if err != nil {
				return err
			}
		}
		return writeBackupResult(cmd.OutOrStdout(), result, opts)
	}}
	addBackupFlags(cmd, &flags)
	return cmd
}

func newBackupScheduleCommand(opts *options) *cobra.Command {
	var flags backupFlags
	var calendar string
	cmd := &cobra.Command{Use: "schedule [project-directory-or-manifest]", Short: "Enable a systemd backup timer", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadProjectArgument(args, opts.environment)
		if err != nil {
			return err
		}
		manager, err := resolveBackup(loaded, flags)
		if err != nil {
			return err
		}
		manager.Progress = progress.FromContext(cmd.Context())
		if calendar == "" {
			calendar = loaded.Project.Backup.Schedule
		}
		calendar, err = backupengine.ParseCalendar(calendar)
		if err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		name := safeUnitName(loaded.Project.Name + "-" + gateway.EnvironmentKey(loaded.Environment))
		paths := backupengine.SchedulePaths(host.DefaultPaths("/"), name)
		result := backupResult{Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment), Action: "schedule", Paths: paths, DryRun: opts.dryRun}
		if !opts.dryRun {
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "backup schedule"); err != nil {
				return err
			}
			services, err := host.DetectServiceManager(host.DefaultPaths("/").OSRelease)
			if err != nil {
				return err
			}
			if services.Name() != "systemd" {
				return fmt.Errorf("backup scheduling currently requires systemd; detected %s", services.Name())
			}
			if err := manager.ValidateCredentials(); err != nil {
				return err
			}
			paths, err = backupengine.WriteSchedule(host.DefaultPaths("/"), backupengine.Schedule{Name: name, Executable: executable, Manifest: loaded.Path, Environment: loaded.Environment, Repository: manager.Repository, PasswordFile: manager.PasswordFile, EnvironmentFile: manager.EnvironmentFile, Calendar: calendar})
			if err != nil {
				return err
			}
			result.Paths = paths
			daemonReload := services.DaemonReloadCommand()
			if _, err := (host.ExecRunner{}).Run(cmd.Context(), daemonReload.Name, daemonReload.Args...); err != nil {
				return err
			}
			enable := services.EnableNowCommand("omurga-backup-" + name + ".timer")
			if _, err := (host.ExecRunner{}).Run(cmd.Context(), enable.Name, enable.Args...); err != nil {
				return err
			}
		}
		return writeBackupResult(cmd.OutOrStdout(), result, opts)
	}}
	addBackupFlags(cmd, &flags)
	cmd.Flags().StringVar(&calendar, "calendar", "", "HH:MM or systemd calendar expression; defaults to backup.schedule")
	return cmd
}

func newBackupUnscheduleCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "unschedule [project-directory-or-manifest]", Short: "Disable and remove a systemd backup timer", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadProjectArgument(args, opts.environment)
		if err != nil {
			return err
		}
		name := safeUnitName(loaded.Project.Name + "-" + gateway.EnvironmentKey(loaded.Environment))
		paths := backupengine.SchedulePaths(host.DefaultPaths("/"), name)
		result := backupResult{Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment), Action: "unschedule", Paths: paths, DryRun: opts.dryRun}
		if !opts.dryRun {
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "backup unschedule"); err != nil {
				return err
			}
			services, err := host.DetectServiceManager(host.DefaultPaths("/").OSRelease)
			if err != nil {
				return err
			}
			if services.Name() != "systemd" {
				return fmt.Errorf("backup scheduling currently requires systemd; detected %s", services.Name())
			}
			disable := services.DisableNowCommand("omurga-backup-" + name + ".timer")
			_, _ = (host.ExecRunner{}).Run(cmd.Context(), disable.Name, disable.Args...)
			for _, path := range paths {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			daemonReload := services.DaemonReloadCommand()
			if _, err := (host.ExecRunner{}).Run(cmd.Context(), daemonReload.Name, daemonReload.Args...); err != nil {
				return err
			}
		}
		return writeBackupResult(cmd.OutOrStdout(), result, opts)
	}}
}

func safeUnitName(value string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func writeBackupResult(writer io.Writer, result backupResult, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(result)
	}
	if result.Output != "" {
		_, err := fmt.Fprintln(writer, result.Output)
		return err
	}
	if result.DryRun && len(result.Command) > 0 {
		_, err := fmt.Fprintf(writer, "would run: %s\n", strings.Join(result.Command, " "))
		return err
	}
	verb := result.Action
	if result.DryRun {
		verb = "would " + result.Action
	}
	_, err := fmt.Fprintf(writer, "%s backup operation for %s/%s\n", verb, result.Project, result.Environment)
	return err
}
