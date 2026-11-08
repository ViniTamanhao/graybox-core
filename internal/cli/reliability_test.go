package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
)

// An output error is too late to promise that requests had no effects. Exercise
// both renderers and commands without depending on a platform-specific device.
func TestAgentOutputFailureAfterSideEffect(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, command := range []string{"diff", "replay"} {
		for _, machine := range []bool{false, true} {
			t.Run(command+map[bool]string{false: "/human", true: "/json"}[machine], func(t *testing.T) {
				var writes atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost {
						t.Errorf("method = %s", r.Method)
					}
					writes.Add(1)
					w.WriteHeader(http.StatusNoContent)
				}))
				defer server.Close()
				path := hardeningRecording(t, server.URL,
					[]recording.Request{{Method: "POST", URL: "/write", Complete: true}},
					[]recording.Response{{StatusCode: 204, Complete: true}})
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var stderr bytes.Buffer
				stdout := &partialReportWriter{}
				args := []string{command, path}
				if machine {
					args = append(args, "--json")
				}
				code := (App{Stdout: stdout, Stderr: &stderr}).Run(context.Background(), args)
				if code != 5 || writes.Load() != 1 || stdout.Len() != 10 || !strings.Contains(stderr.String(), "write output") {
					t.Fatalf("exit=%d requests=%d stdout=%q stderr=%s", code, writes.Load(), stdout.String(), &stderr)
				}
				if machine && json.Valid(stdout.Bytes()) {
					t.Fatal("partial report unexpectedly valid")
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("baseline changed")
				}
			})
		}
	}
}

type partialReportWriter struct{ bytes.Buffer }

func (w *partialReportWriter) Write(data []byte) (int, error) {
	n, _ := w.Buffer.Write(data[:min(10, len(data))])
	return n, errors.New("synthetic output failure")
}

func (w *partialReportWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}
