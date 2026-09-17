package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opemori/graybox-core/internal/recording"
	"github.com/opemori/graybox-core/internal/storage"
)

func TestListAndShowJSONAreValidAndSharePersistedResults(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cli.graybox")
	store, err := storage.Create(ctx, path, "test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = store.Add(ctx, recording.Exchange{Protocol: "http", StartedAt: now, EndedAt: now.Add(time.Millisecond), Duration: time.Millisecond,
		Request:  recording.Request{Method: http.MethodPost, URL: "/checkout?try=1", Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"cart":1}`)},
		Response: recording.Response{StatusCode: http.StatusInternalServerError, Headers: http.Header{"Content-Type": {"application/octet-stream"}}, Body: []byte{0, 255}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"list", []string{"ls", path, "--method", "post", "--path", "/checkout", "--status", "500", "--json"}},
		{"show", []string{"show", path, "1", "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := (App{Stdout: &stdout, Stderr: &stderr, Version: "test"}).Run(ctx, tc.args)
			if code != ExitSuccess {
				t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
			}
			if !json.Valid(stdout.Bytes()) {
				t.Fatalf("invalid JSON: %q", stdout.String())
			}
			if !strings.Contains(stdout.String(), `"status":500`) {
				t.Fatalf("persisted status missing from %q", stdout.String())
			}
		})
	}
}

func TestUsageErrorsStayOnStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := (App{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), []string{"show", "only-a-file", "--json"})
	if code != ExitUsage || stdout.Len() != 0 || !strings.Contains(stderr.String(), "requires a recording file and exchange ID") {
		t.Fatalf("exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
}

func TestParseTargetExplainsMissingScheme(t *testing.T) {
	_, err := parseTarget("localhost:8080")
	if err == nil || err.Error() != `target "localhost:8080" is missing a URL scheme; try http://localhost:8080` {
		t.Fatalf("error = %v", err)
	}
}
