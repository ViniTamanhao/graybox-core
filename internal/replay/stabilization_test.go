package replay

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
)

func TestRunnerCancellationAndDeadline(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			entered := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := server.Client()
			if deadline {
				client.Timeout = 100 * time.Millisecond
			} else {
				go func() {
					select {
					case <-entered:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			target, _ := url.Parse(server.URL)
			results, err := (Runner{Source: fakeSource{recording.Exchange{ID: 1, Request: recording.Request{Method: "GET", URL: "/", Complete: true}}}, Client: client}).Run(ctx, target, nil)
			if err != nil || len(results) != 1 || results[0].Err == nil || results[0].StatusCode != 0 {
				t.Fatalf("unexpected result: %+v %v", results, err)
			}
			if deadline {
				var timeout net.Error
				if !errors.As(results[0].Err, &timeout) || !timeout.Timeout() {
					t.Fatalf("not timeout: %v", results[0].Err)
				}
			} else if !errors.Is(results[0].Err, context.Canceled) {
				t.Fatalf("not cancellation: %v", results[0].Err)
			}
		})
	}
}

func TestRunnerRefusedConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse("http://" + listener.Addr().String())
	listener.Close()
	results, err := (Runner{Source: fakeSource{recording.Exchange{ID: 1, Request: recording.Request{Method: "GET", URL: "/", Complete: true}}}}).Run(context.Background(), target, nil)
	if err != nil || len(results) != 1 || results[0].Err == nil || results[0].StatusCode != 0 {
		t.Fatalf("connection failure not reported: %+v %v", results, err)
	}
}

func TestRepeatedPOSTHasRealSideEffects(t *testing.T) {
	var created atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || string(body) != `{"name":"demo"}` {
			t.Error("request changed")
		}
		created.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	runner := Runner{Source: fakeSource{recording.Exchange{ID: 1, Request: recording.Request{Method: "POST", URL: "/resources", Body: []byte(`{"name":"demo"}`), Complete: true}}}}
	for range 2 {
		results, err := runner.Run(context.Background(), target, nil)
		if err != nil || len(results) != 1 || results[0].Err != nil || results[0].StatusCode != 201 {
			t.Fatalf("POST failed: %+v %v", results, err)
		}
	}
	if created.Load() != 2 {
		t.Fatalf("expected two explicit POST side effects, got %d", created.Load())
	}
}

func TestPartialCredentialReplacementRefusesRemainingMarker(t *testing.T) {
	request := httptest.NewRequest("POST", "http://example.test/?token=%3CREDACTED%3E", nil)
	request.Header.Set("Content-Type", "application/json")
	body := []byte(`{"first":"<REDACTED>","second":"<REDACTED>"}`)
	replacements := Replacements{Query: map[string]string{"token": "synthetic-query"}, JSON: map[string]string{"/first": "synthetic-first"}}
	if err := replacements.apply(request, body); err == nil {
		t.Fatal("partial body replacement accepted")
	}
	replacements.JSON["/second"] = "synthetic-second"
	if err := replacements.apply(request, body); err != nil {
		t.Fatal(err)
	}
	replaced, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(replaced) != `{"first":"synthetic-first","second":"synthetic-second"}` || request.URL.Query().Get("token") != "synthetic-query" {
		t.Fatal("complete replacements not applied")
	}
}
