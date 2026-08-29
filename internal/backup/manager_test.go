package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omurga/internal/host"
)

func TestLoadEnvironmentFileRequiresRestrictedPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backend.env")
	if err := os.WriteFile(path, []byte("AWS_ACCESS_KEY_ID=test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEnvironmentFile(path); err == nil {
		t.Fatal("expected broad permission error")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := LoadEnvironmentFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["AWS_ACCESS_KEY_ID"] != "test" {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestWriteScheduleDoesNotAddDefaultEnvironmentOverlay(t *testing.T) {
	paths := host.DefaultPaths(t.TempDir())
	files, err := WriteSchedule(paths, Schedule{Name: "demo-default", Executable: "/usr/local/bin/omurga", Manifest: "/srv/demo/omurga.yaml", Repository: "sftp:user@example:/backup", PasswordFile: "/etc/omurga/backup/demo.password", Calendar: "*-*-* 03:00:00"})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), `"--env"`) {
		t.Fatalf("default deployment must not request an overlay: %s", content)
	}
}

func TestParseCalendar(t *testing.T) {
	calendar, err := ParseCalendar("03:15")
	if err != nil || calendar != "*-*-* 03:15:00" {
		t.Fatalf("unexpected calendar: %q %v", calendar, err)
	}
	if _, err := ParseCalendar("25:00"); err == nil {
		t.Fatal("expected invalid time error")
	}
}
