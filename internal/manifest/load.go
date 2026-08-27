package manifest

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

var environmentNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func Load(path, environment string) (LoadedProject, error) {
	manifestPath, err := resolveManifestPath(path)
	if err != nil {
		return LoadedProject{}, err
	}

	base, err := readMap(manifestPath)
	if err != nil {
		return LoadedProject{}, fmt.Errorf("could not read manifest: %w", err)
	}

	merged := base
	if environment != "" {
		if !environmentNamePattern.MatchString(environment) {
			return LoadedProject{}, fmt.Errorf("invalid environment name %q", environment)
		}

		overlayPath := filepath.Join(filepath.Dir(manifestPath), "environments", environment+".yaml")
		overlay, err := readMap(overlayPath)
		if err != nil {
			return LoadedProject{}, fmt.Errorf("could not read environment %s: %w", environment, err)
		}
		if _, exists := overlay["version"]; exists {
			return LoadedProject{}, fmt.Errorf("an environment file cannot override version")
		}
		if _, exists := overlay["name"]; exists {
			return LoadedProject{}, fmt.Errorf("an environment file cannot override name")
		}
		merged = mergeMaps(base, overlay)
	}

	data, err := yaml.Marshal(merged)
	if err != nil {
		return LoadedProject{}, fmt.Errorf("could not marshal merged manifest: %w", err)
	}

	var project Project
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&project); err != nil {
		return LoadedProject{}, fmt.Errorf("invalid manifest schema: %w", err)
	}
	if err := ensureSingleDocument(decoder); err != nil {
		return LoadedProject{}, err
	}
	if err := Validate(project); err != nil {
		return LoadedProject{}, err
	}

	absolutePath, err := filepath.Abs(manifestPath)
	if err != nil {
		return LoadedProject{}, fmt.Errorf("could not resolve manifest path: %w", err)
	}

	return LoadedProject{
		Project:     project,
		Path:        absolutePath,
		Environment: environment,
	}, nil
}

func resolveManifestPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("project path not found: %w", err)
	}
	if info.IsDir() {
		path = filepath.Join(path, "omurga.yaml")
	}
	if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
		return "", fmt.Errorf("manifest extension must be .yaml or .yml: %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("manifest not found: %w", err)
	}
	return filepath.Clean(path), nil
}

func readMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var value map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("file is empty")
	}
	if err := ensureSingleDocument(decoder); err != nil {
		return nil, err
	}
	return value, nil
}

func ensureSingleDocument(decoder *yaml.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("could not read YAML document: %w", err)
	}
	return fmt.Errorf("a file can contain only one YAML document")
}

func mergeMaps(base, overlay map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(overlay))
	for key, value := range base {
		result[key] = value
	}

	for key, overlayValue := range overlay {
		baseMap, baseIsMap := result[key].(map[string]any)
		overlayMap, overlayIsMap := overlayValue.(map[string]any)
		if baseIsMap && overlayIsMap {
			result[key] = mergeMaps(baseMap, overlayMap)
			continue
		}
		result[key] = overlayValue
	}
	return result
}
