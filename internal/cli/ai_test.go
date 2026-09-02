package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIConfigureCommandWritesRemoteSettings(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("OMURGA_CONFIG_HOME", configHome)

	output := &bytes.Buffer{}
	configure := NewRootCommand()
	configure.SetOut(output)
	configure.SetErr(output)
	configure.SetArgs([]string{"ai", "configure", "--endpoint", "https://provider.example/v1/chat/completions", "--model", "test-model"})
	if err := configure.Execute(); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(configHome, "ai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "key") || !strings.Contains(string(content), "test-model") {
		t.Fatalf("unexpected AI configuration: %s", content)
	}
	if !strings.Contains(output.String(), "remote LLM configured") {
		t.Fatalf("unexpected output: %s", output.String())
	}
}
