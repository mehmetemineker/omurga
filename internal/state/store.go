package state

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schemaVersion = 1

type Store struct {
	db       *sql.DB
	path     string
	readOnly bool
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("state database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("could not create state directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("could not open state database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{db: db, path: path}
	if err := store.initialize(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		db.Close()
		return nil, fmt.Errorf("could not secure state database: %w", err)
	}
	return store, nil
}

func OpenReadOnly(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("state database path is required")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("could not access state database: %w", err)
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("could not resolve state database path: %w", err)
	}
	databaseURL := &url.URL{Scheme: "file", Path: filepath.ToSlash(absolutePath)}
	query := databaseURL.Query()
	query.Set("mode", "ro")
	databaseURL.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", databaseURL.String())
	if err != nil {
		return nil, fmt.Errorf("could not open state database read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, path: path, readOnly: true}
	if err := store.validateSchema(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) ReadOnly() bool {
	return s != nil && s.readOnly
}

func (s *Store) validateSchema(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("could not read state database schema version: %w", err)
	}
	if version != schemaVersion {
		return fmt.Errorf("state database schema version %d is not supported; expected version %d", version, schemaVersion)
	}
	return nil
}

func (s *Store) initialize(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("could not connect to state database: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return fmt.Errorf("could not configure state database busy timeout: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("could not enable state database foreign keys: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("could not enable state database WAL mode: %w", err)
	}

	var version int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("could not read state database schema version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("state database schema version %d is newer than supported version %d", version, schemaVersion)
	}
	if version == schemaVersion {
		return nil
	}

	if err := beginImmediate(ctx, conn); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("could not recheck state database schema version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("state database schema version %d is newer than supported version %d", version, schemaVersion)
	}
	if version == schemaVersion {
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return fmt.Errorf("could not commit state database schema check: %w", err)
		}
		committed = true
		return nil
	}

	if _, err := conn.ExecContext(ctx, `
CREATE TABLE gateway_ports (
    project TEXT NOT NULL,
    environment TEXT NOT NULL,
    service TEXT NOT NULL,
    container_port INTEGER NOT NULL CHECK (container_port BETWEEN 1 AND 65535),
    host_port INTEGER NOT NULL UNIQUE CHECK (host_port BETWEEN 20000 AND 29999),
    created_at TEXT NOT NULL,
    PRIMARY KEY (project, environment, service, container_port)
);
CREATE INDEX gateway_ports_project_idx ON gateway_ports (project, environment);
PRAGMA user_version = 1;
`); err != nil {
		return fmt.Errorf("could not create state database schema: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("could not commit state database schema: %w", err)
	}
	committed = true
	return nil
}

func beginImmediate(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("could not begin state database transaction: %w", err)
	}
	return nil
}
