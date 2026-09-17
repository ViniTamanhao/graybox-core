package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/opemori/graybox-core/internal/recording"
)

var ErrNotFound = errors.New("exchange not found")

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// SetMetadata writes a recording-level metadata value.
func (s *Store) SetMetadata(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO metadata(key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("write recording metadata %q: %w", key, err)
	}
	return nil
}

// Metadata reads a recording-level metadata value.
func (s *Store) Metadata(ctx context.Context, key string) (string, error) {
	var value string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = ?`, key).Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("metadata %q: %w", key, ErrNotFound)
		}
		return "", fmt.Errorf("read recording metadata %q: %w", key, err)
	}
	return value, nil
}

// Add atomically persists an exchange and returns its stable numeric ID.
func (s *Store) Add(ctx context.Context, exchange recording.Exchange) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin recording exchange: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
INSERT INTO exchanges(protocol, started_at, completed_at, duration_ns, request_method, request_url, response_status, proxy_error)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, exchange.Protocol,
		exchange.StartedAt.UTC().Format(time.RFC3339Nano), exchange.EndedAt.UTC().Format(time.RFC3339Nano),
		exchange.Duration.Nanoseconds(), exchange.Request.Method, exchange.Request.URL, exchange.Response.StatusCode, exchange.ProxyError)
	if err != nil {
		return 0, fmt.Errorf("insert exchange: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read exchange id: %w", err)
	}
	if err := insertHeaders(ctx, tx, "request_headers", id, exchange.Request.Headers); err != nil {
		return 0, err
	}
	if err := insertHeaders(ctx, tx, "response_headers", id, exchange.Response.Headers); err != nil {
		return 0, err
	}
	requestSize, requestTruncated, err := bodyMetadata(exchange.Request.Body, exchange.Request.BodySize, exchange.Request.BodyTruncated)
	if err != nil {
		return 0, fmt.Errorf("request body metadata: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO request_bodies(exchange_id, content, original_size, captured_size, truncated) VALUES (?, ?, ?, ?, ?)`,
		id, nonNilBytes(exchange.Request.Body), requestSize, len(exchange.Request.Body), requestTruncated); err != nil {
		return 0, fmt.Errorf("insert request body: %w", err)
	}
	responseSize, responseTruncated, err := bodyMetadata(exchange.Response.Body, exchange.Response.BodySize, exchange.Response.BodyTruncated)
	if err != nil {
		return 0, fmt.Errorf("response body metadata: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO response_bodies(exchange_id, content, original_size, captured_size, truncated) VALUES (?, ?, ?, ?, ?)`,
		id, nonNilBytes(exchange.Response.Body), responseSize, len(exchange.Response.Body), responseTruncated); err != nil {
		return 0, fmt.Errorf("insert response body: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit exchange: %w", err)
	}
	return id, nil
}

func bodyMetadata(body []byte, originalSize int64, truncated bool) (int64, bool, error) {
	capturedSize := int64(len(body))
	if originalSize == 0 && capturedSize > 0 {
		originalSize = capturedSize
	}
	if originalSize < capturedSize {
		return 0, false, fmt.Errorf("original size %d is smaller than captured size %d", originalSize, capturedSize)
	}
	return originalSize, truncated || originalSize > capturedSize, nil
}

func nonNilBytes(body []byte) []byte {
	if body == nil {
		return []byte{}
	}
	return body
}

func insertHeaders(ctx context.Context, tx *sql.Tx, table string, id int64, headers http.Header) error {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	query := fmt.Sprintf(`INSERT INTO %s(exchange_id, name, value, ordinal) VALUES (?, ?, ?, ?)`, table)
	for _, name := range names {
		for ordinal, value := range headers[name] {
			if _, err := tx.ExecContext(ctx, query, id, name, value, ordinal); err != nil {
				return fmt.Errorf("insert %s: %w", strings.TrimSuffix(table, "s"), err)
			}
		}
	}
	return nil
}

// Get returns one complete recorded exchange.
func (s *Store) Get(ctx context.Context, id int64) (recording.Exchange, error) {
	var ex recording.Exchange
	var started, ended string
	var durationNS int64
	err := s.db.QueryRowContext(ctx, `
SELECT id, protocol, started_at, completed_at, duration_ns, request_method, request_url, response_status, proxy_error
FROM exchanges WHERE id = ?`, id).Scan(&ex.ID, &ex.Protocol, &started, &ended, &durationNS,
		&ex.Request.Method, &ex.Request.URL, &ex.Response.StatusCode, &ex.ProxyError)
	if errors.Is(err, sql.ErrNoRows) {
		return ex, ErrNotFound
	}
	if err != nil {
		return ex, fmt.Errorf("read exchange: %w", err)
	}
	ex.StartedAt, err = time.Parse(time.RFC3339Nano, started)
	if err != nil {
		return ex, fmt.Errorf("parse exchange start time: %w", err)
	}
	ex.EndedAt, err = time.Parse(time.RFC3339Nano, ended)
	if err != nil {
		return ex, fmt.Errorf("parse exchange completion time: %w", err)
	}
	ex.Duration = time.Duration(durationNS)
	if ex.Request.Headers, err = s.readHeaders(ctx, "request_headers", id); err != nil {
		return ex, err
	}
	if ex.Response.Headers, err = s.readHeaders(ctx, "response_headers", id); err != nil {
		return ex, err
	}
	var requestCapturedSize int64
	if err := s.db.QueryRowContext(ctx, `SELECT content, original_size, captured_size, truncated FROM request_bodies WHERE exchange_id = ?`, id).
		Scan(&ex.Request.Body, &ex.Request.BodySize, &requestCapturedSize, &ex.Request.BodyTruncated); err != nil {
		return ex, fmt.Errorf("read request body: %w", err)
	}
	if err := validateBodyMetadata(ex.Request.Body, ex.Request.BodySize, requestCapturedSize, ex.Request.BodyTruncated); err != nil {
		return ex, fmt.Errorf("%w: request body metadata: %v", ErrInvalidRecording, err)
	}
	var responseCapturedSize int64
	if err := s.db.QueryRowContext(ctx, `SELECT content, original_size, captured_size, truncated FROM response_bodies WHERE exchange_id = ?`, id).
		Scan(&ex.Response.Body, &ex.Response.BodySize, &responseCapturedSize, &ex.Response.BodyTruncated); err != nil {
		return ex, fmt.Errorf("read response body: %w", err)
	}
	if err := validateBodyMetadata(ex.Response.Body, ex.Response.BodySize, responseCapturedSize, ex.Response.BodyTruncated); err != nil {
		return ex, fmt.Errorf("%w: response body metadata: %v", ErrInvalidRecording, err)
	}
	return ex, nil
}

func validateBodyMetadata(body []byte, originalSize, capturedSize int64, truncated bool) error {
	if capturedSize != int64(len(body)) {
		return fmt.Errorf("captured size %d does not match BLOB length %d", capturedSize, len(body))
	}
	if originalSize < capturedSize {
		return fmt.Errorf("original size %d is smaller than captured size %d", originalSize, capturedSize)
	}
	if originalSize > capturedSize && !truncated {
		return fmt.Errorf("body omits bytes but is not marked truncated")
	}
	return nil
}

func (s *Store) readHeaders(ctx context.Context, table string, id int64) (http.Header, error) {
	query := fmt.Sprintf(`SELECT name, value FROM %s WHERE exchange_id = ? ORDER BY name, ordinal`, table)
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", table, err)
	}
	defer rows.Close()
	headers := make(http.Header)
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, fmt.Errorf("scan %s: %w", table, err)
		}
		headers[name] = append(headers[name], value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", table, err)
	}
	return headers, nil
}

// List returns summaries ordered by request start time and stable exchange ID.
func (s *Store) List(ctx context.Context, filter recording.Filter) ([]recording.Summary, error) {
	query := `SELECT id, protocol, started_at, duration_ns, request_method, request_url, response_status FROM exchanges WHERE 1=1`
	var args []any
	if filter.Status != 0 {
		query += ` AND response_status = ?`
		args = append(args, filter.Status)
	}
	if filter.Method != "" {
		query += ` AND UPPER(request_method) = ?`
		args = append(args, strings.ToUpper(filter.Method))
	}
	query += ` ORDER BY started_at, id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list exchanges: %w", err)
	}
	defer rows.Close()
	var result []recording.Summary
	for rows.Next() {
		var item recording.Summary
		var started string
		var durationNS int64
		if err := rows.Scan(&item.ID, &item.Protocol, &started, &durationNS, &item.Method, &item.URL, &item.StatusCode); err != nil {
			return nil, fmt.Errorf("scan exchange summary: %w", err)
		}
		item.StartedAt, err = time.Parse(time.RFC3339Nano, started)
		if err != nil {
			return nil, fmt.Errorf("parse exchange start time: %w", err)
		}
		item.Duration = time.Duration(durationNS)
		if filter.Path != "" && !matchesPath(item.URL, filter.Path) {
			continue
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exchange summaries: %w", err)
	}
	return result, nil
}

func matchesPath(rawURL, wanted string) bool {
	if strings.Contains(wanted, "?") {
		return rawURL == wanted
	}
	parsed, err := url.ParseRequestURI(rawURL)
	return err == nil && parsed.EscapedPath() == wanted
}
