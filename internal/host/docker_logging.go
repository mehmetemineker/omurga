package host

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	dockerLogDriver = "local"
	dockerLogSize   = "10m"
	dockerLogFiles  = "3"
)

func dockerLogRotationConfigured(path string) (bool, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var config map[string]any
	if err := json.Unmarshal(content, &config); err != nil {
		return false, fmt.Errorf("could not parse Docker daemon configuration %s: %w", path, err)
	}
	options, ok := config["log-opts"].(map[string]any)
	if !ok {
		return false, nil
	}
	return config["log-driver"] == dockerLogDriver &&
		options["max-size"] == dockerLogSize &&
		options["max-file"] == dockerLogFiles, nil
}

func ensureDockerLogRotation(path string, dryRun bool) (bool, error) {
	config := make(map[string]any)
	if content, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(content, &config); err != nil {
			return false, fmt.Errorf("could not parse Docker daemon configuration %s: %w", path, err)
		}
		if config == nil {
			config = make(map[string]any)
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}

	options, ok := config["log-opts"].(map[string]any)
	if !ok {
		options = make(map[string]any)
	}
	config["log-driver"] = dockerLogDriver
	options["max-size"] = dockerLogSize
	options["max-file"] = dockerLogFiles
	config["log-opts"] = options

	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return false, fmt.Errorf("could not encode Docker daemon configuration: %w", err)
	}
	content = append(content, '\n')
	return ensureFile(path, content, 0o644, dryRun)
}
