package gateway

import "testing"

func TestUniqueTargetsValidatesDeduplicatesAndSorts(t *testing.T) {
	targets, err := UniqueTargets([]Target{
		{Service: "web", ContainerPort: 8080},
		{Service: "api", ContainerPort: 3000},
		{Service: "web", ContainerPort: 8080},
	})
	if err != nil {
		t.Fatalf("UniqueTargets() error = %v", err)
	}
	if len(targets) != 2 || targets[0].Service != "api" || targets[1].Service != "web" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}

func TestNextAvailableWrapsWithinManagedRange(t *testing.T) {
	port, err := NextAvailable(PortEnd, map[int]bool{PortEnd: true})
	if err != nil {
		t.Fatalf("NextAvailable() error = %v", err)
	}
	if port != PortStart {
		t.Fatalf("NextAvailable() = %d, want %d", port, PortStart)
	}
}
