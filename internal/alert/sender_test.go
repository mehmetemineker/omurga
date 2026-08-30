package alert

import (
	"path/filepath"
	"testing"
)

func TestCompareMonitorStateOnlyReportsChanges(t *testing.T) {
	issue := MonitorIssue{Name: "disk", Status: "critical", Message: "disk usage is 95%"}
	first := CompareMonitorState(MonitorState{}, []MonitorIssue{issue})
	if len(first.NewIssues) != 1 || len(first.Resolved) != 0 {
		t.Fatalf("unexpected first monitor delta: %#v", first)
	}
	second := CompareMonitorState(first.NextState, []MonitorIssue{issue})
	if len(second.NewIssues) != 0 || len(second.Resolved) != 0 {
		t.Fatalf("unchanged monitor issue was reported again: %#v", second)
	}
	resolved := CompareMonitorState(second.NextState, nil)
	if len(resolved.Resolved) != 1 || resolved.Resolved[0] != "disk" {
		t.Fatalf("unexpected resolved monitor delta: %#v", resolved)
	}
}

func TestMonitorStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := MonitorState{Issues: map[string]string{"services": "fingerprint"}}
	if err := SaveMonitorState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMonitorState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Issues["services"] != want.Issues["services"] {
		t.Fatalf("monitor state = %#v, want %#v", got, want)
	}
}

func TestParseSchedule(t *testing.T) {
	got, err := ParseSchedule("03:05")
	if err != nil || got != "*-*-* 03:05:00" {
		t.Fatalf("ParseSchedule() = %q, %v", got, err)
	}
	if _, err := ParseSchedule("25:00"); err == nil {
		t.Fatal("ParseSchedule() accepted an invalid time")
	}
}
