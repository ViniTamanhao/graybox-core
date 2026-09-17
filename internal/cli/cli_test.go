package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"runtime/debug"
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
		Request:    recording.Request{Method: http.MethodPost, URL: "/checkout?try=1", Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"cart":1}`)},
		ProxyError: "dial upstream: connection refused",
		Response:   recording.Response{StatusCode: http.StatusInternalServerError, Headers: http.Header{"Content-Type": {"application/octet-stream"}}, Body: []byte{0, 255}}})
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
			if tc.name == "show" && !strings.Contains(stdout.String(), `"proxy_error":"dial upstream: connection refused"`) {
				t.Fatalf("proxy error missing from %q", stdout.String())
			}
		})
	}
	var stdout, stderr bytes.Buffer
	code := (App{Stdout: &stdout, Stderr: &stderr, Version: "test"}).Run(ctx, []string{"show", path, "1"})
	if code != ExitSuccess || !strings.Contains(stdout.String(), "Proxy error: dial upstream: connection refused") {
		t.Fatalf("human show exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
}

func TestRecordDefaultListenIsLoopback(t *testing.T) {
	if defaultListenAddress != "127.0.0.1:9000" {
		t.Fatalf("default listen address = %q", defaultListenAddress)
	}
	var stdout, stderr bytes.Buffer
	code := (App{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), []string{"record", "--help"})
	if code != ExitSuccess || !strings.Contains(stdout.String(), "default 127.0.0.1:9000") {
		t.Fatalf("help exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	if got := displayListen(":9000"); got != "http://0.0.0.0:9000" {
		t.Fatalf("wildcard display = %q", got)
	}
}

func TestReplayProtectsSavedRemoteTarget(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "remote.graybox")
	store, err := storage.Create(ctx, path, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetMetadata(ctx, "target_url", "https://api.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := (App{Stdout: &stdout, Stderr: &stderr}).Run(ctx, []string{"replay", path})
	if code != ExitUsage || !strings.Contains(stderr.String(), "is not loopback") || !strings.Contains(stderr.String(), "--target") {
		t.Fatalf("exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = (App{Stdout: &stdout, Stderr: &stderr}).Run(ctx, []string{"replay", path, "--unsafe-original-target"})
	if code != ExitSuccess || !strings.Contains(stdout.String(), "Replaying 0 exchanges") {
		t.Fatalf("unsafe override exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	for _, raw := range []string{"http://localhost:8000", "http://service.localhost", "http://127.0.0.1", "http://[::1]"} {
		target, err := parseTarget(raw)
		if err != nil || !isLoopbackTarget(target) {
			t.Errorf("target %q local = false, err %v", raw, err)
		}
	}
}

func TestVersionFallsBackToModuleBuildInfo(t *testing.T) {
	original := readBuildInfo
	t.Cleanup(func() { readBuildInfo = original })
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, true
	}
	var stdout, stderr bytes.Buffer
	code := (App{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), []string{"version"})
	if code != ExitSuccess || stdout.String() != "graybox v0.1.0\n" || stderr.Len() != 0 {
		t.Fatalf("exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	code = (App{Stdout: &stdout, Stderr: &stderr, Version: "v9.9.9"}).Run(context.Background(), []string{"version"})
	if code != ExitSuccess || stdout.String() != "graybox v9.9.9\n" {
		t.Fatalf("ldflags override exit/stdout = %d/%q", code, stdout.String())
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
