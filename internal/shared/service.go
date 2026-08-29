package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"

	"omurga/internal/host"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type CatalogEntry struct {
	Image               string   `json:"image"`
	DataTarget          string   `json:"dataTarget"`
	RequiresEnvironment bool     `json:"requiresEnvironment"`
	Command             []string `json:"command,omitempty"`
}

func Catalog() map[string]CatalogEntry {
	return map[string]CatalogEntry{
		"postgres": {Image: "postgres:16", DataTarget: "/var/lib/postgresql/data", RequiresEnvironment: true},
		"redis":    {Image: "redis:7.4", DataTarget: "/data", Command: []string{"redis-server", "--appendonly", "yes"}},
	}
}

type Compose struct {
	Services map[string]Service `yaml:"services"`
	Networks map[string]Network `yaml:"networks"`
}

type Service struct {
	Image       string   `yaml:"image"`
	Restart     string   `yaml:"restart"`
	Command     []string `yaml:"command,omitempty"`
	Environment []string `yaml:"env_file,omitempty"`
	Volumes     []string `yaml:"volumes"`
	Networks    []string `yaml:"networks"`
}

type Network struct {
	External bool   `yaml:"external"`
	Name     string `yaml:"name"`
}

func Layout(paths host.Paths, name string) (root, compose, data string, err error) {
	if !namePattern.MatchString(name) {
		return "", "", "", fmt.Errorf("shared service name %q is invalid", name)
	}
	root = filepath.Join(paths.SharedServices, name)
	return root, filepath.Join(root, "compose.yaml"), filepath.Join(root, "data"), nil
}

func Generate(kind, name, image, environmentFile string, paths host.Paths) ([]byte, string, error) {
	entry, exists := Catalog()[kind]
	if !exists && image == "" {
		return nil, "", fmt.Errorf("unknown catalog service %s; --image is required for a custom service", kind)
	}
	if image == "" {
		image = entry.Image
	}
	if entry.DataTarget == "" {
		entry.DataTarget = "/data"
	}
	if entry.RequiresEnvironment && environmentFile == "" {
		return nil, "", fmt.Errorf("shared %s requires --environment-file", kind)
	}
	_, _, data, err := Layout(paths, name)
	if err != nil {
		return nil, "", err
	}
	service := Service{Image: image, Restart: "unless-stopped", Command: entry.Command, Volumes: []string{filepath.ToSlash(data) + ":" + entry.DataTarget}, Networks: []string{"omurga-shared"}}
	if environmentFile != "" {
		service.Environment = []string{filepath.ToSlash(environmentFile)}
	}
	compose := Compose{Services: map[string]Service{name: service}, Networks: map[string]Network{"omurga-shared": {External: true, Name: "omurga-shared"}}}
	content, err := yaml.Marshal(compose)
	if err != nil {
		return nil, "", err
	}
	return content, data, nil
}

func List(paths host.Paths) ([]string, error) {
	entries, err := os.ReadDir(paths.SharedServices)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			if _, err := os.Stat(filepath.Join(paths.SharedServices, entry.Name(), "compose.yaml")); err == nil {
				names = append(names, entry.Name())
			}
		}
	}
	sort.Strings(names)
	return names, nil
}
