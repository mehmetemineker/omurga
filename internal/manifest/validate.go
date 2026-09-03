package manifest

import (
	"fmt"
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type Issue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type ValidationError struct {
	Issues []Issue `json:"issues"`
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		parts = append(parts, fmt.Sprintf("%s: %s", issue.Path, issue.Message))
	}
	return "invalid manifest: " + strings.Join(parts, "; ")
}

func Validate(project Project) error {
	issues := make([]Issue, 0)
	add := func(path, message string) {
		issues = append(issues, Issue{Path: path, Message: message})
	}

	if project.Version != 1 {
		add("version", "only version: 1 is supported")
	}
	if !identifierPattern.MatchString(project.Name) {
		add("name", "must contain only lowercase letters, digits, and hyphens")
	}
	if len(project.Services) == 0 {
		add("services", "at least one service must be defined")
	}
	if project.Gateway.Email != "" {
		parsed, err := mail.ParseAddress(project.Gateway.Email)
		if err != nil || parsed.Address != project.Gateway.Email {
			add("gateway.email", "must be a valid email address")
		}
	}

	serviceNames := sortedKeys(project.Services)
	secretDefinitions := map[string]SecretMount{}
	for _, name := range serviceNames {
		service := project.Services[name]
		path := "services." + name
		if !identifierPattern.MatchString(name) {
			add(path, "service name must contain only lowercase letters, digits, and hyphens")
		}
		if strings.TrimSpace(service.Image) == "" {
			add(path+".image", "image is required")
		}
		if service.PullPolicy != "" && service.PullPolicy != "always" && service.PullPolicy != "if-not-present" && service.PullPolicy != "never" {
			add(path+".pullPolicy", "must be always, if-not-present, or never")
		}
		if service.Restart != "" && service.Restart != "no" && service.Restart != "always" && service.Restart != "on-failure" && service.Restart != "unless-stopped" {
			add(path+".restart", "must be no, always, on-failure, or unless-stopped")
		}

		exposed := make(map[int]struct{}, len(service.Expose))
		for i, port := range service.Expose {
			if port < 1 || port > 65535 {
				add(fmt.Sprintf("%s.expose[%d]", path, i), "port must be between 1 and 65535")
			}
			if _, exists := exposed[port]; exists {
				add(fmt.Sprintf("%s.expose[%d]", path, i), "the same port cannot be defined more than once")
			}
			exposed[port] = struct{}{}
		}

		secretNames := map[string]struct{}{}
		for i, secret := range service.Secrets {
			secretPath := fmt.Sprintf("%s.secrets[%d]", path, i)
			if !identifierPattern.MatchString(secret.Name) {
				add(secretPath+".name", "must be a valid secret name")
			}
			if _, exists := secretNames[secret.Name]; exists {
				add(secretPath+".name", "the same secret cannot be mounted more than once")
			}
			secretNames[secret.Name] = struct{}{}
			if !strings.HasPrefix(secret.Target, "/run/secrets/") {
				add(secretPath+".target", "must be under /run/secrets/")
			}
			if secret.Mode != "" {
				mode, err := strconv.ParseUint(secret.Mode, 8, 9)
				if err != nil || mode > 0o777 {
					add(secretPath+".mode", "must be an octal permission between 000 and 777")
				}
			}
			if secret.UID < 0 {
				add(secretPath+".uid", "cannot be negative")
			}
			if secret.GID < 0 {
				add(secretPath+".gid", "cannot be negative")
			}
			if existing, exists := secretDefinitions[secret.Name]; exists && (existing.Mode != secret.Mode || existing.UID != secret.UID || existing.GID != secret.GID) {
				add(secretPath, "the same secret must use identical mode, uid, and gid in every service")
			} else {
				secretDefinitions[secret.Name] = secret
			}
		}

		volumeNames := map[string]struct{}{}
		for i, volume := range service.Volumes {
			volumePath := fmt.Sprintf("%s.volumes[%d]", path, i)
			if !identifierPattern.MatchString(volume.Name) {
				add(volumePath+".name", "must be a valid volume name")
			}
			if _, exists := volumeNames[volume.Name]; exists {
				add(volumePath+".name", "the same volume cannot be mounted more than once")
			}
			volumeNames[volume.Name] = struct{}{}
			if !strings.HasPrefix(volume.Target, "/") {
				add(volumePath+".target", "must be an absolute path inside the container")
			}
		}

		if service.Resources.PIDs < 0 {
			add(path+".resources.pids", "cannot be negative")
		}
		if service.Logging.MaxFiles < 0 {
			add(path+".logging.maxFiles", "cannot be negative")
		}
	}

	domains := map[string]struct{}{}
	for i, route := range project.Gateway.Routes {
		path := fmt.Sprintf("gateway.routes[%d]", i)
		if strings.TrimSpace(route.Domain) == "" || strings.ContainsAny(route.Domain, " /\\") {
			add(path+".domain", "must be a valid domain")
		}
		if _, exists := domains[route.Domain]; exists {
			add(path+".domain", "the same domain cannot be defined more than once")
		}
		domains[route.Domain] = struct{}{}

		service, exists := project.Services[route.Service]
		if !exists {
			add(path+".service", "must reference a defined service")
			continue
		}
		if !containsPort(service.Expose, route.Port) {
			add(path+".port", "must be present in the service expose list")
		}
		for headerIndex, name := range route.ResponseHeaders.Remove {
			if !validHeaderName(name) {
				add(fmt.Sprintf("%s.responseHeaders.remove[%d]", path, headerIndex), "must be a valid HTTP header name")
			}
		}
		for name, value := range route.ResponseHeaders.Set {
			if !validHeaderName(name) {
				add(fmt.Sprintf("%s.responseHeaders.set.%s", path, name), "must be a valid HTTP header name")
			}
			if strings.TrimSpace(value) == "" {
				add(fmt.Sprintf("%s.responseHeaders.set.%s", path, name), "must not be empty")
			} else if strings.ContainsAny(value, "\r\n") {
				add(fmt.Sprintf("%s.responseHeaders.set.%s", path, name), "must not contain carriage returns or line feeds")
			}
		}
	}

	dependencyNames := sortedKeys(project.Dependencies)
	for _, name := range dependencyNames {
		dependency := project.Dependencies[name]
		path := "dependencies." + name
		if _, conflicts := project.Services[name]; conflicts {
			add(path, "dependency name conflicts with a service name")
		}
		if !identifierPattern.MatchString(name) {
			add(path, "dependency name must contain only lowercase letters, digits, and hyphens")
		}
		if dependency.Type != "postgres" && dependency.Type != "redis" {
			add(path+".type", "must be postgres or redis")
		}
		if strings.TrimSpace(dependency.Version) == "" {
			add(path+".version", "version is required")
		}
		mode := dependency.Mode
		if mode == "" {
			mode = "project"
		}
		if mode != "project" && mode != "shared" {
			add(path+".mode", "must be project or shared")
		}
		if mode == "shared" && dependency.Instance == "" {
			add(path+".instance", "instance is required in shared mode")
		}
		if dependency.Type == "redis" && dependency.Persistence != "" && dependency.Persistence != "aof" && dependency.Persistence != "rdb" && dependency.Persistence != "none" {
			add(path+".persistence", "must be aof, rdb, or none")
		}
		if dependency.Type == "postgres" && mode == "project" {
			if dependency.Database == "" {
				add(path+".database", "database is required for a project PostgreSQL instance")
			}
			if dependency.User == "" {
				add(path+".user", "user is required for a project PostgreSQL instance")
			}
			if !identifierPattern.MatchString(dependency.PasswordSecret) {
				add(path+".passwordSecret", "a valid passwordSecret is required for a project PostgreSQL instance")
			}
		}
	}

	if project.Backup.Retention.Daily < 0 || project.Backup.Retention.Weekly < 0 || project.Backup.Retention.Monthly < 0 {
		add("backup.retention", "retention values cannot be negative")
	}
	if project.Backup.Enabled && strings.TrimSpace(project.Backup.Destination) == "" {
		add("backup.destination", "destination is required when backups are enabled")
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, character := range []byte(name) {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9':
		case character == '!' || character == '#' || character == '$' || character == '%' || character == '&' || character == '\'' || character == '*' || character == '+' || character == '-' || character == '.' || character == '^' || character == '_' || character == '`' || character == '|' || character == '~':
		default:
			return false
		}
	}
	return true
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func containsPort(ports []int, expected int) bool {
	for _, port := range ports {
		if port == expected {
			return true
		}
	}
	return false
}
