package project

import (
	"context"
	"testing"

	"omurga/internal/gateway"
	"omurga/internal/manifest"
)

type fakePortStore struct {
	allocated bool
	planned   bool
	targets   []gateway.Target
}

func (s *fakePortStore) AllocateGatewayPorts(_ context.Context, _, _ string, targets []gateway.Target) (map[string]int, error) {
	s.allocated = true
	s.targets = targets
	return map[string]int{"app:3000": 21000}, nil
}

func (s *fakePortStore) PlanGatewayPorts(_ context.Context, _, _ string, targets []gateway.Target) (map[string]int, error) {
	s.planned = true
	s.targets = targets
	return map[string]int{"app:3000": 21000}, nil
}

func TestPersistentPortsDoesNotAllocateDuringDryRun(t *testing.T) {
	store := &fakePortStore{}
	project := manifest.Project{
		Name:    "demo",
		Gateway: manifest.Gateway{Routes: []manifest.Route{{Service: "app", Port: 3000}}},
	}

	ports, err := PersistentPorts(context.Background(), store, project, "production", true)
	if err != nil {
		t.Fatalf("PersistentPorts() error = %v", err)
	}
	if !store.planned || store.allocated || len(store.targets) != 1 || ports["app:3000"] != 21000 {
		t.Fatalf("unexpected dry-run behavior: store=%#v ports=%#v", store, ports)
	}
}
