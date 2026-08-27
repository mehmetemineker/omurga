package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"omurga/internal/gateway"
)

func TestOpenCreatesSecureVersionedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	if store.Path() != path {
		t.Fatalf("Path() = %q, want %q", store.Path(), path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("could not stat database: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("database mode = %o, want 600", info.Mode().Perm())
	}
	var version int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("could not read schema version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("schema version = %d, want %d", version, schemaVersion)
	}
}

func TestOpenReadOnlyDoesNotCreateOrModifyState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	missingPath := filepath.Join(root, "missing", "state.db")
	if _, err := OpenReadOnly(ctx, missingPath); err == nil {
		t.Fatal("OpenReadOnly() succeeded for a missing database")
	}
	if _, err := os.Stat(filepath.Dir(missingPath)); !os.IsNotExist(err) {
		t.Fatalf("OpenReadOnly() created a directory: %v", err)
	}

	path := filepath.Join(root, "state.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	readOnly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	defer readOnly.Close()
	if !readOnly.ReadOnly() {
		t.Fatal("OpenReadOnly() did not mark the store as read-only")
	}
	if _, err := readOnly.AllocateGatewayPorts(ctx, "demo", "production", []gateway.Target{{Service: "app", ContainerPort: 3000}}); err == nil {
		t.Fatal("read-only store allowed a gateway port allocation")
	}
	ports, err := readOnly.PlanGatewayPorts(ctx, "demo", "production", []gateway.Target{{Service: "app", ContainerPort: 3000}})
	if err != nil {
		t.Fatalf("PlanGatewayPorts() error = %v", err)
	}
	if len(ports) != 1 {
		t.Fatalf("unexpected plan: %#v", ports)
	}
}

func TestAllocateGatewayPortsIsStableAndGloballyUnique(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	target := gateway.Target{Service: "app", ContainerPort: 3000}
	first, err := store.AllocateGatewayPorts(ctx, "alpha", "production", []gateway.Target{target})
	if err != nil {
		t.Fatalf("AllocateGatewayPorts() error = %v", err)
	}
	second, err := store.AllocateGatewayPorts(ctx, "alpha", "production", []gateway.Target{target})
	if err != nil {
		t.Fatalf("second AllocateGatewayPorts() error = %v", err)
	}
	if first["app:3000"] != second["app:3000"] {
		t.Fatalf("allocation changed: first=%#v second=%#v", first, second)
	}

	other, err := store.AllocateGatewayPorts(ctx, "beta", "production", []gateway.Target{target})
	if err != nil {
		t.Fatalf("other AllocateGatewayPorts() error = %v", err)
	}
	if other["app:3000"] == first["app:3000"] {
		t.Fatalf("projects received the same host port: %#v %#v", first, other)
	}
}

func TestPlanGatewayPortsAvoidsCollisionsWithoutWriting(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	target := gateway.Target{Service: "app", ContainerPort: 3000}
	candidate := gateway.Candidate("demo", "production", target)
	if _, err := store.db.Exec(`
INSERT INTO gateway_ports (project, environment, service, container_port, host_port, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, "existing", "production", "web", 8080, candidate, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("could not seed collision: %v", err)
	}

	planned, err := store.PlanGatewayPorts(ctx, "demo", "production", []gateway.Target{target})
	if err != nil {
		t.Fatalf("PlanGatewayPorts() error = %v", err)
	}
	if planned["app:3000"] == candidate {
		t.Fatalf("planned port did not avoid collision: %#v", planned)
	}
	ports, err := store.ListGatewayPorts(ctx, "demo", "production")
	if err != nil {
		t.Fatalf("ListGatewayPorts() error = %v", err)
	}
	if len(ports) != 0 {
		t.Fatalf("planning persisted an allocation: %#v", ports)
	}

	allocated, err := store.AllocateGatewayPorts(ctx, "demo", "production", []gateway.Target{target})
	if err != nil {
		t.Fatalf("AllocateGatewayPorts() error = %v", err)
	}
	if allocated["app:3000"] != planned["app:3000"] {
		t.Fatalf("plan and allocation differ without an intervening write: plan=%#v allocation=%#v", planned, allocated)
	}
}

func TestListGatewayPortsFiltersAndSorts(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	_, err = store.AllocateGatewayPorts(ctx, "demo", "production", []gateway.Target{
		{Service: "web", ContainerPort: 8080},
		{Service: "api", ContainerPort: 3000},
	})
	if err != nil {
		t.Fatalf("AllocateGatewayPorts() error = %v", err)
	}
	ports, err := store.ListGatewayPorts(ctx, "demo", "production")
	if err != nil {
		t.Fatalf("ListGatewayPorts() error = %v", err)
	}
	if len(ports) != 2 || ports[0].Service != "api" || ports[1].Service != "web" {
		t.Fatalf("unexpected ports: %#v", ports)
	}
}

func TestConcurrentStoresSerializeCollidingAllocations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	firstStore, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	defer firstStore.Close()
	secondStore, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer secondStore.Close()

	target := gateway.Target{Service: "app", ContainerPort: 3000}
	firstProject := "first"
	candidate := gateway.Candidate(firstProject, "production", target)
	secondProject := ""
	for index := 0; index < 100000; index++ {
		name := fmt.Sprintf("second-%d", index)
		if gateway.Candidate(name, "production", target) == candidate {
			secondProject = name
			break
		}
	}
	if secondProject == "" {
		t.Fatal("could not find a deterministic candidate collision")
	}

	start := make(chan struct{})
	type result struct {
		ports map[string]int
		err   error
	}
	results := make(chan result, 2)
	allocate := func(store *Store, project string) {
		<-start
		ports, err := store.AllocateGatewayPorts(ctx, project, "production", []gateway.Target{target})
		results <- result{ports: ports, err: err}
	}
	go allocate(firstStore, firstProject)
	go allocate(secondStore, secondProject)
	close(start)

	firstResult := <-results
	secondResult := <-results
	if firstResult.err != nil || secondResult.err != nil {
		t.Fatalf("concurrent allocation errors: %v, %v", firstResult.err, secondResult.err)
	}
	if firstResult.ports["app:3000"] == secondResult.ports["app:3000"] {
		t.Fatalf("concurrent allocations collided: %#v %#v", firstResult.ports, secondResult.ports)
	}
}

func TestDeploymentStateLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	deployment := Deployment{
		Project: "demo", Environment: "production", Status: "running", Revision: "abc123",
		ManifestPath: "/projects/demo/omurga.yaml", ComposePath: "/var/lib/omurga/projects/demo/production/compose.yaml",
	}
	if err := store.PutDeployment(ctx, deployment); err != nil {
		t.Fatalf("PutDeployment() error = %v", err)
	}
	stored, exists, err := store.GetDeployment(ctx, "demo", "production")
	if err != nil || !exists {
		t.Fatalf("GetDeployment() = %#v, %v, %v", stored, exists, err)
	}
	if stored.Status != "running" || stored.UpdatedAt == "" {
		t.Fatalf("unexpected deployment: %#v", stored)
	}
	if err := store.SetDeploymentStatus(ctx, "demo", "production", "stopped", ""); err != nil {
		t.Fatalf("SetDeploymentStatus() error = %v", err)
	}
	stored, _, err = store.GetDeployment(ctx, "demo", "production")
	if err != nil || stored.Status != "stopped" {
		t.Fatalf("unexpected updated deployment: %#v, %v", stored, err)
	}
}

func TestVersionOneDatabaseIsReadableAndMigratesOnWriteOpen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := raw.Exec("DROP TABLE deployments; PRAGMA user_version = 1;"); err != nil {
		raw.Close()
		t.Fatalf("could not downgrade test database: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw Close() error = %v", err)
	}

	readOnly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	if readOnly.version != 1 {
		t.Fatalf("read-only schema version = %d, want 1", readOnly.version)
	}
	if _, exists, err := readOnly.GetDeployment(ctx, "demo", "production"); err != nil || exists {
		t.Fatalf("GetDeployment() on v1 = %v, %v", exists, err)
	}
	if err := readOnly.Close(); err != nil {
		t.Fatalf("read-only Close() error = %v", err)
	}

	migrated, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("migrating Open() error = %v", err)
	}
	defer migrated.Close()
	if migrated.version != schemaVersion {
		t.Fatalf("migrated schema version = %d, want %d", migrated.version, schemaVersion)
	}
	if err := migrated.PutDeployment(ctx, Deployment{
		Project: "demo", Environment: "production", Status: "running", Revision: "abc",
		ManifestPath: "/project/omurga.yaml", ComposePath: "/project/compose.yaml",
	}); err != nil {
		t.Fatalf("PutDeployment() after migration error = %v", err)
	}
}
