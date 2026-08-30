package webhook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderServiceUnitUsesPinnedArgumentsAndRestartPolicy(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "omurga")
	config := filepath.Join(t.TempDir(), "webhooks.yaml")
	unit, err := RenderServiceUnit(binary, "127.0.0.1:8090", config)
	if err != nil {
		t.Fatal(err)
	}
	text := string(unit)
	for _, expected := range []string{
		"Description=Omurga image deployment webhook",
		"ExecStart=" + systemdQuote(binary) + ` "webhook" "serve" "--listen" "127.0.0.1:8090" "--config" ` + systemdQuote(config),
		"Restart=on-failure",
		"User=root",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("service unit does not contain %q:\n%s", expected, text)
		}
	}
}

func TestWriteServiceUnitReplacesUnitAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "systemd", "omurga-webhook.service")
	if err := WriteServiceUnit(path, []byte("[Unit]\nDescription=test\n")); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "[Unit]\nDescription=test\n" {
		t.Fatalf("unexpected service unit: %q", content)
	}
}
