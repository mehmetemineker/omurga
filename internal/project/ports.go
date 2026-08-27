package project

import (
	"context"

	"omurga/internal/gateway"
	"omurga/internal/manifest"
)

type GatewayPortStore interface {
	AllocateGatewayPorts(context.Context, string, string, []gateway.Target) (map[string]int, error)
	PlanGatewayPorts(context.Context, string, string, []gateway.Target) (map[string]int, error)
}

func PersistentPorts(ctx context.Context, store GatewayPortStore, project manifest.Project, environment string, dryRun bool) (map[string]int, error) {
	targets := GatewayTargets(project)
	if dryRun {
		return store.PlanGatewayPorts(ctx, project.Name, environment, targets)
	}
	return store.AllocateGatewayPorts(ctx, project.Name, environment, targets)
}
