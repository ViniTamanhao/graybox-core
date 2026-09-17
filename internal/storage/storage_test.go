package storage

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
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
	if !errors.Is(err, ErrInvalidRecording) {
		t.Fatalf("Open error = %v, want ErrInvalidRecording", err)
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
