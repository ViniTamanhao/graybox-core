package replay

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/opemori/graybox-core/internal/recording"
	"github.com/opemori/graybox-core/internal/sanitize"
)

type fakeSource struct{ exchange recording.Exchange }

func (f fakeSource) Get(context.Context, int64) (recording.Exchange, error) { return f.exchange, nil }
func (f fakeSource) List(context.Context, recording.Filter) ([]recording.Summary, error) {
	return []recording.Summary{{ID: f.exchange.ID}}, nil
}

func TestBuildURL(t *testing.T) {
	target, _ := url.Parse("https://example.test/api/")
	got, err := BuildURL(target, "/users/a%2Fb?q=hello%20world")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "https://example.test/api/users/a%2Fb?q=hello%20world" {
		t.Fatalf("URL = %q", got.String())
	}
}

func TestRunnerPreservesRequestAndOmitsUnsafeHeaders(t *testing.T) {
	var method, requestURI, body, authorization, connection, hop string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, requestURI = r.Method, r.URL.RequestURI()
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		authorization, connection = r.Header.Get("Authorization"), r.Header.Get("Connection")
		hop = r.Header.Get("X-Hop")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	ex := recording.Exchange{ID: 7, Request: recording.Request{Method: "PATCH", URL: "/thing?q=1", Body: []byte("payload"), Headers: http.Header{
		"Authorization": {sanitize.RedactedValue}, "Connection": {"close, X-Hop"}, "X-Hop": {"unsafe"}, "X-Test": {"yes"}}}}
	results, err := (Runner{Source: fakeSource{ex}, Client: server.Client()}).Run(context.Background(), target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Err != nil || results[0].StatusCode != http.StatusAccepted {
		t.Fatalf("results = %#v", results)
	}
	if method != "PATCH" || requestURI != "/thing?q=1" || body != "payload" {
		t.Fatalf("received %s %s %q", method, requestURI, body)
	}
	if authorization != "" || connection != "" || hop != "" {
		t.Fatalf("unsafe headers replayed: authorization=%q connection=%q nominated=%q", authorization, connection, hop)
	}
	if results[0].Duration < 0 || results[0].Duration > time.Minute {
		t.Fatalf("unexpected duration %s", results[0].Duration)
	}
	if !strings.HasPrefix(results[0].TargetURL, server.URL) {
		t.Fatalf("target URL = %q", results[0].TargetURL)
	}
}

func TestRunnerStopsAtRedirectAndDoesNotInjectCompression(t *testing.T) {
	var redirected int
	var acceptEncoding string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptEncoding = r.Header.Get("Accept-Encoding")
		if r.URL.Path == "/next" {
			redirected++
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	ex := recording.Exchange{ID: 1, Request: recording.Request{Method: http.MethodGet, URL: "/start"}}
	results, err := (Runner{Source: fakeSource{ex}}).Run(context.Background(), target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Err != nil || results[0].StatusCode != http.StatusFound {
		t.Fatalf("results = %#v", results)
	}
	if redirected != 0 {
		t.Fatalf("redirect target was requested %d times", redirected)
	}
	if acceptEncoding != "" {
		t.Fatalf("implicit Accept-Encoding = %q", acceptEncoding)
	}
}

func TestRunnerRefusesTruncatedRequestBody(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	ex := recording.Exchange{ID: 3, Request: recording.Request{Method: http.MethodPost, URL: "/", Body: []byte("part"), BodySize: 10, BodyTruncated: true}}
	results, err := (Runner{Source: fakeSource{ex}}).Run(context.Background(), target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "truncated") {
		t.Fatalf("results = %#v", results)
	}
	if called {
		t.Fatal("request with a truncated body was sent")
	}
}
