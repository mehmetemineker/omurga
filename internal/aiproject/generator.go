package aiproject

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"omurga/internal/manifest"
	projectruntime "omurga/internal/project"
)

var environmentNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type CreateResult struct {
	Project     manifest.Project `json:"project"`
	Directory   string           `json:"directory"`
	Manifest    string           `json:"manifest"`
	Environment string           `json:"environment"`
	DryRun      bool             `json:"dryRun"`
}

func ParseManifest(content string) (manifest.Project, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if strings.TrimSpace(lines[len(lines)-1]) == "```" {
				lines = lines[:len(lines)-1]
			}
			content = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	var project manifest.Project
	decoder := json.NewDecoder(bytes.NewBufferString(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&project); err != nil {
		return manifest.Project{}, fmt.Errorf("LLM did not return a valid Omurga manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return manifest.Project{}, fmt.Errorf("LLM returned more than one JSON value")
	} else if err != io.EOF {
		return manifest.Project{}, fmt.Errorf("LLM returned invalid trailing JSON: %w", err)
	}
	if err := manifest.Validate(project); err != nil {
		return manifest.Project{}, fmt.Errorf("LLM generated an invalid manifest: %w", err)
	}
	if err := rejectSecretValues(project); err != nil {
		return manifest.Project{}, err
	}
	return project, nil
}

func rejectSecretValues(project manifest.Project) error {
	for serviceName, service := range project.Services {
		for key, value := range service.Environment {
			upperKey := strings.ToUpper(key)
			if strings.HasSuffix(upperKey, "_FILE") {
				continue
			}
			for _, marker := range []string{"PASSWORD", "TOKEN", "SECRET", "PRIVATE_KEY", "API_KEY"} {
				if strings.Contains(upperKey, marker) && strings.TrimSpace(value) != "" {
					return fmt.Errorf("LLM generated a secret-like value at services.%s.environment.%s; add secrets manually with `omurga secret set`", serviceName, key)
				}
			}
		}
	}
	return nil
}

func CreateProject(project manifest.Project, parent, name, environment string, dryRun bool) (CreateResult, error) {
	if name != "" {
		project.Name = name
	}
	if err := manifest.Validate(project); err != nil {
		return CreateResult{}, err
	}
	if environment != "" && !environmentNamePattern.MatchString(environment) {
		return CreateResult{}, fmt.Errorf("environment name must contain only lowercase letters, digits, and hyphens")
	}
	if parent == "" {
		parent = "."
	}
	parent, err := filepath.Abs(parent)
	if err != nil {
		return CreateResult{}, fmt.Errorf("could not resolve project parent directory: %w", err)
	}
	directory := filepath.Join(parent, project.Name)
	result := CreateResult{
		Project: project, Directory: directory,
		Manifest:    filepath.Join(directory, "omurga.yaml"),
		Environment: environment, DryRun: dryRun,
	}
	if _, err := os.Stat(directory); err == nil {
		return CreateResult{}, fmt.Errorf("project directory already exists: %s", directory)
	} else if !os.IsNotExist(err) {
		return CreateResult{}, err
	}
	if dryRun {
		return result, nil
	}
	content, err := yaml.Marshal(project)
	if err != nil {
		return CreateResult{}, fmt.Errorf("could not encode generated manifest: %w", err)
	}
	if err := projectruntime.WriteArtifact(result.Manifest, content, 0o640); err != nil {
		return CreateResult{}, fmt.Errorf("could not write generated manifest: %w", err)
	}
	if environment != "" {
		environmentPath := filepath.Join(directory, "environments", environment+".yaml")
		if err := projectruntime.WriteArtifact(environmentPath, []byte("host: "+environment+"\n"), 0o640); err != nil {
			return CreateResult{}, fmt.Errorf("could not write generated environment: %w", err)
		}
	}
	return result, nil
}
