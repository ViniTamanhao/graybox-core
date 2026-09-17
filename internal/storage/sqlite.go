// Package storage persists Graybox recordings in documented SQLite files.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/opemori/graybox-core/internal/recording"
	_ "modernc.org/sqlite"
)

var (
	ErrInvalidRecording  = errors.New("file does not contain a valid Graybox schema")
	ErrUnsupportedSchema = errors.New("recording uses an unsupported schema version")
)

// Store owns the SQLite connection for one recording.
type Store struct {
	db     *sql.DB
	mu     sync.Mutex
	closed bool
}

// Create creates a new recording without overwriting an existing file.
func Create(ctx context.Context, path, grayboxVersion string) (*Store, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create recording: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("create recording: %w", err)
	}

	s, err := openDB(path)
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	if err := s.initialize(ctx, grayboxVersion); err != nil {
		_ = s.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return s, nil
}

// Open opens and validates an existing recording.
func Open(ctx context.Context, path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRecording, err)
	}
	s, err := openDB(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRecording, err)
	}
	if err := s.validate(ctx); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func openDB(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	// A single connection keeps PRAGMA state deterministic and serializes the
	// short write transactions made by concurrent proxy handlers.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure sqlite database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) initialize(ctx context.Context, grayboxVersion string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("initialize recording: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schemaV1); err != nil {
		return fmt.Errorf("initialize recording schema: %w", err)
	}
	metadata := map[string]string{
		"format":          "graybox",
		"schema_version":  strconv.Itoa(recording.SchemaVersion),
		"created_at":      nowUTC(),
		"graybox_version": grayboxVersion,
	}
	for key, value := range metadata {
		if _, err := tx.ExecContext(ctx, `INSERT INTO metadata(key, value) VALUES (?, ?)`, key, value); err != nil {
			return fmt.Errorf("write recording metadata: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recording schema: %w", err)
	}
	return nil
}

func (s *Store) validate(ctx context.Context) error {
	var format, version string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'format'`).Scan(&format); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRecording, err)
	}
	if format != "graybox" {
		return ErrInvalidRecording
	}
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'schema_version'`).Scan(&version); err != nil {
		return fmt.Errorf("%w: missing schema version", ErrInvalidRecording)
	}
	if version != strconv.Itoa(recording.SchemaVersion) {
		return fmt.Errorf("%w: got %q, support %d", ErrUnsupportedSchema, version, recording.SchemaVersion)
	}
	var tables int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
WHERE type = 'table' AND name IN ('metadata', 'exchanges', 'request_headers', 'response_headers', 'request_bodies', 'response_bodies')`).Scan(&tables); err != nil {
		return fmt.Errorf("%w: cannot inspect tables: %v", ErrInvalidRecording, err)
	}
	if tables != 6 {
		return fmt.Errorf("%w: recording schema is incomplete", ErrInvalidRecording)
	}
	return nil
}

// Close flushes and closes the recording.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if _, err := s.db.Exec(`PRAGMA optimize`); err != nil {
		_ = s.db.Close()
		return fmt.Errorf("optimize recording: %w", err)
	}
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close recording: %w", err)
	}
	return nil
}

const schemaV1 = `
CREATE TABLE metadata (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
CREATE TABLE exchanges (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    protocol        TEXT NOT NULL,
    started_at      TEXT NOT NULL,
    completed_at    TEXT NOT NULL,
    duration_ns     INTEGER NOT NULL CHECK (duration_ns >= 0),
    request_method  TEXT NOT NULL,
    request_url     TEXT NOT NULL,
    response_status INTEGER NOT NULL
);
CREATE TABLE request_headers (
    exchange_id INTEGER NOT NULL REFERENCES exchanges(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    value       TEXT NOT NULL,
    ordinal     INTEGER NOT NULL,
    PRIMARY KEY (exchange_id, name, ordinal)
);
CREATE TABLE response_headers (
    exchange_id INTEGER NOT NULL REFERENCES exchanges(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    value       TEXT NOT NULL,
    ordinal     INTEGER NOT NULL,
    PRIMARY KEY (exchange_id, name, ordinal)
);
CREATE TABLE request_bodies (
    exchange_id INTEGER PRIMARY KEY REFERENCES exchanges(id) ON DELETE CASCADE,
    content     BLOB NOT NULL
);
CREATE TABLE response_bodies (
    exchange_id INTEGER PRIMARY KEY REFERENCES exchanges(id) ON DELETE CASCADE,
    content     BLOB NOT NULL
);
CREATE INDEX exchanges_started_at_idx ON exchanges(started_at);
CREATE INDEX exchanges_method_idx ON exchanges(request_method);
CREATE INDEX exchanges_status_idx ON exchanges(response_status);
`
