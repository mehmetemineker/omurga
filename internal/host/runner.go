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

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = os.Environ()
	if name == "apt-get" {
		command.Env = append(command.Env, "DEBIAN_FRONTEND=noninteractive")
	}
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

func (ExecRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func (ExecRunner) Stream(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = os.Environ()
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
