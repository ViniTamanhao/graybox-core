package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
	"github.com/ViniTamanhao/graybox-core/internal/storage"
)

func TestListAndShowJSONAreValidAndSharePersistedResults(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cli.graybox")
	store, err := storage.Create(ctx, path, "test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = store.Add(ctx, recording.Exchange{
		Protocol:   "http",
		StartedAt:  now,
		EndedAt:    now.Add(time.Millisecond),
		Duration:   time.Millisecond,
		ProxyError: "dial upstream: connection refused",
		Request: recording.Request{
			Method:       http.MethodPost,
			URL:          "/checkout?try=1",
			Headers:      http.Header{"Content-Type": {"application/json"}},
			Body:         []byte(`{"cart":1}`),
			ObservedSize: 10,
			Complete:     true,
		},
		Response: recording.Response{
			StatusCode:   http.StatusInternalServerError,
			Headers:      http.Header{"Content-Type": {"application/octet-stream"}},
			Body:         []byte{0, 255},
			ObservedSize: 2,
			Complete:     true,
		},
	})
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

type failingRecordingStore struct {
	addCalls atomic.Uint64
	closed   atomic.Bool
}

func (s *failingRecordingStore) Add(context.Context, recording.Exchange) (int64, error) {
	s.addCalls.Add(1)
	return 0, errors.New("simulated persistence failure")
}

func (*failingRecordingStore) SetMetadata(context.Context, string, string) error { return nil }
func (s *failingRecordingStore) Close() error {
	s.closed.Store(true)
	return nil
}

func TestRecordShutdownReportsPersistenceFailuresAndFails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	store := new(failingRecordingStore)
	var stdout, stderr bytes.Buffer
	app := App{
		Stdout: &stdout,
		Stderr: &stderr,
		createRecording: func(context.Context, string, string) (recordingStore, error) {
			return store, nil
		},
		listen: func(string, string) (net.Listener, error) { return listener, nil },
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		done <- app.Run(ctx, []string{"record", "--listen", "127.0.0.1:0", "--target", upstream.URL, "--output", "ignored.graybox", "--json"})
	}()

	for range 2 {
		resp, err := http.Get("http://" + listener.Addr().String() + "/persist-me")
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			cancel()
			t.Fatalf("proxied response status = %d", resp.StatusCode)
		}
	}
	cancel()
	select {
	case code := <-done:
		if code != ExitInternal {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("record command did not shut down")
	}
	if store.addCalls.Load() != 2 || !store.closed.Load() {
		t.Fatalf("add calls/closed = %d/%v", store.addCalls.Load(), store.closed.Load())
	}
	if !strings.Contains(stderr.String(), "Recording completed with errors:\n2 exchanges could not be persisted") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if !json.Valid(stdout.Bytes()) || strings.Contains(stdout.String(), "Recording completed") {
		t.Fatalf("JSON stdout = %q", stdout.String())
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
	for _, raw := range []string{"http://localhost.example.com", "http://127.0.0.1.example.com", "http://192.0.2.1"} {
		target, err := parseTarget(raw)
		if err != nil || isLoopbackTarget(target) {
			t.Errorf("target %q local = true, err %v", raw, err)
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

func TestVersionJSONIsStableAndValid(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := (App{Stdout: &stdout, Stderr: &stderr, Version: "v0.1.0"}).Run(context.Background(), []string{"version", "--json"})
	if code != ExitSuccess || stdout.String() != "{\"version\":\"v0.1.0\"}\n" || !json.Valid(stdout.Bytes()) || stderr.Len() != 0 {
		t.Fatalf("exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
}

func TestAllHelpCommands(t *testing.T) {
	commands := [][]string{{"help"}, {"help", "record"}, {"help", "ls"}, {"help", "show"}, {"help", "replay"}, {"help", "version"}}
	for _, args := range commands {
		var stdout, stderr bytes.Buffer
		code := (App{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), args)
		if code != ExitSuccess || stdout.Len() == 0 || stderr.Len() != 0 {
			t.Errorf("args/exit/stdout/stderr = %v/%d/%q/%q", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestShowHelpAllowsIncompleteExchanges(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := (App{Stdout: &stdout, Stderr: &stderr}).Run(
		context.Background(),
		[]string{"show", "--help"},
	)
	if code != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("exit/stderr = %d/%q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "complete recorded exchange") ||
		!strings.Contains(stdout.String(), "incomplete bodies are labeled") {
		t.Fatalf("show help = %q", stdout.String())
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
