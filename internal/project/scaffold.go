package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

var projectNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type CreateResult struct {
	Name       string `json:"name"`
	Directory  string `json:"directory"`
	Manifest   string `json:"manifest"`
	Production string `json:"productionEnvironment"`
	DryRun     bool   `json:"dryRun"`
}

func WriteArtifact(filePath string, data []byte, mode fs.FileMode) error {
	absolutePath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("could not resolve output path: %w", err)
	}
	directory := filepath.Dir(absolutePath)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("could not create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".omurga-artifact-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if err := os.Remove(absolutePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not replace existing output file: %w", err)
		}
	}
	if err := os.Rename(temporaryPath, absolutePath); err != nil {
		return fmt.Errorf("could not replace output file: %w", err)
	}
	return nil
}

func Create(name, parent string, dryRun bool) (CreateResult, error) {
	if !projectNamePattern.MatchString(name) {
		return CreateResult{}, fmt.Errorf("project name must contain only lowercase letters, digits, and hyphens")
	}
	if parent == "" {
		parent = "."
	}
	absoluteParent, err := filepath.Abs(parent)
	if err != nil {
		return CreateResult{}, fmt.Errorf("could not resolve project parent directory: %w", err)
	}
	directory := filepath.Join(absoluteParent, name)
	result := CreateResult{
		Name:       name,
		Directory:  directory,
		Manifest:   filepath.Join(directory, "omurga.yaml"),
		Production: filepath.Join(directory, "environments", "production.yaml"),
		DryRun:     dryRun,
	}
	if _, err := os.Stat(directory); err == nil {
		return CreateResult{}, fmt.Errorf("project directory already exists: %s", directory)
	} else if !os.IsNotExist(err) {
		return CreateResult{}, err
	}
	if dryRun {
		return result, nil
	}

	if err := os.MkdirAll(filepath.Dir(result.Production), 0o755); err != nil {
		return CreateResult{}, fmt.Errorf("could not create project directory: %w", err)
	}
	base := fmt.Sprintf(`version: 1

name: %s

services:
  app:
    image: nginx:alpine
    pullPolicy: if-not-present
    expose:
      - 80
    environment:
      APP_ENV: development
    resources:
      cpus: "0.25"
      memory: 64M
    logging:
      driver: json-file
      maxSize: 20M
      maxFiles: 5
    healthcheck:
      command: [CMD, wget, --spider, http://localhost/]
      interval: 30s
      timeout: 5s
      retries: 3

gateway:
  routes:
    - domain: %s.localhost
      service: app
      port: 80
      https: false
`, name, name)
	production := fmt.Sprintf(`host: production

services:
  app:
    environment:
      APP_ENV: production

gateway:
  routes:
    - domain: %s.example.com
      service: app
      port: 80
      https: true
`, name)
	if err := WriteArtifact(result.Manifest, []byte(base), 0o640); err != nil {
		return CreateResult{}, fmt.Errorf("could not write project manifest: %w", err)
	}
	if err := WriteArtifact(result.Production, []byte(production), 0o640); err != nil {
		return CreateResult{}, fmt.Errorf("could not write production environment: %w", err)
	}
	return result, nil
}
