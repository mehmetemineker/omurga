// Package progress provides terminal-safe activity reporting for long-running work.
package progress

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type Mode string

const (
	ModeAuto  Mode = "auto"
	ModeTTY   Mode = "tty"
	ModePlain Mode = "plain"
	ModeOff   Mode = "off"
)

type Reporter struct {
	writer io.Writer
	mode   Mode
	mu     sync.Mutex
}

type Task struct {
	reporter *Reporter
	label    string
	started  time.Time
	stop     chan struct{}
	done     sync.Once
	mu       sync.Mutex
	detail   string
	updated  time.Time
	finished bool
}

type reporterContextKey struct{}

func ParseMode(value string) (Mode, error) {
	mode := Mode(strings.ToLower(strings.TrimSpace(value)))
	switch mode {
	case ModeAuto, ModeTTY, ModePlain, ModeOff:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid progress mode %q: expected auto, tty, plain, or off", value)
	}
}

func New(writer io.Writer, mode Mode) *Reporter {
	if mode == ModeAuto {
		mode = ModePlain
		if file, ok := writer.(*os.File); ok {
			if info, err := file.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
				mode = ModeTTY
			}
		}
	}
	return &Reporter{writer: writer, mode: mode}
}

func WithReporter(ctx context.Context, reporter *Reporter) context.Context {
	return context.WithValue(ctx, reporterContextKey{}, reporter)
}

func FromContext(ctx context.Context) *Reporter {
	reporter, _ := ctx.Value(reporterContextKey{}).(*Reporter)
	return reporter
}

func (r *Reporter) Enabled() bool {
	return r != nil && r.mode != ModeOff
}

func (r *Reporter) Start(label string) *Task {
	if !r.Enabled() {
		return &Task{}
	}
	task := &Task{reporter: r, label: label, started: time.Now(), stop: make(chan struct{})}
	if r.mode == ModeTTY {
		task.renderSpinner(0)
		go task.spin()
		return task
	}
	r.write("→ " + label + "\n")
	return task
}

func (t *Task) Update(message string) {
	if t == nil || t.reporter == nil {
		return
	}
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	t.detail = message
	previous := t.updated
	t.updated = time.Now()
	t.mu.Unlock()
	if t.reporter.mode == ModeTTY {
		return
	}
	if !previous.IsZero() && time.Since(previous) < 2*time.Second {
		return
	}
	t.reporter.write("  " + message + "\n")
}

func (t *Task) Complete() { t.finish(true, nil) }

func (t *Task) Fail(err error) { t.finish(false, err) }

func (t *Task) spin() {
	ticker := time.NewTicker(125 * time.Millisecond)
	defer ticker.Stop()
	frame := 1
	for {
		select {
		case <-t.stop:
			return
		case <-ticker.C:
			t.renderSpinner(frame)
			frame++
		}
	}
}

func (t *Task) renderSpinner(frame int) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	detail := t.detail
	t.mu.Unlock()
	if detail != "" {
		detail = " · " + detail
	}
	t.reporter.mu.Lock()
	fmt.Fprintf(t.reporter.writer, "\r\033[2K%s %s%s (%s)", frames[frame%len(frames)], t.label, detail, elapsed(time.Since(t.started)))
	t.reporter.mu.Unlock()
}

func (t *Task) finish(success bool, err error) {
	if t == nil || t.reporter == nil {
		return
	}
	t.done.Do(func() {
		t.mu.Lock()
		t.finished = true
		t.mu.Unlock()
		if t.reporter.mode == ModeTTY {
			close(t.stop)
		}
		icon := "✓"
		if !success {
			icon = "✗"
		}
		line := fmt.Sprintf("%s %s — %s", icon, t.label, elapsed(time.Since(t.started)))
		if err != nil {
			line += ": " + err.Error()
		}
		if t.reporter.mode == ModeTTY {
			t.reporter.mu.Lock()
			fmt.Fprintf(t.reporter.writer, "\r\033[2K%s\n", line)
			t.reporter.mu.Unlock()
			return
		}
		t.reporter.write(line + "\n")
	})
}

func (r *Reporter) write(value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = io.WriteString(r.writer, value)
}

func elapsed(duration time.Duration) string {
	if duration < time.Second {
		return duration.Round(10 * time.Millisecond).String()
	}
	return duration.Round(100 * time.Millisecond).String()
}
