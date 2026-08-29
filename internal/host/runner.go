package host

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
	LookPath(name string) (string, error)
}

type StreamRunner interface {
	Stream(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error
}

type IORunner interface {
	RunIO(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error
}

type EnvironmentRunner interface {
	RunEnvironment(ctx context.Context, environment map[string]string, name string, args ...string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	return ExecRunner{}.RunEnvironment(ctx, nil, name, args...)
}

func (ExecRunner) RunEnvironment(ctx context.Context, environment map[string]string, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	overrides := make(map[string]string, len(environment)+1)
	for key, value := range environment {
		overrides[key] = value
	}
	if name == "apt-get" {
		overrides["DEBIAN_FRONTEND"] = "noninteractive"
	}
	command.Env = mergedEnvironment(os.Environ(), overrides)
	output, err := command.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("%s failed: %w", name, err)
		}
		return text, fmt.Errorf("%s failed: %w: %s", name, err, text)
	}
	return text, nil
}

func mergedEnvironment(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	for _, assignment := range base {
		key, _, found := strings.Cut(assignment, "=")
		if found {
			if _, replaced := overrides[key]; replaced {
				continue
			}
		}
		result = append(result, assignment)
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func (ExecRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func (ExecRunner) Stream(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	return ExecRunner{}.RunIO(ctx, nil, stdout, stderr, name, args...)
}

func (ExecRunner) RunIO(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = os.Environ()
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func IsRoot(ctx context.Context, runner Runner) (bool, error) {
	output, err := runner.Run(ctx, "id", "-u")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) == "0", nil
}
