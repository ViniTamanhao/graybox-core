package replay

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
	"github.com/ViniTamanhao/graybox-core/internal/sanitize"
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

	got, err = BuildURL(target, "/empty-query?")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "https://example.test/api/empty-query?" || !got.ForceQuery {
		t.Fatalf("forced-query URL = %q, ForceQuery = %v", got.String(), got.ForceQuery)
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
	ex := recording.Exchange{
		ID: 7,
		Request: recording.Request{
			Method:       "PATCH",
			URL:          "/thing?q=1",
			Body:         []byte("payload"),
			ObservedSize: 7,
			Complete:     true,
			Headers: http.Header{
				"Authorization": {sanitize.RedactedValue},
				"Connection":    {"close, X-Hop"},
				"X-Hop":         {"unsafe"},
				"X-Test":        {"yes"},
			},
		},
	}
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
	ex := recording.Exchange{
		ID: 1,
		Request: recording.Request{
			Method:   http.MethodGet,
			URL:      "/start",
			Complete: true,
		},
	}
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
	ex := recording.Exchange{
		ID: 3,
		Request: recording.Request{
			Method:       http.MethodPost,
			URL:          "/",
			Body:         []byte("part"),
			ObservedSize: 10,
			Truncated:    true,
			Complete:     true,
		},
	}
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

func TestRunnerRefusesIncompleteRequestBody(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	ex := recording.Exchange{
		ID: 4,
		Request: recording.Request{
			Method:       http.MethodPost,
			URL:          "/",
			Body:         []byte("partial"),
			ObservedSize: 7,
			Complete:     false,
		},
	}

	results, err := (Runner{Source: fakeSource{ex}}).Run(context.Background(), target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Err == nil ||
		results[0].Err.Error() != "cannot replay exchange: request body was incomplete" {
		t.Fatalf("results = %#v", results)
	}
	if called {
		t.Fatal("request with an incomplete body was sent")
	}
}

func TestRunnerRunEachCapturesResponse(
	t *testing.T,
) {
	const responseBody = "response-body"

	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				_ *http.Request,
			) {
				w.Header().Set(
					"Content-Type",
					"text/plain",
				)
				w.Header().Set(
					"X-Test",
					"yes",
				)
				w.Header().Set(
					"Set-Cookie",
					"session=secret",
				)

				w.WriteHeader(
					http.StatusAccepted,
				)

				_, _ = io.WriteString(
					w,
					responseBody,
				)
			},
		),
	)
	defer server.Close()

	target, err := url.Parse(
		server.URL,
	)
	if err != nil {
		t.Fatal(err)
	}

	exchange := recording.Exchange{
		ID: 7,
		Request: recording.Request{
			Method:   http.MethodGet,
			URL:      "/resource",
			Complete: true,
		},
	}

	var got Execution

	err = (Runner{
		Source: fakeSource{
			exchange: exchange,
		},
		Client: server.Client(),
	}).RunEach(
		context.Background(),
		target,
		nil,
		func(
			baseline recording.Exchange,
			execution Execution,
		) error {
			if baseline.ID != exchange.ID {
				t.Fatalf(
					"baseline ID = %d, want %d",
					baseline.ID,
					exchange.ID,
				)
			}

			got = execution
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.Err != nil {
		t.Fatalf(
			"execution error = %v",
			got.Err,
		)
	}

	if got.Response == nil {
		t.Fatal(
			"execution response = nil",
		)
	}

	if got.Response.StatusCode != http.StatusAccepted {
		t.Fatalf(
			"status = %d, want %d",
			got.Response.StatusCode,
			http.StatusAccepted,
		)
	}

	if got.Status != "202 Accepted" {
		t.Fatalf(
			"status text = %q, want %q",
			got.Status,
			"202 Accepted",
		)
	}

	if string(got.Response.Body) != responseBody {
		t.Fatalf(
			"body = %q, want %q",
			got.Response.Body,
			responseBody,
		)
	}

	if got.Response.ObservedSize != int64(
		len(responseBody),
	) {
		t.Fatalf(
			"observed size = %d, want %d",
			got.Response.ObservedSize,
			len(responseBody),
		)
	}

	if got.Response.Truncated {
		t.Fatal(
			"response was unexpectedly truncated",
		)
	}

	if !got.Response.Complete {
		t.Fatal(
			"response was unexpectedly incomplete",
		)
	}

	if got.Response.Headers.Get(
		"X-Test",
	) != "yes" {
		t.Fatalf(
			"X-Test = %q, want yes",
			got.Response.Headers.Get("X-Test"),
		)
	}

	if got.Response.Headers.Get(
		"Set-Cookie",
	) != sanitize.RedactedValue {
		t.Fatalf(
			"Set-Cookie = %q, want redacted",
			got.Response.Headers.Get("Set-Cookie"),
		)
	}
}

func TestRunnerRunEachBoundsResponseBodyCapture(
	t *testing.T,
) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				_ *http.Request,
			) {
				_, _ = io.WriteString(
					w,
					"abcdefghij",
				)
			},
		),
	)
	defer server.Close()

	target, err := url.Parse(
		server.URL,
	)
	if err != nil {
		t.Fatal(err)
	}

	exchange := recording.Exchange{
		ID: 8,
		Request: recording.Request{
			Method:   http.MethodGet,
			URL:      "/large",
			Complete: true,
		},
	}

	var got Execution

	err = (Runner{
		Source: fakeSource{
			exchange: exchange,
		},
		Client:            server.Client(),
		ResponseBodyLimit: 4,
	}).RunEach(
		context.Background(),
		target,
		nil,
		func(
			_ recording.Exchange,
			execution Execution,
		) error {
			got = execution
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.Err != nil {
		t.Fatalf(
			"execution error = %v",
			got.Err,
		)
	}

	if got.Response == nil {
		t.Fatal(
			"execution response = nil",
		)
	}

	if string(got.Response.Body) != "abcd" {
		t.Fatalf(
			"body = %q, want %q",
			got.Response.Body,
			"abcd",
		)
	}

	if got.Response.ObservedSize != 10 {
		t.Fatalf(
			"observed size = %d, want 10",
			got.Response.ObservedSize,
		)
	}

	if !got.Response.Truncated {
		t.Fatal(
			"response was not marked truncated",
		)
	}

	if !got.Response.Complete {
		t.Fatal(
			"truncated but normally completed response was marked incomplete",
		)
	}
}

type roundTripFunc func(
	*http.Request,
) (*http.Response, error)

func (f roundTripFunc) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	return f(request)
}

type failingReadCloser struct {
	done bool
}

func (r *failingReadCloser) Read(
	buffer []byte,
) (int, error) {
	if r.done {
		return 0, io.EOF
	}

	r.done = true

	n := copy(
		buffer,
		[]byte("part"),
	)

	return n, errors.New(
		"simulated response read failure",
	)
}

func (*failingReadCloser) Close() error {
	return nil
}

func TestRunnerRunEachPreservesPartialResponseOnReadFailure(
	t *testing.T,
) {
	client := &http.Client{
		Transport: roundTripFunc(
			func(
				*http.Request,
			) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header: http.Header{
						"X-Test": {"yes"},
					},
					Body: &failingReadCloser{},
				}, nil
			},
		),
	}

	target, err := url.Parse(
		"http://example.test",
	)
	if err != nil {
		t.Fatal(err)
	}

	exchange := recording.Exchange{
		ID: 9,
		Request: recording.Request{
			Method:   http.MethodGet,
			URL:      "/broken",
			Complete: true,
		},
	}

	var got Execution

	err = (Runner{
		Source: fakeSource{
			exchange: exchange,
		},
		Client: client,
	}).RunEach(
		context.Background(),
		target,
		nil,
		func(
			_ recording.Exchange,
			execution Execution,
		) error {
			got = execution
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.Err == nil ||
		!strings.Contains(
			got.Err.Error(),
			"read replay response",
		) {
		t.Fatalf(
			"execution error = %v, want response read failure",
			got.Err,
		)
	}

	if got.Response == nil {
		t.Fatal(
			"partial HTTP response was discarded",
		)
	}

	if got.Response.StatusCode != http.StatusOK {
		t.Fatalf(
			"status = %d, want %d",
			got.Response.StatusCode,
			http.StatusOK,
		)
	}

	if string(got.Response.Body) != "part" {
		t.Fatalf(
			"body = %q, want %q",
			got.Response.Body,
			"part",
		)
	}

	if got.Response.ObservedSize != 4 {
		t.Fatalf(
			"observed size = %d, want 4",
			got.Response.ObservedSize,
		)
	}

	if got.Response.Complete {
		t.Fatal(
			"failed response stream was marked complete",
		)
	}

	if got.Response.Truncated {
		t.Fatal(
			"fully retained partial stream was incorrectly marked truncated",
		)
	}
}
