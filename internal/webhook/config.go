package webhook

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

const configVersion = 1

var identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type Config struct {
	Version int    `yaml:"version" json:"version"`
	Hooks   []Hook `yaml:"hooks" json:"hooks"`
}

type Hook struct {
	Name         string `yaml:"name" json:"name"`
	Project      string `yaml:"project" json:"project"`
	Environment  string `yaml:"environment" json:"environment"`
	Service      string `yaml:"service" json:"service"`
	ManifestPath string `yaml:"manifestPath" json:"manifestPath"`
	ImagePrefix  string `yaml:"imagePrefix" json:"imagePrefix"`
	SecretFile   string `yaml:"secretFile" json:"secretFile"`
	Enabled      bool   `yaml:"enabled" json:"enabled"`
}

type RuntimeHook struct {
	Hook
	secret []byte
}

func LoadConfig(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("could not read webhook config: %w", err)
	}
	if runtime.GOOS != "windows" {
		if info, statErr := os.Stat(path); statErr != nil {
			return Config{}, fmt.Errorf("could not inspect webhook config: %w", statErr)
		} else if info.Mode().Perm()&0o077 != 0 {
			return Config{}, fmt.Errorf("webhook config must not be readable by group or other users")
		}
	}
	var config Config
	if err := yaml.Unmarshal(content, &config); err != nil {
		return Config{}, fmt.Errorf("could not parse webhook config: %w", err)
	}
	if err := validateConfig(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func LoadRuntimeHooks(path string) ([]RuntimeHook, error) {
	config, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	hooks := make([]RuntimeHook, 0, len(config.Hooks))
	for _, hook := range config.Hooks {
		if !hook.Enabled {
			continue
		}
		secret, err := os.ReadFile(hook.SecretFile)
		if err != nil {
			return nil, fmt.Errorf("could not read secret for webhook %s: %w", hook.Name, err)
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(hook.SecretFile)
			if err != nil {
				return nil, fmt.Errorf("could not inspect secret for webhook %s: %w", hook.Name, err)
			}
			if info.Mode().Perm()&0o077 != 0 {
				return nil, fmt.Errorf("webhook %s secret must not be readable by group or other users", hook.Name)
			}
		}
		secret = []byte(strings.TrimSpace(string(secret)))
		if len(secret) < 32 {
			return nil, fmt.Errorf("webhook %s secret must contain at least 32 bytes", hook.Name)
		}
		hooks = append(hooks, RuntimeHook{Hook: hook, secret: secret})
	}
	return hooks, nil
}

func AddHook(path string, hook Hook) (string, error) {
	if hook.Enabled == false {
		hook.Enabled = true
	}
	if err := validateHook(hook); err != nil {
		return "", err
	}
	config := Config{Version: configVersion}
	if content, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(content, &config); err != nil {
			return "", fmt.Errorf("could not parse webhook config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("could not read webhook config: %w", err)
	}
	if config.Version == 0 {
		config.Version = configVersion
	}
	if err := validateConfigVersion(config.Version); err != nil {
		return "", err
	}
	if err := validateConfig(config); err != nil {
		return "", err
	}
	for _, existing := range config.Hooks {
		if existing.Name == hook.Name {
			return "", fmt.Errorf("webhook %q already exists", hook.Name)
		}
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", fmt.Errorf("could not generate webhook secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	if err := writeSecret(hook.SecretFile, []byte(secret+"\n")); err != nil {
		return "", err
	}
	config.Hooks = append(config.Hooks, hook)
	if err := saveConfig(path, config); err != nil {
		return "", err
	}
	return secret, nil
}

func validateConfig(config Config) error {
	if err := validateConfigVersion(config.Version); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(config.Hooks))
	for _, hook := range config.Hooks {
		if err := validateHook(hook); err != nil {
			return err
		}
		if _, exists := seen[hook.Name]; exists {
			return fmt.Errorf("duplicate webhook name %q", hook.Name)
		}
		seen[hook.Name] = struct{}{}
	}
	return nil
}

func validateConfigVersion(version int) error {
	if version != configVersion {
		return fmt.Errorf("unsupported webhook config version %d", version)
	}
	return nil
}

func validateHook(hook Hook) error {
	for field, value := range map[string]string{
		"name": hook.Name, "project": hook.Project, "environment": hook.Environment, "service": hook.Service,
	} {
		if !identifierPattern.MatchString(value) {
			return fmt.Errorf("webhook %s must contain only lowercase letters, digits, and hyphens", field)
		}
	}
	if !filepath.IsAbs(hook.ManifestPath) {
		return fmt.Errorf("webhook %s manifestPath must be absolute", hook.Name)
	}
	if !filepath.IsAbs(hook.SecretFile) {
		return fmt.Errorf("webhook %s secretFile must be absolute", hook.Name)
	}
	if err := validateImagePrefix(hook.ImagePrefix); err != nil {
		return fmt.Errorf("webhook %s imagePrefix: %w", hook.Name, err)
	}
	return nil
}

func validateImagePrefix(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "@ \t\r\n") {
		return fmt.Errorf("must be a non-empty image reference without a digest")
	}
	return nil
}

func saveConfig(path string, config Config) error {
	content, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("could not encode webhook config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("could not create webhook config directory: %w", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("could not write webhook config: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("could not restrict webhook config permissions: %w", err)
	}
	return nil
}

func writeSecret(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("could not create webhook secret directory: %w", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("could not write webhook secret: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("could not restrict webhook secret permissions: %w", err)
	}
	return nil
}
