package project

import (
	"fmt"
	"hash/crc32"
	"path"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"omurga/internal/manifest"
)

const (
	GatewayPortStart = 20000
	GatewayPortEnd   = 29999
)

type RenderOptions struct {
	Environment        string
	DeploymentRoot     string
	RuntimeSecretsRoot string
	Ports              map[string]int
}

type Artifacts struct {
	Compose []byte
	Caddy   []byte
	Ports   map[string]int
}

func EnvironmentKey(environment string) string {
	if environment == "" {
		return "default"
	}
	return environment
}

func ComposeProjectName(projectName, environment string) string {
	return "omurga-" + projectName + "-" + EnvironmentKey(environment)
}

func PortKey(service string, containerPort int) string {
	return fmt.Sprintf("%s:%d", service, containerPort)
}

func PreviewPorts(project manifest.Project, environment string) map[string]int {
	ports := make(map[string]int)
	used := make(map[int]bool)
	rangeSize := GatewayPortEnd - GatewayPortStart + 1
	for _, route := range project.Gateway.Routes {
		key := PortKey(route.Service, route.Port)
		if _, exists := ports[key]; exists {
			continue
		}
		seed := project.Name + ":" + EnvironmentKey(environment) + ":" + key
		candidate := GatewayPortStart + int(crc32.ChecksumIEEE([]byte(seed))%uint32(rangeSize))
		for used[candidate] {
			candidate++
			if candidate > GatewayPortEnd {
				candidate = GatewayPortStart
			}
		}
		ports[key] = candidate
		used[candidate] = true
	}
	return ports
}

func DefaultRenderOptions(project manifest.Project, environment string) RenderOptions {
	environmentKey := EnvironmentKey(environment)
	return RenderOptions{
		Environment:        environment,
		DeploymentRoot:     path.Join("/var/lib/omurga/projects", project.Name, environmentKey),
		RuntimeSecretsRoot: path.Join("/run/omurga/secrets", project.Name, environmentKey),
		Ports:              PreviewPorts(project, environment),
	}
}

func Generate(project manifest.Project, options RenderOptions) (Artifacts, error) {
	if err := manifest.Validate(project); err != nil {
		return Artifacts{}, err
	}
	if options.DeploymentRoot == "" || options.RuntimeSecretsRoot == "" {
		return Artifacts{}, fmt.Errorf("deployment and runtime secret roots are required")
	}
	if options.Ports == nil {
		options.Ports = PreviewPorts(project, options.Environment)
	}

	compose := ComposeFile{
		Services: make(map[string]ComposeService),
		Secrets:  make(map[string]ComposeSecret),
	}
	dependencies := make(map[string]ComposeDependency)

	dependencyNames := sortedKeys(project.Dependencies)
	for _, name := range dependencyNames {
		dependency := project.Dependencies[name]
		if dependency.Mode == "shared" {
			return Artifacts{}, fmt.Errorf("shared dependency %s is not supported by the project renderer yet", name)
		}
		service, secrets, err := renderDependency(name, dependency, options)
		if err != nil {
			return Artifacts{}, err
		}
		compose.Services[name] = service
		for secretName, secret := range secrets {
			compose.Secrets[secretName] = secret
		}
		dependencies[name] = ComposeDependency{Condition: "service_healthy", Restart: true}
	}

	serviceNames := sortedKeys(project.Services)
	for _, name := range serviceNames {
		service := project.Services[name]
		generated := ComposeService{
			Image:       service.Image,
			PullPolicy:  composePullPolicy(service.PullPolicy),
			Restart:     firstNonEmpty(service.Restart, "unless-stopped"),
			Command:     service.Command,
			Expose:      service.Expose,
			Environment: service.Environment,
			CPUs:        service.Resources.CPUs,
			Memory:      service.Resources.Memory,
			PIDs:        service.Resources.PIDs,
			DependsOn:   cloneDependencies(dependencies),
			Labels: map[string]string{
				"dev.omurga.managed":     "true",
				"dev.omurga.project":     project.Name,
				"dev.omurga.environment": EnvironmentKey(options.Environment),
			},
		}

		for _, volume := range service.Volumes {
			generated.Volumes = append(generated.Volumes, ComposeVolume{
				Type:   "bind",
				Source: path.Join(options.DeploymentRoot, "data", volume.Name),
				Target: volume.Target,
			})
		}
		for _, secret := range service.Secrets {
			generated.Secrets = append(generated.Secrets, ComposeServiceSecret{
				Source: secret.Name,
				Target: secret.Target,
			})
			compose.Secrets[secret.Name] = ComposeSecret{File: path.Join(options.RuntimeSecretsRoot, secret.Name)}
		}
		for _, route := range project.Gateway.Routes {
			if route.Service != name {
				continue
			}
			hostPort, exists := options.Ports[PortKey(name, route.Port)]
			if !exists {
				return Artifacts{}, fmt.Errorf("gateway port assignment is missing for %s:%d", name, route.Port)
			}
			if !containsPublishedPort(generated.Ports, route.Port) {
				generated.Ports = append(generated.Ports, ComposePort{
					Name:      fmt.Sprintf("gateway-%d", route.Port),
					Target:    route.Port,
					Published: strconv.Itoa(hostPort),
					HostIP:    "127.0.0.1",
					Protocol:  "tcp",
				})
			}
		}
		if service.Logging.Driver != "" {
			optionsMap := make(map[string]string)
			if service.Logging.MaxSize != "" {
				optionsMap["max-size"] = service.Logging.MaxSize
			}
			if service.Logging.MaxFiles > 0 {
				optionsMap["max-file"] = strconv.Itoa(service.Logging.MaxFiles)
			}
			generated.Logging = &ComposeLogging{Driver: service.Logging.Driver, Options: optionsMap}
		}
		if len(service.Healthcheck.Command) > 0 {
			generated.Healthcheck = &ComposeHealthcheck{
				Test:        service.Healthcheck.Command,
				Interval:    service.Healthcheck.Interval,
				Timeout:     service.Healthcheck.Timeout,
				Retries:     service.Healthcheck.Retries,
				StartPeriod: service.Healthcheck.StartPeriod,
			}
		}
		compose.Services[name] = generated
	}

	if len(compose.Secrets) == 0 {
		compose.Secrets = nil
	}
	composeData, err := yaml.Marshal(compose)
	if err != nil {
		return Artifacts{}, fmt.Errorf("could not marshal Compose file: %w", err)
	}
	caddyData, err := renderCaddy(project, options)
	if err != nil {
		return Artifacts{}, err
	}
	return Artifacts{Compose: composeData, Caddy: caddyData, Ports: options.Ports}, nil
}

func renderDependency(name string, dependency manifest.Dependency, options RenderOptions) (ComposeService, map[string]ComposeSecret, error) {
	secrets := make(map[string]ComposeSecret)
	dataRoot := path.Join(options.DeploymentRoot, "data", "dependencies", name)
	labels := map[string]string{
		"dev.omurga.managed":     "true",
		"dev.omurga.environment": EnvironmentKey(options.Environment),
		"dev.omurga.dependency":  dependency.Type,
	}

	switch dependency.Type {
	case "postgres":
		secretTarget := "/run/secrets/" + dependency.PasswordSecret
		secrets[dependency.PasswordSecret] = ComposeSecret{File: path.Join(options.RuntimeSecretsRoot, dependency.PasswordSecret)}
		return ComposeService{
			Image:      "postgres:" + dependency.Version,
			PullPolicy: "missing",
			Restart:    "unless-stopped",
			Environment: map[string]string{
				"POSTGRES_DB":            dependency.Database,
				"POSTGRES_USER":          dependency.User,
				"POSTGRES_PASSWORD_FILE": secretTarget,
			},
			Secrets: []ComposeServiceSecret{{
				Source: dependency.PasswordSecret,
				Target: secretTarget,
			}},
			Volumes: []ComposeVolume{{Type: "bind", Source: dataRoot, Target: "/var/lib/postgresql/data"}},
			Healthcheck: &ComposeHealthcheck{
				Test:     []string{"CMD-SHELL", fmt.Sprintf("pg_isready -U %s -d %s", dependency.User, dependency.Database)},
				Interval: "10s",
				Timeout:  "5s",
				Retries:  5,
			},
			Labels: labels,
		}, secrets, nil
	case "redis":
		command := []string{"redis-server"}
		volumes := []ComposeVolume(nil)
		switch firstNonEmpty(dependency.Persistence, "aof") {
		case "aof":
			command = append(command, "--appendonly", "yes")
			volumes = append(volumes, ComposeVolume{Type: "bind", Source: dataRoot, Target: "/data"})
		case "rdb":
			command = append(command, "--save", "60", "1", "--appendonly", "no")
			volumes = append(volumes, ComposeVolume{Type: "bind", Source: dataRoot, Target: "/data"})
		case "none":
			command = append(command, "--save", "", "--appendonly", "no")
		}
		if dependency.MaxMemory != "" {
			command = append(command, "--maxmemory", dependency.MaxMemory)
		}
		if dependency.EvictionPolicy != "" {
			command = append(command, "--maxmemory-policy", dependency.EvictionPolicy)
		}
		return ComposeService{
			Image:       "redis:" + dependency.Version,
			PullPolicy:  "missing",
			Restart:     "unless-stopped",
			Command:     command,
			Volumes:     volumes,
			Healthcheck: &ComposeHealthcheck{Test: []string{"CMD", "redis-cli", "ping"}, Interval: "10s", Timeout: "5s", Retries: 5},
			Labels:      labels,
		}, secrets, nil
	default:
		return ComposeService{}, nil, fmt.Errorf("unsupported dependency type %q", dependency.Type)
	}
}

func renderCaddy(project manifest.Project, options RenderOptions) ([]byte, error) {
	if len(project.Gateway.Routes) == 0 {
		return nil, nil
	}
	var builder strings.Builder
	builder.WriteString("# Generated by Omurga. Do not edit.\n\n")
	for _, route := range project.Gateway.Routes {
		hostPort, exists := options.Ports[PortKey(route.Service, route.Port)]
		if !exists {
			return nil, fmt.Errorf("gateway port assignment is missing for %s:%d", route.Service, route.Port)
		}
		address := route.Domain
		if route.HTTPS != nil && !*route.HTTPS {
			address = "http://" + address
		}
		fmt.Fprintf(&builder, "%s {\n", address)
		builder.WriteString("    encode zstd gzip\n")
		fmt.Fprintf(&builder, "    reverse_proxy 127.0.0.1:%d\n", hostPort)
		builder.WriteString("}\n\n")
	}
	return []byte(builder.String()), nil
}

func composePullPolicy(policy string) string {
	switch policy {
	case "if-not-present", "":
		return "missing"
	default:
		return policy
	}
}

func cloneDependencies(input map[string]ComposeDependency) map[string]ComposeDependency {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]ComposeDependency, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func containsPublishedPort(ports []ComposePort, target int) bool {
	for _, port := range ports {
		if port.Target == target {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
