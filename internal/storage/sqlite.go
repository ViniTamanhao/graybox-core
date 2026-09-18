// Package storage persists Graybox recordings in documented SQLite files.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
	_ "modernc.org/sqlite"
)

var (
	ErrInvalidRecording  = errors.New("file does not contain a valid Graybox schema")
	ErrUnsupportedSchema = errors.New("recording uses an unsupported schema version")
)

// Store owns the SQLite connection for one recording.
type Store struct {
	db       *sql.DB
	readOnly bool
	mu       sync.Mutex
	closed   bool
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

	s, err := openDB(path, false)
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
	return open(ctx, path, false)
}

// OpenReadOnly opens and validates a recording without write access.
func OpenReadOnly(ctx context.Context, path string) (*Store, error) {
	return open(ctx, path, true)
}

func open(ctx context.Context, path string, readOnly bool) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRecording, err)
	}
	s, err := openDB(path, readOnly)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRecording, err)
	}
	if err := s.validate(ctx); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func openDB(path string, readOnly bool) (*Store, error) {
	dsn := path
	if readOnly {
		var err error
		dsn, err = sqliteReadOnlyDSN(path)
		if err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", dsn)
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
	return &Store{db: db, readOnly: readOnly}, nil
}

func sqliteReadOnlyDSN(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve recording path: %w", err)
	}
	return sqliteReadOnlyDSNFromAbsolute(absolute, runtime.GOOS == "windows"), nil
}

func sqliteReadOnlyDSNFromAbsolute(absolute string, windows bool) string {
	if windows {
		absolute = strings.ReplaceAll(absolute, `\`, "/")
		if len(absolute) >= 3 && isASCIIAlpha(absolute[0]) && absolute[1] == ':' && absolute[2] == '/' {
			absolute = "/" + absolute
		}
	}
	location := &url.URL{Scheme: "file", Path: absolute}
	query := location.Query()
	query.Set("mode", "ro")
	location.RawQuery = query.Encode()
	return location.String()
}

func isASCIIAlpha(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
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
	var createdAt, grayboxVersion string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'created_at'`).Scan(&createdAt); err != nil {
		return fmt.Errorf("%w: missing creation time", ErrInvalidRecording)
	}
	if _, err := time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return fmt.Errorf("%w: invalid creation time: %v", ErrInvalidRecording, err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'graybox_version'`).Scan(&grayboxVersion); err != nil || grayboxVersion == "" {
		return fmt.Errorf("%w: missing Graybox version", ErrInvalidRecording)
	}
	for _, table := range schemaV1Tables {
		if err := s.validateTable(ctx, table); err != nil {
			return err
		}
	}
	for _, table := range []string{"request_bodies", "response_bodies"} {
		if err := s.validateBodyConstraints(ctx, table); err != nil {
			return err
		}
	}
	for _, index := range []indexSpec{
		{"exchanges_started_at_idx", "started_at"},
		{"exchanges_method_idx", "request_method"},
		{"exchanges_status_idx", "response_status"},
	} {
		if err := s.validateIndex(ctx, index); err != nil {
			return err
		}
	}
	return nil
}

type tableSpec struct {
	name    string
	columns []columnSpec
}

type columnSpec struct {
	name       string
	typeName   string
	notNull    bool
	primaryKey int
}

type indexSpec struct {
	name   string
	column string
}

var schemaV1Tables = []tableSpec{
	{"metadata", []columnSpec{{"key", "TEXT", false, 1}, {"value", "TEXT", true, 0}}},
	{"exchanges", []columnSpec{
		{"id", "INTEGER", false, 1}, {"protocol", "TEXT", true, 0}, {"started_at", "TEXT", true, 0},
		{"completed_at", "TEXT", true, 0}, {"duration_ns", "INTEGER", true, 0}, {"request_method", "TEXT", true, 0},
		{"request_url", "TEXT", true, 0}, {"response_status", "INTEGER", true, 0}, {"proxy_error", "TEXT", true, 0},
	}},
	{"request_headers", headerColumnSpecs()},
	{"response_headers", headerColumnSpecs()},
	{"request_bodies", bodyColumnSpecs()},
	{"response_bodies", bodyColumnSpecs()},
}

func headerColumnSpecs() []columnSpec {
	return []columnSpec{{"exchange_id", "INTEGER", true, 1}, {"name", "TEXT", true, 2}, {"value", "TEXT", true, 0}, {"ordinal", "INTEGER", true, 3}}
}

func bodyColumnSpecs() []columnSpec {
	return []columnSpec{
		{"exchange_id", "INTEGER", false, 1},
		{"content", "BLOB", true, 0},
		{"observed_size", "INTEGER", true, 0},
		{"captured_size", "INTEGER", true, 0},
		{"truncated", "INTEGER", true, 0},
		{"complete", "INTEGER", true, 0},
	}
}

func (s *Store) validateTable(ctx context.Context, table tableSpec) error {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s)`, table.name))
	if err != nil {
		return fmt.Errorf("%w: cannot inspect table %s: %v", ErrInvalidRecording, table.name, err)
	}
	defer rows.Close()
	actual := make([]columnSpec, 0, len(table.columns))
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, typeName string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("%w: inspect table %s: %v", ErrInvalidRecording, table.name, err)
		}
		if cid != len(actual) || defaultValue.Valid {
			return fmt.Errorf("%w: table %s does not match schema version 1", ErrInvalidRecording, table.name)
		}
		actual = append(actual, columnSpec{name: name, typeName: strings.ToUpper(typeName), notNull: notNull != 0, primaryKey: primaryKey})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: inspect table %s: %v", ErrInvalidRecording, table.name, err)
	}
	if !reflect.DeepEqual(actual, table.columns) {
		return fmt.Errorf("%w: table %s is missing or does not match schema version 1", ErrInvalidRecording, table.name)
	}
	return nil
}

func (s *Store) validateBodyConstraints(ctx context.Context, table string) error {
	var definition string

	if err := s.db.QueryRowContext(
		ctx,
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`,
		table,
	).Scan(&definition); err != nil {
		return fmt.Errorf(
			"%w: cannot inspect table %s",
			ErrInvalidRecording,
			table,
		)
	}

	compact := strings.NewReplacer(
		" ", "",
		"\n", "",
		"\r", "",
		"\t", "",
	).Replace(strings.ToUpper(definition))

	for _, constraint := range []string{
		"CHECK(OBSERVED_SIZE>=0)",
		"CHECK(CAPTURED_SIZE>=0)",
		"CHECK(TRUNCATEDIN(0,1))",
		"CHECK(COMPLETEIN(0,1))",
		"CHECK(CAPTURED_SIZE=LENGTH(CONTENT))",
		"CHECK(CAPTURED_SIZE<=OBSERVED_SIZE)",
		"CHECK((TRUNCATED=0ANDCAPTURED_SIZE=OBSERVED_SIZE)OR(TRUNCATED=1ANDCAPTURED_SIZE<OBSERVED_SIZE))",
	} {
		if !strings.Contains(compact, constraint) {
			return fmt.Errorf(
				"%w: table %s has incompatible body metadata constraints",
				ErrInvalidRecording,
				table,
			)
		}
	}

	return nil
}

func (s *Store) validateIndex(ctx context.Context, index indexSpec) error {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_info(%s)`, index.name))
	if err != nil {
		return fmt.Errorf("%w: cannot inspect index %s: %v", ErrInvalidRecording, index.name, err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var sequence, columnID int
		var name string
		if err := rows.Scan(&sequence, &columnID, &name); err != nil {
			return fmt.Errorf("%w: inspect index %s: %v", ErrInvalidRecording, index.name, err)
		}
		if sequence != len(columns) {
			return fmt.Errorf("%w: index %s does not match schema version 1", ErrInvalidRecording, index.name)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: inspect index %s: %v", ErrInvalidRecording, index.name, err)
	}
	if len(columns) != 1 || columns[0] != index.column {
		return fmt.Errorf("%w: index %s is missing or does not match schema version 1", ErrInvalidRecording, index.name)
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
	if !s.readOnly {
		if _, err := s.db.Exec(`PRAGMA optimize`); err != nil {
			_ = s.db.Close()
			return fmt.Errorf("optimize recording: %w", err)
		}
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
    response_status INTEGER NOT NULL,
    proxy_error     TEXT NOT NULL
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
    exchange_id  INTEGER PRIMARY KEY REFERENCES exchanges(id) ON DELETE CASCADE,
    content       BLOB NOT NULL,
    observed_size INTEGER NOT NULL CHECK (observed_size >= 0),
    captured_size INTEGER NOT NULL CHECK (captured_size >= 0),
    truncated     INTEGER NOT NULL CHECK (truncated IN (0, 1)),
    complete      INTEGER NOT NULL CHECK (complete IN (0, 1)),
    CHECK (captured_size = length(content)),
    CHECK (captured_size <= observed_size),
    CHECK ((truncated = 0 AND captured_size = observed_size) OR
           (truncated = 1 AND captured_size < observed_size))
);
CREATE TABLE response_bodies (
    exchange_id  INTEGER PRIMARY KEY REFERENCES exchanges(id) ON DELETE CASCADE,
    content       BLOB NOT NULL,
    observed_size INTEGER NOT NULL CHECK (observed_size >= 0),
    captured_size INTEGER NOT NULL CHECK (captured_size >= 0),
    truncated     INTEGER NOT NULL CHECK (truncated IN (0, 1)),
    complete      INTEGER NOT NULL CHECK (complete IN (0, 1)),
    CHECK (captured_size = length(content)),
    CHECK (captured_size <= observed_size),
    CHECK ((truncated = 0 AND captured_size = observed_size) OR
           (truncated = 1 AND captured_size < observed_size))
);
CREATE INDEX exchanges_started_at_idx ON exchanges(started_at);
CREATE INDEX exchanges_method_idx ON exchanges(request_method);
CREATE INDEX exchanges_status_idx ON exchanges(response_status);
`
