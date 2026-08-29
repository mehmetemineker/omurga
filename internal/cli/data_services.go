package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"omurga/internal/gateway"
	"omurga/internal/host"
	"omurga/internal/manifest"
	projectruntime "omurga/internal/project"
)

var databaseIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

type dataCommandResult struct {
	Project      string   `json:"project"`
	Environment  string   `json:"environment"`
	Instance     string   `json:"instance"`
	Action       string   `json:"action"`
	Command      []string `json:"command"`
	Output       string   `json:"output,omitempty"`
	Path         string   `json:"path,omitempty"`
	SafetyBackup string   `json:"safetyBackup,omitempty"`
	DryRun       bool     `json:"dryRun,omitempty"`
}

func newPostgresCommand(opts *options) *cobra.Command {
	return newGroupCommand("postgres", "Manage project PostgreSQL instances and databases",
		newDataCaptureCommand(opts, "postgres", "status", "Show PostgreSQL status"),
		newDataCaptureCommand(opts, "postgres", "databases", "List databases"),
		newPostgresCreateDatabaseCommand(opts),
		newPostgresCreateUserCommand(opts),
		newDataShellCommand(opts, "postgres"),
		newPostgresBackupCommand(opts),
		newPostgresRestoreCommand(opts),
	)
}

func newRedisCommand(opts *options) *cobra.Command {
	return newGroupCommand("redis", "Manage project Redis instances",
		newDataCaptureCommand(opts, "redis", "status", "Show Redis status"),
		newDataCaptureCommand(opts, "redis", "stats", "Show Redis statistics"),
		newDataShellCommand(opts, "redis"),
		newRedisBackupCommand(opts),
		newRedisFlushCommand(opts),
	)
}

func dataContext(args []string, environment, kind, requested string) (manifest.LoadedProject, string, manifest.Dependency, []string, error) {
	loaded, err := loadProjectArgument(args, environment)
	if err != nil {
		return manifest.LoadedProject{}, "", manifest.Dependency{}, nil, err
	}
	names := make([]string, 0)
	for name, dependency := range loaded.Project.Dependencies {
		if dependency.Type == kind && dependency.Mode != "shared" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if requested == "" {
		if len(names) == 0 {
			return manifest.LoadedProject{}, "", manifest.Dependency{}, nil, fmt.Errorf("project has no project-scoped %s instance", kind)
		}
		if len(names) > 1 {
			return manifest.LoadedProject{}, "", manifest.Dependency{}, nil, fmt.Errorf("multiple %s instances are defined; select one with --instance", kind)
		}
		requested = names[0]
	}
	dependency, exists := loaded.Project.Dependencies[requested]
	if !exists || dependency.Type != kind || dependency.Mode == "shared" {
		return manifest.LoadedProject{}, "", manifest.Dependency{}, nil, fmt.Errorf("project-scoped %s instance %s was not found", kind, requested)
	}
	env := gateway.EnvironmentKey(loaded.Environment)
	layout := projectruntime.NewLifecycle(host.DefaultPaths("/"), host.ExecRunner{}).Layout(loaded.Project.Name, env)
	compose := []string{"compose", "--project-name", projectruntime.ComposeProjectName(loaded.Project.Name, env), "--file", layout.Compose}
	return loaded, requested, dependency, compose, nil
}

func newDataCaptureCommand(opts *options, kind, action, short string) *cobra.Command {
	var instance string
	cmd := &cobra.Command{Use: action + " [project-directory-or-manifest]", Short: short, Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLocalHost(opts.host); err != nil {
			return err
		}
		loaded, selected, dependency, compose, err := dataContext(args, opts.environment, kind, instance)
		if err != nil {
			return err
		}
		var operation []string
		if action == "status" {
			operation = []string{"ps", selected}
		} else if kind == "postgres" {
			operation = []string{"exec", "-T", selected, "psql", "-U", dependency.User, "-d", "postgres", "-Atc", "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname"}
		} else {
			operation = []string{"exec", "-T", selected, "redis-cli", "INFO"}
		}
		return runDataCapture(cmd, opts, loaded, selected, action, append(compose, operation...))
	}}
	cmd.Flags().StringVar(&instance, "instance", "", "dependency name to operate on")
	return cmd
}

func newPostgresCreateDatabaseCommand(opts *options) *cobra.Command {
	var instance string
	cmd := &cobra.Command{Use: "create-db [database] [project-directory-or-manifest]", Short: "Create a database", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		if !databaseIdentifierPattern.MatchString(args[0]) {
			return fmt.Errorf("invalid database identifier %q", args[0])
		}
		loaded, selected, dependency, compose, err := dataContext(args[1:], opts.environment, "postgres", instance)
		if err != nil {
			return err
		}
		sql := `CREATE DATABASE "` + strings.ReplaceAll(args[0], `"`, `""`) + `"`
		operation := []string{"exec", "-T", selected, "psql", "-v", "ON_ERROR_STOP=1", "-U", dependency.User, "-d", "postgres", "-c", sql}
		return runDataCapture(cmd, opts, loaded, selected, "create-db", append(compose, operation...))
	}}
	cmd.Flags().StringVar(&instance, "instance", "", "PostgreSQL dependency name")
	return cmd
}

func newPostgresCreateUserCommand(opts *options) *cobra.Command {
	var instance, passwordFile string
	cmd := &cobra.Command{Use: "create-user [user] [project-directory-or-manifest]", Short: "Create a PostgreSQL login role", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		if !databaseIdentifierPattern.MatchString(args[0]) {
			return fmt.Errorf("invalid role identifier %q", args[0])
		}
		loaded, selected, dependency, compose, err := dataContext(args[1:], opts.environment, "postgres", instance)
		if err != nil {
			return err
		}
		operation := []string{"exec", "-T", selected, "psql", "-v", "ON_ERROR_STOP=1", "-U", dependency.User, "-d", "postgres"}
		command := append(compose, operation...)
		result := makeDataResult(loaded, selected, "create-user", command, opts.dryRun)
		if opts.dryRun {
			return writeDataResult(cmd.OutOrStdout(), result, opts)
		}
		if passwordFile == "" {
			return fmt.Errorf("--password-file is required")
		}
		if err := requireRoot(cmd.Context(), host.ExecRunner{}, "postgres create-user"); err != nil {
			return err
		}
		password, err := readSecretValue(passwordFile, cmd.InOrStdin())
		if err != nil {
			return err
		}
		escaped := strings.ReplaceAll(string(bytes.TrimRight(password, "\r\n")), "'", "''")
		sql := `CREATE ROLE "` + strings.ReplaceAll(args[0], `"`, `""`) + `" LOGIN PASSWORD '` + escaped + `';\n`
		var output bytes.Buffer
		if err := (host.ExecRunner{}).RunIO(cmd.Context(), strings.NewReader(sql), &output, cmd.ErrOrStderr(), "docker", command...); err != nil {
			return err
		}
		result.Output = strings.TrimSpace(output.String())
		return writeDataResult(cmd.OutOrStdout(), result, opts)
	}}
	cmd.Flags().StringVar(&instance, "instance", "", "PostgreSQL dependency name")
	cmd.Flags().StringVar(&passwordFile, "password-file", "", "read the new role password from a file, or - for standard input")
	return cmd
}

func newDataShellCommand(opts *options, kind string) *cobra.Command {
	var instance string
	cmd := &cobra.Command{Use: "shell [project-directory-or-manifest]", Short: "Open an interactive " + kind + " shell", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if opts.json || opts.quiet {
			return fmt.Errorf("interactive shells do not support --json or --quiet")
		}
		loaded, selected, dependency, compose, err := dataContext(args, opts.environment, kind, instance)
		if err != nil {
			return err
		}
		operation := []string{"exec", selected}
		if kind == "postgres" {
			operation = append(operation, "psql", "-U", dependency.User, "-d", dependency.Database)
		} else {
			operation = append(operation, "redis-cli")
		}
		command := append(compose, operation...)
		if opts.dryRun {
			return writeDataResult(cmd.OutOrStdout(), makeDataResult(loaded, selected, "shell", command, true), opts)
		}
		if err := requireRoot(cmd.Context(), host.ExecRunner{}, kind+" shell"); err != nil {
			return err
		}
		return (host.ExecRunner{}).RunIO(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), "docker", command...)
	}}
	cmd.Flags().StringVar(&instance, "instance", "", "dependency name to operate on")
	return cmd
}

func newPostgresBackupCommand(opts *options) *cobra.Command {
	var instance, output string
	cmd := &cobra.Command{Use: "backup [project-directory-or-manifest]", Short: "Create a PostgreSQL custom-format dump", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		loaded, selected, dependency, compose, err := dataContext(args, opts.environment, "postgres", instance)
		if err != nil {
			return err
		}
		if output == "" {
			output = filepath.Join(host.DefaultPaths("/").BackupStaging, fmt.Sprintf("%s-%s-%s-%s.dump", loaded.Project.Name, gateway.EnvironmentKey(loaded.Environment), selected, time.Now().UTC().Format("20060102T150405Z")))
		}
		command := append(compose, "exec", "-T", selected, "pg_dump", "-U", dependency.User, "-d", dependency.Database, "--format=custom")
		result := makeDataResult(loaded, selected, "backup", command, opts.dryRun)
		result.Path = output
		if opts.dryRun {
			return writeDataResult(cmd.OutOrStdout(), result, opts)
		}
		if err := requireRoot(cmd.Context(), host.ExecRunner{}, "postgres backup"); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
			return err
		}
		file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("could not create dump file: %w", err)
		}
		runErr := (host.ExecRunner{}).RunIO(cmd.Context(), nil, file, cmd.ErrOrStderr(), "docker", command...)
		closeErr := file.Close()
		if runErr != nil || closeErr != nil {
			_ = os.Remove(output)
			return errors.Join(runErr, closeErr)
		}
		return writeDataResult(cmd.OutOrStdout(), result, opts)
	}}
	cmd.Flags().StringVar(&instance, "instance", "", "PostgreSQL dependency name")
	cmd.Flags().StringVarP(&output, "output", "o", "", "dump output path")
	return cmd
}

func newPostgresRestoreCommand(opts *options) *cobra.Command {
	var instance, input string
	var noSafetyBackup bool
	cmd := &cobra.Command{Use: "restore [project-directory-or-manifest]", Short: "Restore a PostgreSQL custom-format dump", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		loaded, selected, dependency, compose, err := dataContext(args, opts.environment, "postgres", instance)
		if err != nil {
			return err
		}
		if input == "" {
			return fmt.Errorf("--file is required")
		}
		command := append(compose, "exec", "-T", selected, "pg_restore", "-U", dependency.User, "-d", dependency.Database, "--clean", "--if-exists", "--no-owner")
		result := makeDataResult(loaded, selected, "restore", command, opts.dryRun)
		result.Path = input
		if opts.dryRun {
			return writeDataResult(cmd.OutOrStdout(), result, opts)
		}
		if !opts.yes {
			return fmt.Errorf("postgres restore requires --yes because database objects will be replaced")
		}
		if err := requireRoot(cmd.Context(), host.ExecRunner{}, "postgres restore"); err != nil {
			return err
		}
		if !noSafetyBackup {
			staging := host.DefaultPaths("/").BackupStaging
			if err := os.MkdirAll(staging, 0o700); err != nil {
				return err
			}
			safetyPath := filepath.Join(staging, fmt.Sprintf("pre-restore-%s-%s-%s.dump", loaded.Project.Name, selected, time.Now().UTC().Format("20060102T150405Z")))
			safetyFile, err := os.OpenFile(safetyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return fmt.Errorf("could not create safety backup: %w", err)
			}
			dumpCommand := append(compose, "exec", "-T", selected, "pg_dump", "-U", dependency.User, "-d", dependency.Database, "--format=custom")
			runErr := (host.ExecRunner{}).RunIO(cmd.Context(), nil, safetyFile, cmd.ErrOrStderr(), "docker", dumpCommand...)
			closeErr := safetyFile.Close()
			if runErr != nil || closeErr != nil {
				_ = os.Remove(safetyPath)
				return fmt.Errorf("could not create pre-restore safety backup: %w", errors.Join(runErr, closeErr))
			}
			result.SafetyBackup = safetyPath
		}
		file, err := os.Open(input)
		if err != nil {
			return err
		}
		defer file.Close()
		if err := (host.ExecRunner{}).RunIO(cmd.Context(), file, cmd.OutOrStdout(), cmd.ErrOrStderr(), "docker", command...); err != nil {
			return err
		}
		return writeDataResult(cmd.OutOrStdout(), result, opts)
	}}
	cmd.Flags().StringVar(&instance, "instance", "", "PostgreSQL dependency name")
	cmd.Flags().StringVar(&input, "file", "", "custom-format dump to restore")
	cmd.Flags().BoolVar(&noSafetyBackup, "no-safety-backup", false, "skip the pre-restore safety dump")
	return cmd
}

func newRedisBackupCommand(opts *options) *cobra.Command {
	var instance, output string
	cmd := &cobra.Command{Use: "backup [project-directory-or-manifest]", Short: "Create a Redis RDB snapshot", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		loaded, selected, _, compose, err := dataContext(args, opts.environment, "redis", instance)
		if err != nil {
			return err
		}
		if output == "" {
			output = filepath.Join(host.DefaultPaths("/").BackupStaging, fmt.Sprintf("%s-%s-%s-%s.rdb", loaded.Project.Name, gateway.EnvironmentKey(loaded.Environment), selected, time.Now().UTC().Format("20060102T150405Z")))
		}
		command := append(compose, "exec", "-T", selected, "redis-cli", "SAVE")
		result := makeDataResult(loaded, selected, "backup", command, opts.dryRun)
		result.Path = output
		if opts.dryRun {
			return writeDataResult(cmd.OutOrStdout(), result, opts)
		}
		if err := requireRoot(cmd.Context(), host.ExecRunner{}, "redis backup"); err != nil {
			return err
		}
		if _, err := (host.ExecRunner{}).Run(cmd.Context(), "docker", command...); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
			return err
		}
		copyCommand := append(compose, "cp", selected+":/data/dump.rdb", output)
		if _, err := (host.ExecRunner{}).Run(cmd.Context(), "docker", copyCommand...); err != nil {
			return err
		}
		return writeDataResult(cmd.OutOrStdout(), result, opts)
	}}
	cmd.Flags().StringVar(&instance, "instance", "", "Redis dependency name")
	cmd.Flags().StringVarP(&output, "output", "o", "", "snapshot output path")
	return cmd
}

func newRedisFlushCommand(opts *options) *cobra.Command {
	var instance string
	cmd := &cobra.Command{Use: "flush [project-directory-or-manifest]", Short: "Flush all Redis data", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		loaded, selected, _, compose, err := dataContext(args, opts.environment, "redis", instance)
		if err != nil {
			return err
		}
		if !opts.dryRun && !opts.yes {
			return fmt.Errorf("redis flush requires --yes because all data will be permanently removed")
		}
		return runDataCapture(cmd, opts, loaded, selected, "flush", append(compose, "exec", "-T", selected, "redis-cli", "FLUSHALL"))
	}}
	cmd.Flags().StringVar(&instance, "instance", "", "Redis dependency name")
	return cmd
}

func runDataCapture(cmd *cobra.Command, opts *options, loaded manifest.LoadedProject, instance, action string, command []string) error {
	result := makeDataResult(loaded, instance, action, command, opts.dryRun)
	if !opts.dryRun {
		if err := requireRoot(cmd.Context(), host.ExecRunner{}, action); err != nil {
			return err
		}
		output, err := (host.ExecRunner{}).Run(cmd.Context(), "docker", command...)
		if err != nil {
			return err
		}
		result.Output = output
	}
	return writeDataResult(cmd.OutOrStdout(), result, opts)
}

func makeDataResult(loaded manifest.LoadedProject, instance, action string, command []string, dryRun bool) dataCommandResult {
	return dataCommandResult{Project: loaded.Project.Name, Environment: gateway.EnvironmentKey(loaded.Environment), Instance: instance, Action: action, Command: append([]string{"docker"}, command...), DryRun: dryRun}
}

func writeDataResult(writer io.Writer, result dataCommandResult, opts *options) error {
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
	if result.DryRun {
		_, err := fmt.Fprintf(writer, "would run: %s\n", strings.Join(result.Command, " "))
		return err
	}
	if result.Path != "" {
		message := fmt.Sprintf("%s completed for %s/%s at %s", result.Action, result.Project, result.Instance, result.Path)
		if result.SafetyBackup != "" {
			message += " (safety backup: " + result.SafetyBackup + ")"
		}
		_, err := fmt.Fprintln(writer, message)
		return err
	}
	_, err := fmt.Fprintf(writer, "%s completed for %s/%s\n", result.Action, result.Project, result.Instance)
	return err
}
