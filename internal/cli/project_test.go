package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectCreateAndRenderCommands(t *testing.T) {
	parent := t.TempDir()
	createOutput := &bytes.Buffer{}
	createCommand := NewRootCommand()
	createCommand.SetOut(createOutput)
	createCommand.SetErr(createOutput)
	createCommand.SetArgs([]string{"project", "create", "demo", "--directory", parent})
	if err := createCommand.Execute(); err != nil {
		t.Fatalf("project create error = %v", err)
	}

	projectDirectory := filepath.Join(parent, "demo")
	if _, err := os.Stat(filepath.Join(projectDirectory, "omurga.yaml")); err != nil {
		t.Fatalf("project manifest is missing: %v", err)
	}

	renderOutput := &bytes.Buffer{}
	renderCommand := NewRootCommand()
	renderCommand.SetOut(renderOutput)
	renderCommand.SetErr(renderOutput)
	renderCommand.SetArgs([]string{"--env", "production", "project", "render", projectDirectory})
	if err := renderCommand.Execute(); err != nil {
		t.Fatalf("project render error = %v", err)
	}
	if !strings.Contains(renderOutput.String(), "services:") || !strings.Contains(renderOutput.String(), "127.0.0.1") {
		t.Fatalf("unexpected render output:\n%s", renderOutput.String())
	}

	deployOutput := &bytes.Buffer{}
	deployCommand := NewRootCommand()
	deployCommand.SetOut(deployOutput)
	deployCommand.SetErr(deployOutput)
	deployCommand.SetArgs([]string{"--env", "production", "--dry-run", "project", "deploy", projectDirectory})
	if err := deployCommand.Execute(); err != nil {
		t.Fatalf("project deploy dry-run error = %v", err)
	}
	if !strings.Contains(deployOutput.String(), "deployment plan") || !strings.Contains(deployOutput.String(), "--wait-timeout 120") {
		t.Fatalf("unexpected deploy plan output:\n%s", deployOutput.String())
	}

	controlOutput := &bytes.Buffer{}
	controlCommand := NewRootCommand()
	controlCommand.SetOut(controlOutput)
	controlCommand.SetErr(controlOutput)
	controlCommand.SetArgs([]string{"--env", "production", "--dry-run", "project", "restart", projectDirectory})
	if err := controlCommand.Execute(); err != nil {
		t.Fatalf("project restart dry-run error = %v", err)
	}
	if !strings.Contains(controlOutput.String(), "docker compose") || !strings.Contains(controlOutput.String(), " restart") {
		t.Fatalf("unexpected restart plan output:\n%s", controlOutput.String())
	}

	logsOutput := &bytes.Buffer{}
	logsCommand := NewRootCommand()
	logsCommand.SetOut(logsOutput)
	logsCommand.SetErr(logsOutput)
	logsCommand.SetArgs([]string{"--env", "production", "--dry-run", "project", "logs", projectDirectory, "--follow", "--tail", "25", "--service", "app"})
	if err := logsCommand.Execute(); err != nil {
		t.Fatalf("project logs dry-run error = %v", err)
	}
	if !strings.Contains(logsOutput.String(), "logs --follow --tail 25 app") {
		t.Fatalf("unexpected logs plan output:\n%s", logsOutput.String())
	}

	deleteOutput := &bytes.Buffer{}
	deleteCommand := NewRootCommand()
	deleteCommand.SetOut(deleteOutput)
	deleteCommand.SetErr(deleteOutput)
	deleteCommand.SetArgs([]string{"--env", "production", "--dry-run", "project", "delete", projectDirectory, "--purge-data"})
	if err := deleteCommand.Execute(); err != nil {
		t.Fatalf("project delete dry-run error = %v", err)
	}
	if !strings.Contains(deleteOutput.String(), "deletion plan") || !strings.Contains(deleteOutput.String(), "purge persistent project data") {
		t.Fatalf("unexpected delete plan output:\n%s", deleteOutput.String())
	}

	unsafeDeleteCommand := NewRootCommand()
	unsafeDeleteCommand.SetArgs([]string{"--env", "production", "project", "delete", projectDirectory, "--purge-data"})
	if err := unsafeDeleteCommand.Execute(); err == nil || !strings.Contains(err.Error(), "--purge-data requires --yes") {
		t.Fatalf("unsafe purge was not rejected: %v", err)
	}
}
