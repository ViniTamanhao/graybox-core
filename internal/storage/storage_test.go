package storage

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/opemori/graybox-core/internal/recording"
)

func TestCreateAddGetAndFilter(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.graybox")
	store, err := Create(ctx, path, "test-version")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SetMetadata(ctx, "target_url", "http://example.test"); err != nil {
		t.Fatal(err)
	}
	if version, err := store.Metadata(ctx, "schema_version"); err != nil || version != "1" {
		t.Fatalf("schema version = %q, %v", version, err)
	}
	if format, err := store.Metadata(ctx, "format"); err != nil || format != "graybox" {
		t.Fatalf("format = %q, %v", format, err)
	}

	started := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	exchange := recording.Exchange{
		Protocol: "http", StartedAt: started, EndedAt: started.Add(1250 * time.Microsecond), Duration: 1250 * time.Microsecond,
		Request: recording.Request{Method: "POST", URL: "/items?q=one%20two", Headers: http.Header{
			"X-Repeated": {"first", "second"}, "Content-Type": {"application/octet-stream"}}, Body: []byte{0, 1, 2, 255}},
		Response: recording.Response{StatusCode: 201, Headers: http.Header{"Set-Cookie": {"<REDACTED>", "<REDACTED>"}}, Body: []byte{9, 0, 8}},
	}
	id, err := store.Add(ctx, exchange)
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("first ID = %d, want 1", id)
	}

	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Request.Body, exchange.Request.Body) || !bytes.Equal(got.Response.Body, exchange.Response.Body) {
		t.Fatal("binary bodies were not preserved")
	}
	if got.Request.BodySize != int64(len(exchange.Request.Body)) || got.Request.BodyTruncated || got.Response.BodySize != int64(len(exchange.Response.Body)) {
		t.Fatalf("body metadata changed: request=%#v response=%#v", got.Request, got.Response)
	}
	if !reflect.DeepEqual(got.Request.Headers.Values("X-Repeated"), []string{"first", "second"}) {
		t.Fatalf("repeated request headers = %#v", got.Request.Headers.Values("X-Repeated"))
	}
	if got.Duration != exchange.Duration || !got.StartedAt.Equal(started) {
		t.Fatalf("timing changed: %#v", got)
	}

	for _, tc := range []struct {
		name   string
		filter recording.Filter
		count  int
	}{
		{"all", recording.Filter{}, 1},
		{"method case insensitive", recording.Filter{Method: "post"}, 1},
		{"status", recording.Filter{Status: 201}, 1},
		{"path ignores query", recording.Filter{Path: "/items"}, 1},
		{"request uri", recording.Filter{Path: "/items?q=one%20two"}, 1},
		{"path mismatch", recording.Filter{Path: "/other"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items, err := store.List(ctx, tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != tc.count {
				t.Fatalf("got %d items, want %d", len(items), tc.count)
			}
		})
	}
	value, err := store.Metadata(ctx, "target_url")
	if err != nil || value != "http://example.test" {
		t.Fatalf("metadata = %q, %v", value, err)
	}
}

func TestListMatchesEscapedPathsWithoutDecoding(t *testing.T) {
	ctx := context.Background()
	store, err := Create(ctx, filepath.Join(t.TempDir(), "escaped.graybox"), "dev")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	for _, requestURL := range []string{"/users/a%2Fb?view=1", "/users/a/b?view=1"} {
		_, err := store.Add(ctx, recording.Exchange{Protocol: "http", StartedAt: now, EndedAt: now,
			Request: recording.Request{Method: http.MethodGet, URL: requestURL}, Response: recording.Response{StatusCode: http.StatusOK}})
		if err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.List(ctx, recording.Filter{Path: "/users/a%2Fb"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].URL != "/users/a%2Fb?view=1" {
		t.Fatalf("escaped path matches = %#v", items)
	}
}

func TestListOrdersByStartedAtThenID(t *testing.T) {
	ctx := context.Background()
	store, err := Create(ctx, filepath.Join(t.TempDir(), "ordered.graybox"), "dev")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	for _, started := range []time.Time{now.Add(time.Second), now} {
		_, err := store.Add(ctx, recording.Exchange{Protocol: "http", StartedAt: started, EndedAt: started,
			Request: recording.Request{Method: http.MethodGet, URL: "/"}, Response: recording.Response{StatusCode: http.StatusOK}})
		if err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.List(ctx, recording.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != 2 || items[1].ID != 1 {
		t.Fatalf("ordered IDs = %#v", items)
	}
}

func TestOpenReadOnlyRejectsWritesAndClosesCleanly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "readonly.graybox")
	store, err := Create(ctx, path, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if !store.readOnly {
		t.Fatal("store is not marked read-only")
	}
	if err := store.SetMetadata(ctx, "unexpected", "write"); err == nil {
		t.Fatal("write through read-only store unexpectedly succeeded")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close read-only store: %v", err)
	}
}

func TestSQLiteReadOnlyDSNPortablePaths(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		windows  bool
		expected string
	}{
		{
			name:     "Unix",
			path:     "/tmp/gray box/capture#1?.graybox",
			expected: "file:///tmp/gray%20box/capture%231%3F.graybox?mode=ro",
		},
		{
			name:     "Windows drive",
			path:     `C:\Users\Gray Box\capture#1?.graybox`,
			windows:  true,
			expected: "file:///C:/Users/Gray%20Box/capture%231%3F.graybox?mode=ro",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sqliteReadOnlyDSNFromAbsolute(test.path, test.windows); got != test.expected {
				t.Fatalf("DSN = %q, want %q", got, test.expected)
			}
		})
	}
}

func TestOpenReadOnlyWorksWithReadOnlyFilesystemPermissions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "filesystem-readonly.graybox")
	store, err := Create(ctx, path, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	store, err = OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(ctx, recording.Filter{}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaVersionValidation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "future.graybox")
	store, err := Create(ctx, path, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE metadata SET value = '999' WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Open(ctx, path)
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("Open error = %v, want ErrUnsupportedSchema", err)
	}
}

func TestIncompleteSchemaIsInvalid(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "incomplete.graybox")
	store, err := Create(ctx, path, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE response_bodies`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Open(ctx, path)
	if !errors.Is(err, ErrInvalidRecording) || !strings.Contains(err.Error(), "response_bodies") {
		t.Fatalf("Open error = %v, want ErrInvalidRecording", err)
	}
}

func TestMissingSchemaIndexIsInvalid(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "missing-index.graybox")
	store, err := Create(ctx, path, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP INDEX exchanges_method_idx`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = OpenReadOnly(ctx, path)
	if !errors.Is(err, ErrInvalidRecording) || !strings.Contains(err.Error(), "exchanges_method_idx") {
		t.Fatalf("OpenReadOnly error = %v", err)
	}
}

func TestPreReleaseBodyConstraintIsInvalid(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "prototype.graybox")
	store, err := openDB(path, false)
	if err != nil {
		t.Fatal(err)
	}
	prototypeSchema := strings.ReplaceAll(schemaV1,
		`CHECK ((truncated = 0 AND captured_size = original_size) OR
           (truncated = 1 AND captured_size < original_size))`,
		`CHECK (truncated = 1 OR captured_size = original_size)`)
	if _, err := store.db.ExecContext(ctx, prototypeSchema); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"format": "graybox", "schema_version": "1", "created_at": nowUTC(), "graybox_version": "prototype",
	} {
		if _, err := store.db.ExecContext(ctx, `INSERT INTO metadata(key, value) VALUES (?, ?)`, key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = OpenReadOnly(ctx, path)
	if !errors.Is(err, ErrInvalidRecording) || !strings.Contains(err.Error(), "incompatible body metadata constraints") {
		t.Fatalf("OpenReadOnly error = %v", err)
	}
}

func TestCreateDoesNotOverwrite(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "existing.graybox")
	store, err := Create(ctx, path, "dev")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := Create(ctx, path, "dev"); err == nil {
		t.Fatal("second Create unexpectedly succeeded")
	}
}

func TestCreateUsesRestrictivePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.graybox")
	store, err := Create(context.Background(), path, "dev")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("recording permissions = %o, want 600", got)
	}
}

func TestAddRollsBackWholeExchangeOnBodyMetadataFailure(t *testing.T) {
	ctx := context.Background()
	store, err := Create(ctx, filepath.Join(t.TempDir(), "atomic.graybox"), "dev")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	_, err = store.Add(ctx, recording.Exchange{
		Protocol:  "http",
		StartedAt: now,
		EndedAt:   now,
		Request:   recording.Request{Method: http.MethodPost, URL: "/", Body: []byte("complete")},
		Response:  recording.Response{StatusCode: http.StatusOK, Body: []byte("too-large"), BodySize: 1},
	})
	if err == nil || !strings.Contains(err.Error(), "response body metadata") {
		t.Fatalf("Add error = %v", err)
	}
	items, err := store.List(ctx, recording.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("partial exchange committed: %#v", items)
	}
}

func TestBodyMetadataRejectsContradictoryTruncation(t *testing.T) {
	if _, _, err := bodyMetadata([]byte("all"), 3, true); err == nil {
		t.Fatal("body marked truncated with equal sizes was accepted")
	}
	if err := validateBodyMetadata([]byte("all"), 3, 3, true); err == nil {
		t.Fatal("persisted body marked truncated with equal sizes was accepted")
	}
}

func TestEmptyBodiesAreStoredAsZeroLengthBlobs(t *testing.T) {
	ctx := context.Background()
	store, err := Create(ctx, filepath.Join(t.TempDir(), "empty.graybox"), "dev")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	id, err := store.Add(ctx, recording.Exchange{Protocol: "http", StartedAt: now, EndedAt: now,
		Request:  recording.Request{Method: http.MethodGet, URL: "/empty", Headers: make(http.Header)},
		Response: recording.Response{StatusCode: http.StatusNoContent, Headers: make(http.Header)}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Request.Body) != 0 || len(got.Response.Body) != 0 {
		t.Fatalf("empty bodies were not zero-length BLOBs: request=%#v response=%#v", got.Request.Body, got.Response.Body)
	}
	var requestType, responseType string
	if err := store.db.QueryRow(`SELECT typeof(rb.content), typeof(sb.content)
FROM request_bodies rb JOIN response_bodies sb USING(exchange_id) WHERE rb.exchange_id = ?`, id).Scan(&requestType, &responseType); err != nil {
		t.Fatal(err)
	}
	if requestType != "blob" || responseType != "blob" {
		t.Fatalf("empty body SQLite types = %s/%s, want blob/blob", requestType, responseType)
	}
}
