package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"omurga/internal/gateway"
)

type GatewayPort struct {
	Project       string `json:"project"`
	Environment   string `json:"environment"`
	Service       string `json:"service"`
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
	CreatedAt     string `json:"createdAt"`
}

func (s *Store) AllocateGatewayPorts(ctx context.Context, project, environment string, requested []gateway.Target) (map[string]int, error) {
	if s.readOnly {
		return nil, fmt.Errorf("cannot allocate gateway ports using a read-only state database")
	}
	if project == "" {
		return nil, fmt.Errorf("project name is required for gateway port allocation")
	}
	targets, err := gateway.UniqueTargets(requested)
	if err != nil {
		return nil, err
	}
	environment = gateway.EnvironmentKey(environment)

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not connect to state database: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return nil, fmt.Errorf("could not configure state database busy timeout: %w", err)
	}
	if err := beginImmediate(ctx, conn); err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	assignments, err := existingAssignments(ctx, conn, project, environment, targets)
	if err != nil {
		return nil, err
	}
	used, err := usedPorts(ctx, conn)
	if err != nil {
		return nil, err
	}

	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	for _, target := range targets {
		key := gateway.Key(target.Service, target.ContainerPort)
		if _, exists := assignments[key]; exists {
			continue
		}
		port, err := gateway.NextAvailable(gateway.Candidate(project, environment, target), used)
		if err != nil {
			return nil, err
		}
		if _, err := conn.ExecContext(ctx, `
INSERT INTO gateway_ports (project, environment, service, container_port, host_port, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, project, environment, target.Service, target.ContainerPort, port, createdAt); err != nil {
			return nil, fmt.Errorf("could not store gateway port for %s:%d: %w", target.Service, target.ContainerPort, err)
		}
		assignments[key] = port
		used[port] = true
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return nil, fmt.Errorf("could not commit gateway port allocation: %w", err)
	}
	committed = true
	return assignments, nil
}

func (s *Store) PlanGatewayPorts(ctx context.Context, project, environment string, requested []gateway.Target) (map[string]int, error) {
	if project == "" {
		return nil, fmt.Errorf("project name is required for gateway port planning")
	}
	targets, err := gateway.UniqueTargets(requested)
	if err != nil {
		return nil, err
	}
	environment = gateway.EnvironmentKey(environment)

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not connect to state database: %w", err)
	}
	defer conn.Close()

	assignments, err := existingAssignments(ctx, conn, project, environment, targets)
	if err != nil {
		return nil, err
	}
	used, err := usedPorts(ctx, conn)
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		key := gateway.Key(target.Service, target.ContainerPort)
		if _, exists := assignments[key]; exists {
			continue
		}
		port, err := gateway.NextAvailable(gateway.Candidate(project, environment, target), used)
		if err != nil {
			return nil, err
		}
		assignments[key] = port
		used[port] = true
	}
	return assignments, nil
}

func (s *Store) ListGatewayPorts(ctx context.Context, project, environment string) ([]GatewayPort, error) {
	query := `
SELECT project, environment, service, container_port, host_port, created_at
FROM gateway_ports
WHERE (? = '' OR project = ?) AND (? = '' OR environment = ?)
ORDER BY project, environment, service, container_port
`
	rows, err := s.db.QueryContext(ctx, query, project, project, environment, environment)
	if err != nil {
		return nil, fmt.Errorf("could not list gateway ports: %w", err)
	}
	defer rows.Close()

	var ports []GatewayPort
	for rows.Next() {
		var port GatewayPort
		if err := rows.Scan(&port.Project, &port.Environment, &port.Service, &port.ContainerPort, &port.HostPort, &port.CreatedAt); err != nil {
			return nil, fmt.Errorf("could not read gateway port: %w", err)
		}
		ports = append(ports, port)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not list gateway ports: %w", err)
	}
	return ports, nil
}

func existingAssignments(ctx context.Context, conn *sql.Conn, project, environment string, targets []gateway.Target) (map[string]int, error) {
	assignments := make(map[string]int, len(targets))
	for _, target := range targets {
		var hostPort int
		err := conn.QueryRowContext(ctx, `
SELECT host_port FROM gateway_ports
WHERE project = ? AND environment = ? AND service = ? AND container_port = ?
`, project, environment, target.Service, target.ContainerPort).Scan(&hostPort)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("could not read gateway port for %s:%d: %w", target.Service, target.ContainerPort, err)
		}
		assignments[gateway.Key(target.Service, target.ContainerPort)] = hostPort
	}
	return assignments, nil
}

func usedPorts(ctx context.Context, conn *sql.Conn) (map[int]bool, error) {
	rows, err := conn.QueryContext(ctx, "SELECT host_port FROM gateway_ports")
	if err != nil {
		return nil, fmt.Errorf("could not read allocated gateway ports: %w", err)
	}
	defer rows.Close()

	used := make(map[int]bool)
	for rows.Next() {
		var port int
		if err := rows.Scan(&port); err != nil {
			return nil, fmt.Errorf("could not read allocated gateway port: %w", err)
		}
		used[port] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read allocated gateway ports: %w", err)
	}
	return used, nil
}
