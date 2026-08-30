package webhook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const replayRetention = 24 * time.Hour

type replayGuard struct {
	mu      sync.Mutex
	path    string
	entries map[string]time.Time
}

func newReplayGuard(path string) (*replayGuard, error) {
	guard := &replayGuard{path: path, entries: make(map[string]time.Time)}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return guard, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not read webhook replay state: %w", err)
	}
	var stored map[string]time.Time
	if err := json.Unmarshal(content, &stored); err != nil {
		return nil, fmt.Errorf("could not parse webhook replay state: %w", err)
	}
	for delivery, expiry := range stored {
		if expiry.After(time.Now()) {
			guard.entries[delivery] = expiry
		}
	}
	return guard, nil
}

func (g *replayGuard) reserve(delivery string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cleanup(now)
	if _, exists := g.entries[delivery]; exists {
		return false
	}
	g.entries[delivery] = now.Add(replayRetention)
	return true
}

func (g *replayGuard) release(delivery string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.entries, delivery)
}

func (g *replayGuard) commit(delivery string, now time.Time) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cleanup(now)
	if _, exists := g.entries[delivery]; !exists {
		g.entries[delivery] = now.Add(replayRetention)
	}
	content, err := json.MarshalIndent(g.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode webhook replay state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(g.path), 0o700); err != nil {
		return fmt.Errorf("could not create webhook replay directory: %w", err)
	}
	if err := os.WriteFile(g.path, append(content, '\n'), 0o600); err != nil {
		return fmt.Errorf("could not write webhook replay state: %w", err)
	}
	_ = os.Chmod(g.path, 0o600)
	return nil
}

func (g *replayGuard) cleanup(now time.Time) {
	for delivery, expiry := range g.entries {
		if !expiry.After(now) {
			delete(g.entries, delivery)
		}
	}
}
