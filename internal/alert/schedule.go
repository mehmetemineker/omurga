package alert

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"omurga/internal/host"
)

type Schedule struct {
	Executable string
	Calendar   string
}

func SchedulePaths(paths host.Paths) []string {
	base := filepath.Join(paths.SystemdUnits, "omurga-alert-monitor")
	return []string{base + ".service", base + ".timer"}
}

func ParseSchedule(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("alert schedule is required")
	}
	if match := regexp.MustCompile(`^(\d{1,2}):(\d{2})$`).FindStringSubmatch(value); match != nil {
		hour, _ := strconv.Atoi(match[1])
		minute, _ := strconv.Atoi(match[2])
		if hour > 23 || minute > 59 {
			return "", fmt.Errorf("alert schedule time is outside the valid 00:00-23:59 range")
		}
		return fmt.Sprintf("*-*-* %02d:%02d:00", hour, minute), nil
	}
	if strings.ContainsAny(value, "\r\n") || !strings.Contains(value, " ") {
		return "", fmt.Errorf("alert schedule must be HH:MM or a systemd calendar expression")
	}
	return value, nil
}

func WriteSchedule(paths host.Paths, schedule Schedule) ([]string, error) {
	if schedule.Executable == "" {
		return nil, fmt.Errorf("alert schedule executable is required")
	}
	calendar, err := ParseSchedule(schedule.Calendar)
	if err != nil {
		return nil, err
	}
	arguments := []string{schedule.Executable, "alert", "check", "--quiet"}
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		quoted[index] = systemdQuote(argument)
	}
	service := "[Unit]\nDescription=Omurga host alert monitor\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=oneshot\nExecStart=" + strings.Join(quoted, " ") + "\n"
	timer := "[Unit]\nDescription=Scheduled Omurga host alert monitor\n\n[Timer]\nOnCalendar=" + calendar + "\nPersistent=true\nRandomizedDelaySec=2m\nUnit=omurga-alert-monitor.service\n\n[Install]\nWantedBy=timers.target\n"
	pathsToWrite := SchedulePaths(paths)
	if err := writeAtomic(pathsToWrite[0], []byte(service), 0o644); err != nil {
		return nil, err
	}
	if err := writeAtomic(pathsToWrite[1], []byte(timer), 0o644); err != nil {
		return nil, err
	}
	return pathsToWrite, nil
}

func systemdQuote(value string) string {
	if !strings.ContainsAny(value, " \t\"") {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".omurga-alert-schedule-*")
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
	return os.Rename(temporaryPath, path)
}
