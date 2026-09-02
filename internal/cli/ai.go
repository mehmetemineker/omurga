package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"omurga/internal/aiproject"
	"omurga/internal/llm"
	"omurga/internal/manifest"
	"omurga/internal/progress"
)

func newAICommand(opts *options) *cobra.Command {
	return newGroupCommand("ai", "Generate Omurga projects with a remote LLM",
		newAIConfigureCommand(opts),
		newAIPlanCommand(opts),
		newAICreateCommand(opts),
	)
}

func newAIConfigureCommand(opts *options) *cobra.Command {
	var endpoint, model string
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure a remote OpenAI-compatible LLM endpoint",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if endpoint == "" || model == "" {
				return fmt.Errorf("--endpoint and --model are required")
			}
			path, err := llm.DefaultConfigPath()
			if err != nil {
				return err
			}
			if !opts.dryRun {
				path, err = llm.SaveSettings(llm.Settings{Endpoint: endpoint, Model: model})
				if err != nil {
					return err
				}
			}
			if opts.quiet {
				return nil
			}
			if opts.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{
					"endpoint": endpoint, "model": model, "path": path,
				})
			}
			verb := "configured"
			if opts.dryRun {
				verb = "would configure"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "remote LLM %s: %s (%s)\n", verb, model, path)
			return err
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "remote OpenAI-compatible chat completions URL")
	cmd.Flags().StringVar(&model, "model", "", "remote model identifier")
	return cmd
}

func newAIPlanCommand(opts *options) *cobra.Command {
	var apiKeyFile, promptFile string
	cmd := &cobra.Command{
		Use:   "plan [prompt]",
		Short: "Generate and validate a project manifest without writing files",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt, err := readAIPrompt(args, promptFile, cmd.InOrStdin())
			if err != nil {
				return err
			}
			project, err := generateAIProject(cmd, prompt, apiKeyFile)
			if err != nil {
				return err
			}
			return writeAIPlan(cmd.OutOrStdout(), project, opts)
		},
	}
	cmd.Flags().StringVar(&apiKeyFile, "api-key-file", "", "read the remote API key from a file")
	cmd.Flags().StringVar(&promptFile, "prompt-file", "", "read the prompt from a file")
	return cmd
}

func newAICreateCommand(opts *options) *cobra.Command {
	var apiKeyFile, promptFile, parent, name, environment string
	cmd := &cobra.Command{
		Use:   "create [prompt]",
		Short: "Generate and create a validated Omurga project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt, err := readAIPrompt(args, promptFile, cmd.InOrStdin())
			if err != nil {
				return err
			}
			project, err := generateAIProject(cmd, prompt, apiKeyFile)
			if err != nil {
				return err
			}
			result, err := aiproject.CreateProject(project, parent, name, environment, opts.dryRun)
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
			if opts.dryRun {
				action = "would create"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s AI-generated project %s at %s\n", action, result.Project.Name, result.Directory)
			return err
		},
	}
	cmd.Flags().StringVar(&apiKeyFile, "api-key-file", "", "read the remote API key from a file")
	cmd.Flags().StringVar(&promptFile, "prompt-file", "", "read the prompt from a file")
	cmd.Flags().StringVar(&parent, "directory", ".", "parent directory for the generated project")
	cmd.Flags().StringVar(&name, "name", "", "override the project name returned by the model")
	cmd.Flags().StringVar(&environment, "environment", "production", "environment overlay to create")
	return cmd
}

func readAIPrompt(args []string, promptFile string, stdin io.Reader) (string, error) {
	if promptFile != "" && len(args) > 0 {
		return "", fmt.Errorf("use either [prompt] or --prompt-file, not both")
	}
	if promptFile != "" {
		content, err := os.ReadFile(promptFile)
		if err != nil {
			return "", fmt.Errorf("could not read prompt file: %w", err)
		}
		if strings.TrimSpace(string(content)) == "" {
			return "", fmt.Errorf("prompt file is empty")
		}
		return string(content), nil
	}
	if len(args) == 1 {
		return strings.TrimSpace(args[0]), nil
	}
	content, err := io.ReadAll(io.LimitReader(stdin, 256*1024))
	if err != nil {
		return "", fmt.Errorf("could not read prompt from standard input: %w", err)
	}
	if strings.TrimSpace(string(content)) == "" {
		return "", fmt.Errorf("prompt cannot be empty")
	}
	return string(content), nil
}

func generateAIProject(cmd *cobra.Command, prompt, apiKeyFile string) (manifest.Project, error) {
	config, err := llm.ResolveConfig(apiKeyFile)
	if err != nil {
		return manifest.Project{}, err
	}
	client, err := llm.NewClient(config)
	if err != nil {
		return manifest.Project{}, err
	}
	task := progress.FromContext(cmd.Context()).Start("Generate project manifest with remote LLM")
	content, err := client.Generate(cmd.Context(), prompt)
	if err != nil {
		task.Fail(err)
		return manifest.Project{}, err
	}
	project, err := aiproject.ParseManifest(content)
	if err != nil {
		task.Fail(err)
		return manifest.Project{}, err
	}
	task.Complete()
	return project, nil
}

func writeAIPlan(writer io.Writer, project manifest.Project, opts *options) error {
	if opts.quiet {
		return nil
	}
	if opts.json {
		return json.NewEncoder(writer).Encode(project)
	}
	content, err := yaml.Marshal(project)
	if err != nil {
		return err
	}
	_, err = writer.Write(content)
	return err
}
