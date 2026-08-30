package webhook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const serviceName = "omurga-webhook"

func ServiceName() string {
	return serviceName
}

func RenderServiceUnit(binary, listen, config string) ([]byte, error) {
	for name, value := range map[string]string{"binary": binary, "listen": listen, "config": config} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("webhook service %s is required and must not contain newlines", name)
		}
	}
	if !filepath.IsAbs(binary) {
		return nil, fmt.Errorf("webhook service binary must be an absolute path")
	}
	if !filepath.IsAbs(config) {
		return nil, fmt.Errorf("webhook service config must be an absolute path")
	}
	arguments := []string{binary, "webhook", "serve", "--listen", listen, "--config", config}
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		quoted[index] = systemdQuote(argument)
	}
	unit := "[Unit]\n" +
		"Description=Omurga image deployment webhook\n" +
		"After=network-online.target docker.service\n" +
		"Wants=network-online.target\n\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"User=root\n" +
		"ExecStart=" + strings.Join(quoted, " ") + "\n" +
		"Restart=on-failure\n" +
		"RestartSec=5s\n" +
		"UMask=0077\n\n" +
		"[Install]\n" +
		"WantedBy=multi-user.target\n"
	return []byte(unit), nil
}

func WriteServiceUnit(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("could not create webhook service directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".omurga-webhook-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("could not replace webhook service unit: %w", err)
	}
	return nil
}

func systemdQuote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`).Replace(value) + `"`
}
