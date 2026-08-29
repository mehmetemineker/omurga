package shared

import (
	"strings"
	"testing"

	"omurga/internal/host"
)

func TestGenerateRedisServiceUsesSharedNetworkAndBindData(t *testing.T) {
	content, data, err := Generate("redis", "cache", "", "", host.DefaultPaths(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "redis:7.4") || !strings.Contains(text, "omurga-shared") || !strings.Contains(text, data) {
		t.Fatalf("unexpected Compose document: %s", text)
	}
}
