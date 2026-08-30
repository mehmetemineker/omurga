package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"omurga/internal/host"
	"omurga/internal/progress"
	projectruntime "omurga/internal/project"
	"omurga/internal/shared"
)

func newServiceCommand(opts *options) *cobra.Command {
	return newGroupCommand("service", "Manage shared Docker services",
		newServiceCatalogCommand(opts),
		newServiceInstallCommand(opts),
		newServiceListCommand(opts),
		newServiceStatusCommand(opts),
		newServiceRemoveCommand(opts),
	)
}

func newServiceCatalogCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "catalog", Short: "Show the built-in service catalog", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		catalog := shared.Catalog()
		if opts.quiet {
			return nil
		}
		if opts.json {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(catalog)
		}
		names := make([]string, 0, len(catalog))
		for name := range catalog {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-16s %s\n", name, catalog[name].Image); err != nil {
				return err
			}
		}
		return nil
	}}
}

func newServiceInstallCommand(opts *options) *cobra.Command {
	var name, image, environmentFile string
	cmd := &cobra.Command{Use: "install [catalog-name]", Short: "Install or update a shared Docker service", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLocalHost(opts.host); err != nil {
			return err
		}
		if name == "" {
			name = args[0]
		}
		paths := host.DefaultPaths("/")
		content, dataPath, err := shared.Generate(args[0], name, image, environmentFile, paths)
		if err != nil {
			return err
		}
		root, composePath, _, err := shared.Layout(paths, name)
		if err != nil {
			return err
		}
		command := []string{"docker", "compose", "--project-name", "omurga-shared-" + name, "--file", composePath, "up", "--detach", "--wait"}
		if opts.dryRun {
			return writeServiceAction(cmd, opts, name, "install", command, []string{composePath, dataPath}, "")
		}
		if err := requireRoot(cmd.Context(), host.ExecRunner{}, "service install"); err != nil {
			return err
		}
		if environmentFile != "" {
			info, err := os.Stat(environmentFile)
			if err != nil {
				return fmt.Errorf("could not access environment file: %w", err)
			}
			if info.Mode().Perm()&0o077 != 0 {
				return fmt.Errorf("shared service environment file permissions are too broad")
			}
		}
		if err := os.MkdirAll(dataPath, 0o750); err != nil {
			return err
		}
		if _, err := (host.ExecRunner{}).Run(cmd.Context(), "docker", "network", "inspect", "omurga-shared"); err != nil {
			if _, err := (host.ExecRunner{}).Run(cmd.Context(), "docker", "network", "create", "omurga-shared"); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(root, 0o750); err != nil {
			return err
		}
		if err := projectruntime.WriteArtifact(composePath, content, 0o640); err != nil {
			return err
		}
		task := progress.FromContext(cmd.Context()).Start("Start and health-check shared service containers")
		output, err := (host.ExecRunner{}).Run(cmd.Context(), command[0], command[1:]...)
		if err != nil {
			task.Fail(err)
			return err
		}
		task.Complete()
		return writeServiceAction(cmd, opts, name, "install", command, []string{composePath, dataPath}, output)
	}}
	cmd.Flags().StringVar(&name, "name", "", "shared service instance name")
	cmd.Flags().StringVar(&image, "image", "", "override image or provide an image for a custom catalog name")
	cmd.Flags().StringVar(&environmentFile, "environment-file", "", "root-only Compose environment file")
	return cmd
}

func newServiceListCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List shared services", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		names, err := shared.List(host.DefaultPaths("/"))
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

func newServiceStatusCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "status [name]", Short: "Show shared service container status", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		_, compose, _, err := shared.Layout(host.DefaultPaths("/"), args[0])
		if err != nil {
			return err
		}
		command := []string{"docker", "compose", "--project-name", "omurga-shared-" + args[0], "--file", compose, "ps"}
		if opts.dryRun {
			return writeServiceAction(cmd, opts, args[0], "status", command, nil, "")
		}
		task := progress.FromContext(cmd.Context()).Start("Remove shared service containers")
		output, err := (host.ExecRunner{}).Run(cmd.Context(), command[0], command[1:]...)
		if err != nil {
			task.Fail(err)
			return err
		}
		task.Complete()
		return writeServiceAction(cmd, opts, args[0], "status", command, nil, output)
	}}
}

func newServiceRemoveCommand(opts *options) *cobra.Command {
	var purgeData bool
	cmd := &cobra.Command{Use: "remove [name]", Short: "Remove a shared service and preserve data by default", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, compose, data, err := shared.Layout(host.DefaultPaths("/"), args[0])
		if err != nil {
			return err
		}
		command := []string{"docker", "compose", "--project-name", "omurga-shared-" + args[0], "--file", compose, "down", "--remove-orphans"}
		paths := []string{compose}
		if purgeData {
			paths = append(paths, data)
		}
		if opts.dryRun {
			return writeServiceAction(cmd, opts, args[0], "remove", command, paths, "")
		}
		if purgeData && !opts.yes {
			return fmt.Errorf("service remove --purge-data requires --yes because persistent data will be permanently deleted")
		}
		if err := requireRoot(cmd.Context(), host.ExecRunner{}, "service remove"); err != nil {
			return err
		}
		if _, err := os.Stat(compose); err != nil {
			return fmt.Errorf("shared service %s was not found", args[0])
		}
		output, err := (host.ExecRunner{}).Run(cmd.Context(), command[0], command[1:]...)
		if err != nil {
			return err
		}
		if err := os.Remove(compose); err != nil {
			return err
		}
		if purgeData {
			absoluteData, _ := filepath.Abs(data)
			absoluteRoot, _ := filepath.Abs(root)
			if filepath.Dir(absoluteData) != absoluteRoot {
				return fmt.Errorf("refusing to purge data outside the shared service root")
			}
			if err := os.RemoveAll(absoluteData); err != nil {
				return err
			}
		}
		_ = os.Remove(root)
		return writeServiceAction(cmd, opts, args[0], "remove", command, paths, output)
	}}
	cmd.Flags().BoolVar(&purgeData, "purge-data", false, "permanently remove shared service data")
	return cmd
}

func writeServiceAction(cmd *cobra.Command, opts *options, name, action string, command, paths []string, output string) error {
	result := map[string]any{"name": name, "action": action, "command": command, "paths": paths, "output": output, "dryRun": opts.dryRun}
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
	if opts.dryRun {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "would run: %s\n", strings.Join(command, " "))
		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s completed for shared service %s\n", action, name)
	return err
}
