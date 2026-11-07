package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
)

// Decode independently of the production DTOs: changes to those structs must
// not silently change the expectations of an agent consuming the CLI.
func TestAgentDiffOutcomeContract(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, tc := range []struct {
		name     string
		outcomes []string
		code     int
	}{
		{"equivalent", []string{"equivalent", "equivalent"}, 0},
		{"one change", []string{"equivalent", "changed"}, 1},
		{"multiple changes", []string{"changed", "changed"}, 1},
		{"failed", []string{"failed"}, 3},
		{"mixed continues after failure", []string{"changed", "failed", "equivalent"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var visited []string
			var visitMu sync.Mutex
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				visitMu.Lock()
				visited = append(visited, r.URL.Path)
				visitMu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/failed" {
					w.WriteHeader(201)
				}
				value := 1
				if r.URL.Path == "/changed" {
					value = 2
				}
				fmt.Fprintf(w, `{"value":%d}`, value)
			}))
			defer server.Close()
			var requests []recording.Request
			var responses []recording.Response
			var wantPaths []string
			counts := map[string]int{"total": len(tc.outcomes), "equivalent": 0, "changed": 0, "failed": 0}
			for _, outcome := range tc.outcomes {
				wantPaths = append(wantPaths, "/"+outcome)
				requests = append(requests, recording.Request{Method: "GET", URL: "/" + outcome, Complete: true})
				responses = append(responses, recording.Response{StatusCode: 200, Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"value":1}`), Complete: outcome != "failed"})
				counts[outcome]++
			}
			path := hardeningRecording(t, server.URL, requests, responses)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var out, errs bytes.Buffer
			code := (App{Stdout: &out, Stderr: &errs}).Run(context.Background(), []string{"diff", path, "--json"})
			if code != tc.code || errs.Len() != 0 {
				t.Fatalf("exit=%d stderr=%s output=%s", code, &errs, &out)
			}
			var report struct {
				Recording, Target string
				BodyCaptureLimit  int64 `json:"body_capture_limit"`
				Ignored           []string
				Summary           map[string]int
				Results           []struct {
					ID                    int64 `json:"exchange_id"`
					Method, Path, Outcome string
					BaselineDuration      float64 `json:"baseline_duration_ms"`
					CurrentDuration       float64 `json:"current_duration_ms"`
					Differences           []struct {
						Kind          string
						Location      struct{ Component, Path string }
						Before, After struct {
							Present bool
							Value   json.Number
						}
					}
					Error string
				}
			}
			if err := json.Unmarshal(out.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if report.Recording != path || report.Target != server.URL || report.BodyCaptureLimit <= 0 || len(report.Ignored) != 2 || !reflect.DeepEqual(report.Summary, counts) || len(report.Results) != len(tc.outcomes) {
				t.Fatalf("report metadata/counts: %s", &out)
			}
			for i, result := range report.Results {
				if result.ID != int64(i+1) || result.Method != "GET" || result.Path != wantPaths[i] || result.Outcome != tc.outcomes[i] || result.Differences == nil || result.BaselineDuration < 0 || result.CurrentDuration < 0 {
					t.Fatalf("result identity/types: %s", &out)
				}
				if (result.Error != "") != (result.Outcome == "failed") {
					t.Fatalf("error/outcome: %s", &out)
				}
				if result.Outcome == "equivalent" {
					if len(result.Differences) != 0 {
						t.Fatalf("equivalent differences: %s", &out)
					}
					continue
				}
				if len(result.Differences) != 1 {
					t.Fatalf("differences: %s", &out)
				}
				d := result.Differences[0]
				kind, component, location, old, current := "value_changed", "response.body", "/value", "1", "2"
				if result.Outcome == "failed" {
					kind, component, location, old, current = "status_changed", "response.status", "", "200", "201"
				}
				if d.Kind != kind || d.Location.Component != component || d.Location.Path != location || !d.Before.Present || !d.After.Present || d.Before.Value.String() != old || d.After.Value.String() != current {
					t.Fatalf("structured difference: %s", &out)
				}
			}
			visitMu.Lock()
			defer visitMu.Unlock()
			if !reflect.DeepEqual(visited, wantPaths) {
				t.Fatalf("requests = %v, want %v", visited, wantPaths)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("baseline changed")
			}
		})
	}
}

func TestAgentReplayFailureCardinality(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("incomplete request sent") }))
			defer server.Close()
			requests := make([]recording.Request, count)
			responses := make([]recording.Response, count)
			for i := range requests {
				requests[i] = recording.Request{Method: "POST", URL: "/", Complete: false}
				responses[i] = recording.Response{StatusCode: 200, Complete: true}
			}
			path := hardeningRecording(t, server.URL, requests, responses)
			var out, errs bytes.Buffer
			code := (App{Stdout: &out, Stderr: &errs}).Run(context.Background(), []string{"replay", path, "--json"})
			want := 1
			if count == 1 {
				want = 3
			}
			var report struct {
				Succeeded, Failed int
				Results           []struct {
					Error     string
					Status    *int
					TargetURL string `json:"target_url"`
				}
			}
			if err := json.Unmarshal(out.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if code != want || errs.Len() != 0 || report.Succeeded != 0 || report.Failed != count || len(report.Results) != count {
				t.Fatalf("exit=%d output=%s stderr=%s", code, &out, &errs)
			}
			for _, result := range report.Results {
				if result.Error == "" || result.Status != nil {
					t.Fatalf("failed result: %s", &out)
				}
			}
		})
	}
}

func TestAgentCommandFailureChannels(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GRAYBOX_AGENT_MISSING", "")
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("preflight failure sent traffic") }))
	defer server.Close()
	path := hardeningRecording(t, server.URL, []recording.Request{{Method: "GET", URL: "/", Complete: true}}, []recording.Response{{StatusCode: 200, Complete: true}})
	invalidConfig := writeHardeningConfig(t, "unknown: true\n")
	for _, command := range []string{"replay", "diff"} {
		for _, extra := range [][]string{
			{"--secret-header", "Authorization=GRAYBOX_AGENT_MISSING"},
			{"--config", invalidConfig},
			{"--target", "localhost:8080"},
		} {
			var out, errs bytes.Buffer
			args := append([]string{command, path, "--json"}, extra...)
			code := (App{Stdout: &out, Stderr: &errs}).Run(context.Background(), args)
			if code != 4 || out.Len() != 0 || errs.Len() == 0 {
				t.Fatalf("%v: exit=%d stdout=%s stderr=%s", args, code, &out, &errs)
			}
		}
	}
}
