package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"omurga/internal/host"
	"omurga/internal/manifest"
	"omurga/internal/progress"
	"omurga/internal/webhook"
)

func newWebhookCommand(opts *options) *cobra.Command {
	return newGroupCommand("webhook", "Manage secure image deployment webhooks",
		newWebhookAddCommand(opts),
		newWebhookListCommand(opts),
		newWebhookServeCommand(opts),
	)
}

func newWebhookAddCommand(opts *options) *cobra.Command {
	var projectName, environment, service, manifestPath, imagePrefix, secretFile string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a signed image deployment webhook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "webhook add"); err != nil {
				return err
			}
			paths := host.DefaultPaths("/")
			if secretFile == "" {
				secretFile = filepath.Join(filepath.Dir(paths.WebhookConfig), "webhooks", args[0]+".secret")
			}
			secret, err := webhook.AddHook(paths.WebhookConfig, webhook.Hook{
				Name: args[0], Project: projectName, Environment: environment, Service: service,
				ManifestPath: manifestPath, ImagePrefix: imagePrefix, SecretFile: secretFile, Enabled: true,
			})
			if err != nil {
				return err
			}
			if opts.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{
					"name": args[0], "config": paths.WebhookConfig, "secret": secret,
				})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "created webhook %s\nconfig: %s\nsecret: %s\n", args[0], paths.WebhookConfig, secret)
			return err
		},
	}
	cmd.Flags().StringVar(&projectName, "project", "", "project name expected in the payload")
	cmd.Flags().StringVar(&environment, "environment", "", "environment expected in the payload")
	cmd.Flags().StringVar(&service, "service", "", "service whose image will be deployed")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "absolute path to the project manifest")
	cmd.Flags().StringVar(&imagePrefix, "image-prefix", "", "allowed image repository prefix")
	cmd.Flags().StringVar(&secretFile, "secret-file", "", "absolute path for the generated signing secret")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("environment")
	_ = cmd.MarkFlagRequired("service")
	_ = cmd.MarkFlagRequired("manifest")
	_ = cmd.MarkFlagRequired("image-prefix")
	return cmd
}

func newWebhookListCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured image deployment webhooks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "webhook list"); err != nil {
				return err
			}
			config, err := webhook.LoadConfig(host.DefaultPaths("/").WebhookConfig)
			if err != nil {
				return err
			}
			if opts.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(config)
			}
			for _, hook := range config.Hooks {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-16s %-16s %-12s %s\n", hook.Name, hook.Project, hook.Environment, hook.Service, hook.ImagePrefix); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newWebhookServeCommand(opts *options) *cobra.Command {
	var listen, configPath string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve signed image deployment webhooks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireLocalHost(opts.host); err != nil {
				return err
			}
			if err := requireRoot(cmd.Context(), host.ExecRunner{}, "webhook serve"); err != nil {
				return err
			}
			paths := host.DefaultPaths("/")
			if configPath == "" {
				configPath = paths.WebhookConfig
			}
			hooks, err := webhook.LoadRuntimeHooks(configPath)
			if err != nil {
				return err
			}
			deployment := func(ctx context.Context, hook webhook.Hook, image string) (webhook.DeploymentResult, error) {
				loaded, err := manifest.Load(hook.ManifestPath, hook.Environment)
				if err != nil {
					return webhook.DeploymentResult{}, err
				}
				if loaded.Project.Name != hook.Project {
					return webhook.DeploymentResult{}, fmt.Errorf("manifest project %q does not match webhook project %q", loaded.Project.Name, hook.Project)
				}
				lifecycle, err := platformLifecycle(paths, host.ExecRunner{}, false, progress.FromContext(ctx))
				if err != nil {
					return webhook.DeploymentResult{}, err
				}
				result, err := lifecycle.DeployImage(ctx, loaded, hook.Service, image, false)
				if err != nil {
					return webhook.DeploymentResult{}, err
				}
				return webhook.DeploymentResult{Project: result.Project, Environment: result.Environment, Service: hook.Service, Image: image, Revision: result.Revision}, nil
			}
			handler, err := webhook.NewHandler(hooks, filepath.Join(paths.StateRoot, "webhooks", "replay.json"), deployment)
			if err != nil {
				return err
			}
			server := &http.Server{
				Addr:              listen,
				Handler:           handler,
				ReadHeaderTimeout: 10 * time.Second,
				ReadTimeout:       5 * time.Minute,
				WriteTimeout:      5 * time.Minute,
				IdleTimeout:       60 * time.Second,
			}
			go func() {
				<-cmd.Context().Done()
				shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = server.Shutdown(shutdownContext)
			}()
			err = server.ListenAndServe()
			if err == http.ErrServerClosed {
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8090", "HTTP address to listen on")
	cmd.Flags().StringVar(&configPath, "config", "", "webhook configuration path")
	return cmd
}
