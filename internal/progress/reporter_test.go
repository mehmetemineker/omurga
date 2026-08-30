package progress

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlainReporterWritesLifecycle(t *testing.T) {
	var output bytes.Buffer
	reporter := New(&output, ModePlain)
	task := reporter.Start("Install Docker")
	task.Update("Downloading packages")
	task.Complete()

	text := output.String()
	for _, expected := range []string{"→ Install Docker", "  Downloading packages", "✓ Install Docker"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in %q", expected, text)
		}
	}
}

func TestOffReporterWritesNothing(t *testing.T) {
	var output bytes.Buffer
	task := New(&output, ModeOff).Start("No output")
	task.Update("Still no output")
	task.Complete()
	if output.Len() != 0 {
		t.Fatalf("expected no output, got %q", output.String())
	}
}

func TestParseMode(t *testing.T) {
	if _, err := ParseMode("invalid"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}
