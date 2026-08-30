package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"omurga/internal/host"
	"omurga/internal/progress"
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Manager struct {
	Runner          host.Runner
	Repository      string
	PasswordFile    string
	EnvironmentFile string
	Progress        *progress.Reporter
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
	if m.Progress != nil && (action == "backup" || action == "restore") {
		if runner, ok := m.Runner.(host.EnvironmentIORunner); ok {
			return m.runWithJSONProgress(ctx, runner, environment, action, command)
		}
	}
	task := m.Progress.Start("Restic " + action)
	if runner, ok := m.Runner.(host.EnvironmentRunner); ok {
		output, runErr := runner.RunEnvironment(ctx, environment, "restic", command...)
		if runErr != nil {
			task.Fail(runErr)
			return output, runErr
		}
		task.Complete()
		return output, nil
	}
	if len(environment) > 0 {
		task.Fail(fmt.Errorf("backup environment files are not supported by the command runner"))
		return "", fmt.Errorf("backup environment files are not supported by the command runner")
	}
	output, runErr := m.Runner.Run(ctx, "restic", command...)
	if runErr != nil {
		task.Fail(runErr)
		return output, runErr
	}
	task.Complete()
	return output, nil
}

func (m Manager) runWithJSONProgress(ctx context.Context, runner host.EnvironmentIORunner, environment map[string]string, action string, command []string) (string, error) {
	command = append(command[:4:4], append([]string{"--json"}, command[4:]...)...)
	task := m.Progress.Start("Restic " + action)
	reader, writer := io.Pipe()
	var stderr bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		err := runner.RunEnvironmentIO(ctx, environment, nil, writer, &stderr, "restic", command...)
		_ = writer.CloseWithError(err)
		errCh <- err
	}()

	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		if update := resticProgressUpdate(scanner.Bytes()); update != "" {
			task.Update(update)
		}
	}
	runErr := <-errCh
	if scanErr := scanner.Err(); scanErr != nil && runErr == nil {
		runErr = scanErr
	}
	if runErr != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			runErr = fmt.Errorf("%w: %s", runErr, message)
		}
		task.Fail(runErr)
		return "", runErr
	}
	task.Complete()
	return "Restic " + action + " completed", nil
}

type resticStatus struct {
	MessageType      string  `json:"message_type"`
	PercentDone      float64 `json:"percent_done"`
	BytesDone        uint64  `json:"bytes_done"`
	TotalBytes       uint64  `json:"total_bytes"`
	FilesDone        uint64  `json:"files_done"`
	TotalFiles       uint64  `json:"total_files"`
	SecondsRemaining float64 `json:"seconds_remaining"`
	BytesRestored    uint64  `json:"bytes_restored"`
}

func resticProgressUpdate(line []byte) string {
	var status resticStatus
	if err := json.Unmarshal(line, &status); err != nil || status.MessageType != "status" {
		return ""
	}
	done, total := status.BytesDone, status.TotalBytes
	if status.BytesRestored > 0 {
		done = status.BytesRestored
	}
	if total == 0 {
		return ""
	}
	percent := status.PercentDone * 100
	if percent == 0 && done > 0 {
		percent = float64(done) * 100 / float64(total)
	}
	message := fmt.Sprintf("%.0f%% · %s / %s", percent, humanBytes(done), humanBytes(total))
	if status.TotalFiles > 0 {
		message += fmt.Sprintf(" · %d / %d files", status.FilesDone, status.TotalFiles)
	}
	if status.SecondsRemaining >= 1 {
		message += " · ETA " + time.Duration(status.SecondsRemaining*float64(time.Second)).Round(time.Second).String()
	}
	return message
}

func humanBytes(value uint64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	amount := float64(value)
	index := 0
	for amount >= 1024 && index < len(units)-1 {
		amount /= 1024
		index++
	}
	if index == 0 {
		return fmt.Sprintf("%d %s", value, units[index])
	}
	return fmt.Sprintf("%.1f %s", amount, units[index])
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
