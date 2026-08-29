package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Deployment struct {
	Project      string `json:"project"`
	Environment  string `json:"environment"`
	Status       string `json:"status"`
	Revision     string `json:"revision"`
	ManifestPath string `json:"manifestPath"`
	ComposePath  string `json:"composePath"`
	CaddyPath    string `json:"caddyPath,omitempty"`
	UpdatedAt    string `json:"updatedAt"`
	LastError    string `json:"lastError,omitempty"`
}

type DeleteProjectResult struct {
	DeploymentsDeleted int64 `json:"deploymentsDeleted"`
	PortsReleased      int64 `json:"portsReleased"`
}

func (s *Store) ListDeployments(ctx context.Context) ([]Deployment, error) {
	if s.version < 2 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT project, environment, status, revision, manifest_path, compose_path, caddy_path, updated_at, last_error
FROM deployments
ORDER BY project, environment
`)
	if err != nil {
		return nil, fmt.Errorf("could not list deployment state: %w", err)
	}
	defer rows.Close()
	var deployments []Deployment
	for rows.Next() {
		var deployment Deployment
		if err := rows.Scan(&deployment.Project, &deployment.Environment, &deployment.Status, &deployment.Revision,
			&deployment.ManifestPath, &deployment.ComposePath, &deployment.CaddyPath,
			&deployment.UpdatedAt, &deployment.LastError); err != nil {
			return nil, fmt.Errorf("could not read deployment state row: %w", err)
		}
		deployments = append(deployments, deployment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not finish listing deployment state: %w", err)
	}
	return deployments, nil
}

func (s *Store) PutDeployment(ctx context.Context, deployment Deployment) error {
	if s.readOnly {
		return fmt.Errorf("cannot update a deployment using a read-only state database")
	}
	if deployment.Project == "" || deployment.Environment == "" {
		return fmt.Errorf("deployment project and environment are required")
	}
	if deployment.Status == "" || deployment.Revision == "" || deployment.ManifestPath == "" || deployment.ComposePath == "" {
		return fmt.Errorf("deployment status, revision, manifest path, and Compose path are required")
	}
	deployment.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO deployments (
    project, environment, status, revision, manifest_path, compose_path, caddy_path, updated_at, last_error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project, environment) DO UPDATE SET
    status = excluded.status,
    revision = excluded.revision,
    manifest_path = excluded.manifest_path,
    compose_path = excluded.compose_path,
    caddy_path = excluded.caddy_path,
    updated_at = excluded.updated_at,
    last_error = excluded.last_error
`, deployment.Project, deployment.Environment, deployment.Status, deployment.Revision,
		deployment.ManifestPath, deployment.ComposePath, deployment.CaddyPath, deployment.UpdatedAt, deployment.LastError)
	if err != nil {
		return fmt.Errorf("could not store deployment state: %w", err)
	}
	return nil
}

func (s *Store) GetDeployment(ctx context.Context, project, environment string) (Deployment, bool, error) {
	if s.version < 2 {
		return Deployment{}, false, nil
	}
	var deployment Deployment
	err := s.db.QueryRowContext(ctx, `
SELECT project, environment, status, revision, manifest_path, compose_path, caddy_path, updated_at, last_error
FROM deployments
WHERE project = ? AND environment = ?
`, project, environment).Scan(
		&deployment.Project, &deployment.Environment, &deployment.Status, &deployment.Revision,
		&deployment.ManifestPath, &deployment.ComposePath, &deployment.CaddyPath,
		&deployment.UpdatedAt, &deployment.LastError,
	)
	if err == sql.ErrNoRows {
		return Deployment{}, false, nil
	}
	if err != nil {
		return Deployment{}, false, fmt.Errorf("could not read deployment state: %w", err)
	}
	return deployment, true, nil
}

func (s *Store) SetDeploymentStatus(ctx context.Context, project, environment, status, lastError string) error {
	if s.readOnly {
		return fmt.Errorf("cannot update a deployment using a read-only state database")
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE deployments SET status = ?, last_error = ?, updated_at = ?
WHERE project = ? AND environment = ?
`, status, lastError, time.Now().UTC().Format(time.RFC3339Nano), project, environment)
	if err != nil {
		return fmt.Errorf("could not update deployment status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("could not inspect deployment status update: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("deployment %s/%s was not found", project, environment)
	}
	return nil
}

func (s *Store) DeleteProject(ctx context.Context, project, environment string) (DeleteProjectResult, error) {
	if s.readOnly {
		return DeleteProjectResult{}, fmt.Errorf("cannot delete project state using a read-only state database")
	}
	if project == "" || environment == "" {
		return DeleteProjectResult{}, fmt.Errorf("project and environment are required for state deletion")
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return DeleteProjectResult{}, fmt.Errorf("could not connect to state database: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return DeleteProjectResult{}, fmt.Errorf("could not configure state database busy timeout: %w", err)
	}
	if err := beginImmediate(ctx, conn); err != nil {
		return DeleteProjectResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	deploymentResult, err := conn.ExecContext(ctx, "DELETE FROM deployments WHERE project = ? AND environment = ?", project, environment)
	if err != nil {
		return DeleteProjectResult{}, fmt.Errorf("could not delete deployment state: %w", err)
	}
	portResult, err := conn.ExecContext(ctx, "DELETE FROM gateway_ports WHERE project = ? AND environment = ?", project, environment)
	if err != nil {
		return DeleteProjectResult{}, fmt.Errorf("could not release gateway ports: %w", err)
	}
	deploymentsDeleted, err := deploymentResult.RowsAffected()
	if err != nil {
		return DeleteProjectResult{}, fmt.Errorf("could not inspect deleted deployment state: %w", err)
	}
	portsReleased, err := portResult.RowsAffected()
	if err != nil {
		return DeleteProjectResult{}, fmt.Errorf("could not inspect released gateway ports: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return DeleteProjectResult{}, fmt.Errorf("could not commit project state deletion: %w", err)
	}
	committed = true
	return DeleteProjectResult{DeploymentsDeleted: deploymentsDeleted, PortsReleased: portsReleased}, nil
}
