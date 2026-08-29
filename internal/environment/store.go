package environment

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"gopkg.in/yaml.v3"
)

func Directory(manifestPath string) string {
	return filepath.Join(filepath.Dir(manifestPath), "environments")
}

func Path(manifestPath, name string) (string, error) {
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		return "", fmt.Errorf("invalid environment name %q", name)
	}
	return filepath.Join(Directory(manifestPath), name+".yaml"), nil
}

func List(manifestPath string) ([]string, error) {
	entries, err := os.ReadDir(Directory(manifestPath))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not list environments: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".yaml" {
			names = append(names, entry.Name()[:len(entry.Name())-len(".yaml")])
		}
	}
	sort.Strings(names)
	return names, nil
}

func Set(manifestPath, environment, service, key, value string) (string, error) {
	path, document, err := load(manifestPath, environment)
	if err != nil {
		return "", err
	}
	services := childMap(document, "services")
	serviceValues := childMap(services, service)
	values := childMap(serviceValues, "environment")
	values[key] = value
	return path, save(path, document)
}

func Unset(manifestPath, environment, service, key string) (string, bool, error) {
	path, document, err := load(manifestPath, environment)
	if err != nil {
		return "", false, err
	}
	services, ok := document["services"].(map[string]any)
	if !ok {
		return path, false, nil
	}
	serviceValues, ok := services[service].(map[string]any)
	if !ok {
		return path, false, nil
	}
	values, ok := serviceValues["environment"].(map[string]any)
	if !ok {
		return path, false, nil
	}
	if _, exists := values[key]; !exists {
		return path, false, nil
	}
	delete(values, key)
	return path, true, save(path, document)
}

func Read(manifestPath, environment string) ([]byte, error) {
	path, err := Path(manifestPath, environment)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read environment %s: %w", environment, err)
	}
	return content, nil
}

func load(manifestPath, environment string) (string, map[string]any, error) {
	path, err := Path(manifestPath, environment)
	if err != nil {
		return "", nil, err
	}
	document := map[string]any{}
	content, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(content, &document); err != nil {
			return "", nil, fmt.Errorf("could not decode environment %s: %w", environment, err)
		}
	} else if !os.IsNotExist(err) {
		return "", nil, fmt.Errorf("could not read environment %s: %w", environment, err)
	}
	return path, document, nil
}

func childMap(parent map[string]any, key string) map[string]any {
	if child, ok := parent[key].(map[string]any); ok {
		return child
	}
	child := map[string]any{}
	parent[key] = child
	return child
}

func save(path string, document map[string]any) error {
	content, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("could not encode environment: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("could not create environment directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".omurga-env-*")
	if err != nil {
		return fmt.Errorf("could not create temporary environment file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o640); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("could not replace environment file: %w", err)
	}
	return nil
}
