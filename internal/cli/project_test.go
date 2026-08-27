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
}
