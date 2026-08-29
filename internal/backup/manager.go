package backup

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"omurga/internal/host"
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Manager struct {
	Runner          host.Runner
	Repository      string
	PasswordFile    string
	EnvironmentFile string
}

func (m Manager) ValidateCredentials() error {
	if strings.TrimSpace(m.Repository) == "" || strings.TrimSpace(m.PasswordFile) == "" {
		return fmt.Errorf("Restic repository and password file are required")
	}
	if err := secureCredentialFile(m.PasswordFile, "Restic password"); err != nil {
		return err
	}
	_, err := LoadEnvironmentFile(m.EnvironmentFile)
	return err
}

func (m Manager) Command(action string, args ...string) []string {
	command := []string{"--repo", m.Repository, "--password-file", m.PasswordFile, action}
	return append(command, args...)
}

func (m Manager) Run(ctx context.Context, action string, args ...string) (string, error) {
	if m.Runner == nil {
		return "", fmt.Errorf("backup command runner is required")
	}
	if strings.TrimSpace(m.Repository) == "" {
		return "", fmt.Errorf("Restic repository is required")
	}
	if strings.TrimSpace(m.PasswordFile) == "" {
		return "", fmt.Errorf("Restic password file is required")
	}
	if err := m.ValidateCredentials(); err != nil {
		return "", err
	}
	environment, err := LoadEnvironmentFile(m.EnvironmentFile)
	if err != nil {
		return "", err
	}
	command := m.Command(action, args...)
	if runner, ok := m.Runner.(host.EnvironmentRunner); ok {
		return runner.RunEnvironment(ctx, environment, "restic", command...)
	}
	if len(environment) > 0 {
		return "", fmt.Errorf("backup environment files are not supported by the command runner")
	}
	return m.Runner.Run(ctx, "restic", command...)
}

func LoadEnvironmentFile(path string) (map[string]string, error) {
	values := map[string]string{}
	if path == "" {
		return values, nil
	}
	if err := secureCredentialFile(path, "backup environment"); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open backup environment file: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !environmentNamePattern.MatchString(key) {
			return nil, fmt.Errorf("invalid environment assignment at %s:%d", path, lineNumber)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("could not read backup environment file: %w", err)
	}
	return values, nil
}

func secureCredentialFile(path, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("could not access %s file: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s file is not a regular file", label)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s file permissions are too broad", label)
	}
	return nil
}

type Schedule struct {
	Name            string
	Executable      string
	Manifest        string
	Environment     string
	Repository      string
	PasswordFile    string
	EnvironmentFile string
	Calendar        string
}

func WriteSchedule(paths host.Paths, schedule Schedule) ([]string, error) {
	if schedule.Name == "" || schedule.Executable == "" || schedule.Manifest == "" || schedule.Calendar == "" {
		return nil, fmt.Errorf("schedule name, executable, manifest, and calendar are required")
	}
	unitName := "omurga-backup-" + schedule.Name
	servicePath := filepath.Join(paths.SystemdUnits, unitName+".service")
	timerPath := filepath.Join(paths.SystemdUnits, unitName+".timer")
	arguments := []string{schedule.Executable}
	if schedule.Environment != "" {
		arguments = append(arguments, "--env", schedule.Environment)
	}
	arguments = append(arguments, "backup", "create", schedule.Manifest, "--repository", schedule.Repository, "--password-file", schedule.PasswordFile)
	if schedule.EnvironmentFile != "" {
		arguments = append(arguments, "--environment-file", schedule.EnvironmentFile)
	}
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		quoted[index] = systemdQuote(argument)
	}
	service := "[Unit]\nDescription=Omurga backup for " + schedule.Name + "\nAfter=network-online.target docker.service\nWants=network-online.target\n\n[Service]\nType=oneshot\nExecStart=" + strings.Join(quoted, " ") + "\nNice=10\nIOSchedulingClass=best-effort\nIOSchedulingPriority=7\n"
	timer := "[Unit]\nDescription=Scheduled Omurga backup for " + schedule.Name + "\n\n[Timer]\nOnCalendar=" + schedule.Calendar + "\nPersistent=true\nRandomizedDelaySec=5m\nUnit=" + unitName + ".service\n\n[Install]\nWantedBy=timers.target\n"
	if err := writeAtomic(servicePath, []byte(service), 0o644); err != nil {
		return nil, err
	}
	if err := writeAtomic(timerPath, []byte(timer), 0o644); err != nil {
		return nil, err
	}
	return []string{servicePath, timerPath}, nil
}

func SchedulePaths(paths host.Paths, name string) []string {
	base := filepath.Join(paths.SystemdUnits, "omurga-backup-"+name)
	return []string{base + ".service", base + ".timer"}
}

func ParseCalendar(value string) (string, error) {
	parts := strings.Split(value, ":")
	if len(parts) == 2 && len(parts[0]) == 2 && len(parts[1]) == 2 {
		if parts[0] < "00" || parts[0] > "23" || parts[1] < "00" || parts[1] > "59" {
			return "", fmt.Errorf("backup schedule time is outside the valid 00:00-23:59 range")
		}
		return "*-*-* " + value + ":00", nil
	}
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("backup schedule must be HH:MM or a systemd calendar expression")
	}
	return value, nil
}

func RetentionArguments(daily, weekly, monthly int) []string {
	values := map[string]int{"--keep-daily": daily, "--keep-weekly": weekly, "--keep-monthly": monthly}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var result []string
	for _, key := range keys {
		if values[key] > 0 {
			result = append(result, key, fmt.Sprint(values[key]))
		}
	}
	return result
}

func systemdQuote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`).Replace(value) + `"`
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("could not create unit directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".omurga-unit-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
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
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("could not replace systemd unit: %w", err)
	}
	return nil
}
