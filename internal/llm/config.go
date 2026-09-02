package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

const configVersion = 1

// Settings contains non-secret remote LLM settings. API keys are deliberately
// excluded and must be provided through the environment or a file.
type Settings struct {
	Version  int    `yaml:"version" json:"version"`
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	Model    string `yaml:"model" json:"model"`
}

// Config is the resolved client configuration.
type Config struct {
	Endpoint string
	Model    string
	APIKey   string
}

func DefaultConfigPath() (string, error) {
	root := os.Getenv("OMURGA_CONFIG_HOME")
	if root == "" {
		var err error
		root, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(root, "omurga")
	}
	return filepath.Join(root, "ai.yaml"), nil
}

func LoadSettings() (Settings, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return Settings{}, err
	}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Settings{Version: configVersion}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("could not read LLM configuration: %w", err)
	}
	var settings Settings
	decoder := yaml.NewDecoder(strings.NewReader(string(content)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&settings); err != nil {
		return Settings{}, fmt.Errorf("could not decode LLM configuration: %w", err)
	}
	if settings.Version != configVersion {
		return Settings{}, fmt.Errorf("unsupported LLM configuration version %d", settings.Version)
	}
	return settings, nil
}

func SaveSettings(settings Settings) (string, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return "", err
	}
	settings.Version = configVersion
	settings.Endpoint = strings.TrimSpace(settings.Endpoint)
	settings.Model = strings.TrimSpace(settings.Model)
	if settings.Endpoint == "" || settings.Model == "" {
		return "", fmt.Errorf("LLM endpoint and model are required")
	}
	content, err := yaml.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("could not encode LLM configuration: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("could not create LLM configuration directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".omurga-ai-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return "", err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return "", fmt.Errorf("could not save LLM configuration: %w", err)
	}
	return path, nil
}

func ResolveConfig(apiKeyFile string) (Config, error) {
	settings, err := LoadSettings()
	if err != nil {
		return Config{}, err
	}
	if value := strings.TrimSpace(os.Getenv("OMURGA_LLM_ENDPOINT")); value != "" {
		settings.Endpoint = value
	}
	if value := strings.TrimSpace(os.Getenv("OMURGA_LLM_MODEL")); value != "" {
		settings.Model = value
	}
	apiKey := strings.TrimSpace(os.Getenv("OMURGA_LLM_API_KEY"))
	if apiKeyFile == "" {
		apiKeyFile = strings.TrimSpace(os.Getenv("OMURGA_LLM_API_KEY_FILE"))
	}
	if apiKeyFile != "" {
		info, err := os.Stat(apiKeyFile)
		if err != nil {
			return Config{}, fmt.Errorf("could not inspect LLM API key file: %w", err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return Config{}, fmt.Errorf("LLM API key file must not be readable by group or other users")
		}
		content, err := os.ReadFile(apiKeyFile)
		if err != nil {
			return Config{}, fmt.Errorf("could not read LLM API key file: %w", err)
		}
		apiKey = strings.TrimSpace(string(content))
	}
	if settings.Endpoint == "" {
		return Config{}, fmt.Errorf("LLM endpoint is not configured; run `omurga ai configure` or set OMURGA_LLM_ENDPOINT")
	}
	if settings.Model == "" {
		return Config{}, fmt.Errorf("LLM model is not configured; run `omurga ai configure` or set OMURGA_LLM_MODEL")
	}
	if apiKey == "" {
		return Config{}, fmt.Errorf("LLM API key is not configured; set OMURGA_LLM_API_KEY or OMURGA_LLM_API_KEY_FILE")
	}
	return Config{Endpoint: settings.Endpoint, Model: settings.Model, APIKey: apiKey}, nil
}
