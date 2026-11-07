package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
	"github.com/ViniTamanhao/graybox-core/internal/sanitize"
)

// Exercise command orchestration, shutdown, SQLite contents and both renderers.
func TestAuthenticatedLifecycle(t *testing.T) {
	t.Chdir(t.TempDir())
	const token = "Bearer synthetic-lifecycle-canary"
	t.Setenv("GRAYBOX_LIFECYCLE_AUTH", token)
	var regressed atomic.Bool
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != token {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=synthetic-cookie-canary")
		total := 4500
		if regressed.Load() {
			total = 5000
		}
		fmt.Fprintf(w, `{"total_cents":%d}`, total)
	}))
	defer api.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan net.Listener, 1)
	done := make(chan int, 1)
	var recordOut, recordErr bytes.Buffer
	app := App{Stdout: &recordOut, Stderr: &recordErr, listen: func(_, _ string) (net.Listener, error) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err == nil {
			ready <- listener
		}
		return listener, err
	}}
	path := "lifecycle.graybox"
	go func() { done <- app.Run(ctx, []string{"record", "--target", api.URL, "--output", path}) }()
	var listener net.Listener
	select {
	case listener = <-ready:
	case code := <-done:
		t.Fatalf("recorder startup: %d %s", code, &recordErr)
	case <-time.After(10 * time.Second):
		t.Fatal("recorder startup timed out")
	}
	request, _ := http.NewRequest("GET", "http://"+listener.Addr().String()+"/orders/42", nil)
	request.Header.Set("Authorization", token)
	request.Header.Set("Cookie", "session=synthetic-request-cookie")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(body) != `{"total_cents":4500}` {
		t.Fatalf("capture response: %d %s %v", response.StatusCode, body, err)
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("shutdown: %d %s", code, &recordErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("shutdown timed out")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{token, "synthetic-cookie-canary", "synthetic-request-cookie"} {
		if bytes.Contains(before, []byte(secret)) {
			t.Fatal("canary persisted")
		}
	}
	// Inspect raw SQLite columns independently of the storage domain reader.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var auth, cookie, responseCookie, method, uri, storedBody string
	var status int
	for _, q := range []struct {
		query string
		dest  *string
	}{
		{`SELECT value FROM request_headers WHERE name='Authorization'`, &auth},
		{`SELECT value FROM request_headers WHERE name='Cookie'`, &cookie},
		{`SELECT value FROM response_headers WHERE name='Set-Cookie'`, &responseCookie},
		{`SELECT request_method FROM exchanges`, &method},
		{`SELECT request_url FROM exchanges`, &uri},
		{`SELECT content FROM response_bodies`, &storedBody},
	} {
		if err := db.QueryRow(q.query).Scan(q.dest); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(`SELECT response_status FROM exchanges`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if auth != sanitize.RedactedValue || cookie != sanitize.RedactedValue || responseCookie != sanitize.RedactedValue || method != "GET" || uri != "/orders/42" || status != 200 || storedBody != string(body) {
		t.Fatal("stored exchange differs from expected sanitized capture")
	}
	run := func(want int, args ...string) []byte {
		t.Helper()
		var out, errs bytes.Buffer
		code := (App{Stdout: &out, Stderr: &errs}).Run(context.Background(), args)
		if code != want || errs.Len() != 0 {
			t.Fatalf("%v: exit=%d stderr=%s output=%s", args, code, &errs, &out)
		}
		if bytes.Contains(out.Bytes(), []byte(token)) {
			t.Fatal("output leaked credential")
		}
		return out.Bytes()
	}
	for _, args := range [][]string{{"ls", path, "--json"}, {"show", path, "1", "--json"}} {
		if !json.Valid(run(0, args...)) {
			t.Fatal("invalid inspection JSON")
		}
	}
	for _, jsonOutput := range []bool{false, true} {
		replayArgs := []string{"replay", path, "--secret-header", "Authorization=GRAYBOX_LIFECYCLE_AUTH"}
		if jsonOutput {
			replayArgs = append(replayArgs, "--json")
		}
		output := run(0, replayArgs...)
		if jsonOutput {
			var report struct {
				Succeeded int `json:"succeeded"`
				Results   []struct {
					Status int `json:"status"`
				} `json:"results"`
			}
			if err := json.Unmarshal(output, &report); err != nil || report.Succeeded != 1 || len(report.Results) != 1 || report.Results[0].Status != 200 {
				t.Fatalf("replay not authenticated: %s", output)
			}
		} else if !bytes.Contains(output, []byte("200 OK")) {
			t.Fatal("replay not authenticated")
		}
		for _, changed := range []bool{false, true, false} {
			regressed.Store(changed)
			args := []string{"diff", path, "--secret-header", "Authorization=GRAYBOX_LIFECYCLE_AUTH"}
			if jsonOutput {
				args = append(args, "--json")
			}
			want, outcome := 0, "equivalent"
			if changed {
				want, outcome = 1, "changed"
			}
			output := run(want, args...)
			if jsonOutput {
				var report struct {
					Summary struct{ Total, Equivalent, Changed, Failed int }
					Results []struct {
						Outcome     string
						Differences []struct {
							Location      struct{ Component, Path string }
							Before, After struct{ Value int }
						}
					}
				}
				if err := json.Unmarshal(output, &report); err != nil || report.Summary.Total != 1 || report.Summary.Failed != 0 || len(report.Results) != 1 || report.Results[0].Outcome != outcome {
					t.Fatalf("diff outcome: %s", output)
				}
				if changed {
					d := report.Results[0].Differences
					if len(d) != 1 || d[0].Location.Path != "/total_cents" || d[0].Before.Value != 4500 || d[0].After.Value != 5000 {
						t.Fatalf("wrong semantic difference: %s", output)
					}
				}
			} else if !bytes.Contains(output, []byte(outcome)) {
				t.Fatalf("missing outcome: %s", output)
			}
		}
	}
	if calls.Load() != 9 {
		t.Fatalf("expected capture plus eight explicit executions, got %d", calls.Load())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("inspection/replay/diff modified recording")
	}
}

func TestHTTPErrorStatusesAreCompletedComparisons(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, status := range []int{401, 403, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "rejected")
			}))
			defer server.Close()
			path := hardeningRecording(t, server.URL, []recording.Request{{Method: "GET", URL: "/", Complete: true}}, []recording.Response{{StatusCode: 200, Body: []byte("ok"), Complete: true}})
			for _, command := range []string{"replay", "diff"} {
				for _, machine := range []bool{false, true} {
					var out, errs bytes.Buffer
					args := []string{command, path}
					if machine {
						args = append(args, "--json")
					}
					want := 0
					if command == "diff" {
						want = 1
					}
					code := (App{Stdout: &out, Stderr: &errs}).Run(context.Background(), args)
					if code != want || errs.Len() != 0 {
						t.Fatalf("%s status %d: exit %d %s", command, status, code, &errs)
					}
					if machine && !json.Valid(out.Bytes()) {
						t.Fatal("invalid JSON")
					}
					if command == "diff" && !strings.Contains(out.String(), "changed") {
						t.Fatal("HTTP response was not a completed comparison")
					}
					if command == "replay" && !strings.Contains(out.String(), fmt.Sprint(status)) {
						t.Fatal("HTTP rejection status hidden")
					}
				}
			}
		})
	}
}

func TestEmptyAndInvalidRecordingsCLI(t *testing.T) {
	t.Chdir(t.TempDir())
	empty := hardeningRecording(t, "http://127.0.0.1:1", nil, nil)
	invalid := filepath.Join(t.TempDir(), "invalid.graybox")
	if err := os.WriteFile(invalid, []byte("not SQLite"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"replay", "diff"} {
		for _, machine := range []bool{false, true} {
			for _, test := range []struct {
				path string
				id   bool
				want int
			}{{empty, false, 0}, {empty, true, 2}, {invalid, false, 2}} {
				var out, errs bytes.Buffer
				args := []string{command, test.path}
				if machine {
					args = append(args, "--json")
				}
				if test.id {
					args = append(args, "--id", "1")
				}
				code := (App{Stdout: &out, Stderr: &errs}).Run(context.Background(), args)
				if code != test.want {
					t.Fatalf("%v: %d %s", args, code, &errs)
				}
				if code == 0 && machine && !json.Valid(out.Bytes()) {
					t.Fatal("invalid empty JSON")
				}
				if code != 0 && (out.Len() != 0 || errs.Len() == 0) {
					t.Fatal("command error must be on stderr")
				}
			}
		}
	}
}

func TestUnreplacedCredentialMarkersNeverReachTarget(t *testing.T) {
	t.Chdir(t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Error("unreplaced credential marker reached target") }))
	defer server.Close()
	for _, request := range []recording.Request{
		{Method: "POST", URL: "/", Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"nested":{"token":"<REDACTED>"}}`), Complete: true},
		{Method: "POST", URL: "/", Headers: http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, Body: []byte("token=%3CREDACTED%3E"), Complete: true},
		{Method: "GET", URL: "/?token=%3CREDACTED%3E", Complete: true},
	} {
		path := hardeningRecording(t, server.URL, []recording.Request{request}, []recording.Response{{StatusCode: 200, Complete: true}})
		for _, command := range []string{"replay", "diff"} {
			for _, machine := range []bool{false, true} {
				var out, errs bytes.Buffer
				args := []string{command, path}
				if machine {
					args = append(args, "--json")
				}
				code := (App{Stdout: &out, Stderr: &errs}).Run(context.Background(), args)
				if code != 3 || errs.Len() != 0 || !strings.Contains(out.String(), "redacted credential") {
					t.Errorf("%v: code %d stdout %s stderr %s", args, code, &out, &errs)
				}
				if machine && !json.Valid(out.Bytes()) {
					t.Error("invalid JSON")
				}
			}
		}
	}
}
