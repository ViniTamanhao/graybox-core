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

type fakeSource struct {
	exchange recording.Exchange
}

func (f fakeSource) Get(
	context.Context,
	int64,
) (recording.Exchange, error) {
	return f.exchange, nil
}

func (f fakeSource) List(
	context.Context,
	recording.Filter,
) ([]recording.Summary, error) {
	return []recording.Summary{
		{
			ID: f.exchange.ID,
		},
	}, nil
}

func TestBuildURL(t *testing.T) {
	target, _ := url.Parse(
		"https://example.test/api/",
	)

	got, err := BuildURL(
		target,
		"/users/a%2Fb?q=hello%20world",
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "https://example.test/api/users/a%2Fb?q=hello%20world" {
		t.Fatalf(
			"URL = %q",
			got.String(),
		)
	}

	got, err = BuildURL(
		target,
		"/empty-query?",
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "https://example.test/api/empty-query?" ||
		!got.ForceQuery {
		t.Fatalf(
			"forced-query URL = %q, ForceQuery = %v",
			got.String(),
			got.ForceQuery,
		)
	}
}

func TestRunnerPreservesRequestAndOmitsUnsafeHeaders(
	t *testing.T,
) {
	var method string
	var requestURI string
	var body string
	var authorization string
	var connection string
	var hop string

	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				method = r.Method
				requestURI = r.URL.RequestURI()

				data, _ := io.ReadAll(
					r.Body,
				)
				body = string(data)

				authorization = r.Header.Get(
					"Authorization",
				)
				connection = r.Header.Get(
					"Connection",
				)
				hop = r.Header.Get(
					"X-Hop",
				)

				w.WriteHeader(
					http.StatusAccepted,
				)
			},
		),
	)
	defer server.Close()

	target, _ := url.Parse(
		server.URL,
	)

	exchange := recording.Exchange{
		ID: 7,
		Request: recording.Request{
			Method:       "PATCH",
			URL:          "/thing?q=1",
			Body:         []byte("payload"),
			ObservedSize: 7,
			Complete:     true,
			Headers: http.Header{
				"Authorization": {
					sanitize.RedactedValue,
				},
				"Connection": {
					"close, X-Hop",
				},
				"X-Hop": {
					"unsafe",
				},
				"X-Test": {
					"yes",
				},
			},
		},
	}

	results, err := (Runner{
		Source: fakeSource{
			exchange: exchange,
		},
		Client: server.Client(),
	}).Run(
		context.Background(),
		target,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 ||
		results[0].Err != nil ||
		results[0].StatusCode != http.StatusAccepted {
		t.Fatalf(
			"results = %#v",
			results,
		)
	}

	if method != "PATCH" ||
		requestURI != "/thing?q=1" ||
		body != "payload" {
		t.Fatalf(
			"received %s %s %q",
			method,
			requestURI,
			body,
		)
	}

	if authorization != "" ||
		connection != "" ||
		hop != "" {
		t.Fatalf(
			"unsafe headers replayed: authorization=%q connection=%q nominated=%q",
			authorization,
			connection,
			hop,
		)
	}

	if results[0].Duration < 0 ||
		results[0].Duration > time.Minute {
		t.Fatalf(
			"unexpected duration %s",
			results[0].Duration,
		)
	}

	if !strings.HasPrefix(
		results[0].TargetURL,
		server.URL,
	) {
		t.Fatalf(
			"target URL = %q",
			results[0].TargetURL,
		)
	}
}

func TestRunnerAppliesRequestHeaderOverrides(
	t *testing.T,
) {
	var receivedHeaders http.Header

	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				receivedHeaders = r.Header.Clone()

				w.WriteHeader(
					http.StatusNoContent,
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

	recordedHeaders := make(http.Header)

	recordedHeaders.Set(
		"Authorization",
		sanitize.RedactedValue,
	)
	recordedHeaders.Set(
		"X-API-Key",
		"recorded-api-key",
	)
	recordedHeaders.Set(
		"X-Recorded",
		"recorded-value",
	)

	exchange := recording.Exchange{
		ID: 12,
		Request: recording.Request{
			Method:   http.MethodGet,
			URL:      "/authenticated",
			Complete: true,
			Headers:  recordedHeaders,
		},
	}

	overrides := make(http.Header)

	overrides.Set(
		"Authorization",
		"Bearer runtime-token",
	)
	overrides.Set(
		"X-API-Key",
		"runtime-api-key",
	)
	overrides.Set(
		"X-Recorded",
		"runtime-value",
	)

	overrides["X-Multi"] = []string{
		"one",
		"two",
	}

	results, err := (Runner{
		Source: fakeSource{
			exchange: exchange,
		},
		Client:                 server.Client(),
		RequestHeaderOverrides: overrides,
	}).Run(
		context.Background(),
		target,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf(
			"len(results) = %d, want 1",
			len(results),
		)
	}

	if results[0].Err != nil {
		t.Fatalf(
			"replay error = %v",
			results[0].Err,
		)
	}

	if results[0].StatusCode != http.StatusNoContent {
		t.Fatalf(
			"status = %d, want %d",
			results[0].StatusCode,
			http.StatusNoContent,
		)
	}

	if got := receivedHeaders.Get(
		"Authorization",
	); got != "Bearer runtime-token" {
		t.Fatalf(
			"Authorization = %q, want runtime value",
			got,
		)
	}

	if got := receivedHeaders.Get(
		"X-API-Key",
	); got != "runtime-api-key" {
		t.Fatalf(
			"X-API-Key = %q, want runtime value",
			got,
		)
	}

	if got := receivedHeaders.Get(
		"X-Recorded",
	); got != "runtime-value" {
		t.Fatalf(
			"X-Recorded = %q, want runtime value",
			got,
		)
	}

	gotMulti := receivedHeaders.Values(
		"X-Multi",
	)

	if len(gotMulti) != 2 ||
		gotMulti[0] != "one" ||
		gotMulti[1] != "two" {
		t.Fatalf(
			"X-Multi = %#v, want [one two]",
			gotMulti,
		)
	}

	if got := exchange.Request.Headers.Get(
		"Authorization",
	); got != sanitize.RedactedValue {
		t.Fatalf(
			"recorded Authorization mutated to %q",
			got,
		)
	}

	if got := exchange.Request.Headers.Get(
		"X-API-Key",
	); got != "recorded-api-key" {
		t.Fatalf(
			"recorded X-API-Key mutated to %q",
			got,
		)
	}
}

func TestRunnerRequestHeaderOverridesPreserveUnmodifiedRecordedHeaders(
	t *testing.T,
) {
	var receivedHeaders http.Header

	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				receivedHeaders = r.Header.Clone()

				w.WriteHeader(
					http.StatusOK,
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

	recordedHeaders := make(http.Header)

	recordedHeaders.Set(
		"Accept",
		"application/json",
	)
	recordedHeaders.Set(
		"X-Recorded",
		"keep-me",
	)
	recordedHeaders.Set(
		"Authorization",
		sanitize.RedactedValue,
	)

	exchange := recording.Exchange{
		ID: 13,
		Request: recording.Request{
			Method:   http.MethodGet,
			URL:      "/resource",
			Complete: true,
			Headers:  recordedHeaders,
		},
	}

	overrides := make(http.Header)

	overrides.Set(
		"Authorization",
		"Bearer runtime-token",
	)

	results, err := (Runner{
		Source: fakeSource{
			exchange: exchange,
		},
		Client:                 server.Client(),
		RequestHeaderOverrides: overrides,
	}).Run(
		context.Background(),
		target,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 ||
		results[0].Err != nil {
		t.Fatalf(
			"results = %#v",
			results,
		)
	}

	if got := receivedHeaders.Get(
		"Authorization",
	); got != "Bearer runtime-token" {
		t.Fatalf(
			"Authorization = %q, want runtime value",
			got,
		)
	}

	if got := receivedHeaders.Get(
		"Accept",
	); got != "application/json" {
		t.Fatalf(
			"Accept = %q, want recorded value",
			got,
		)
	}

	if got := receivedHeaders.Get(
		"X-Recorded",
	); got != "keep-me" {
		t.Fatalf(
			"X-Recorded = %q, want recorded value",
			got,
		)
	}
}

func TestRunnerStopsAtRedirectAndDoesNotInjectCompression(
	t *testing.T,
) {
	var redirected int
	var acceptEncoding string

	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				acceptEncoding = r.Header.Get(
					"Accept-Encoding",
				)

				if r.URL.Path == "/next" {
					redirected++
					w.WriteHeader(
						http.StatusOK,
					)
					return
				}

				http.Redirect(
					w,
					r,
					"/next",
					http.StatusFound,
				)
			},
		),
	)
	defer server.Close()

	target, _ := url.Parse(
		server.URL,
	)

	exchange := recording.Exchange{
		ID: 1,
		Request: recording.Request{
			Method:   http.MethodGet,
			URL:      "/start",
			Complete: true,
		},
	}

	results, err := (Runner{
		Source: fakeSource{
			exchange: exchange,
		},
	}).Run(
		context.Background(),
		target,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 ||
		results[0].Err != nil ||
		results[0].StatusCode != http.StatusFound {
		t.Fatalf(
			"results = %#v",
			results,
		)
	}

	if redirected != 0 {
		t.Fatalf(
			"redirect target was requested %d times",
			redirected,
		)
	}

	if acceptEncoding != "" {
		t.Fatalf(
			"implicit Accept-Encoding = %q",
			acceptEncoding,
		)
	}
}

func TestRunnerRefusesTruncatedRequestBody(
	t *testing.T,
) {
	called := false

	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				http.ResponseWriter,
				*http.Request,
			) {
				called = true
			},
		),
	)
	defer server.Close()

	target, _ := url.Parse(
		server.URL,
	)

	exchange := recording.Exchange{
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

	results, err := (Runner{
		Source: fakeSource{
			exchange: exchange,
		},
	}).Run(
		context.Background(),
		target,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 ||
		results[0].Err == nil ||
		!strings.Contains(
			results[0].Err.Error(),
			"truncated",
		) {
		t.Fatalf(
			"results = %#v",
			results,
		)
	}

	if called {
		t.Fatal(
			"request with a truncated body was sent",
		)
	}
}

func TestRunnerRefusesIncompleteRequestBody(
	t *testing.T,
) {
	called := false

	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				http.ResponseWriter,
				*http.Request,
			) {
				called = true
			},
		),
	)
	defer server.Close()

	target, _ := url.Parse(
		server.URL,
	)

	exchange := recording.Exchange{
		ID: 4,
		Request: recording.Request{
			Method:       http.MethodPost,
			URL:          "/",
			Body:         []byte("partial"),
			ObservedSize: 7,
			Complete:     false,
		},
	}

	results, err := (Runner{
		Source: fakeSource{
			exchange: exchange,
		},
	}).Run(
		context.Background(),
		target,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 ||
		results[0].Err == nil ||
		results[0].Err.Error() !=
			"cannot replay exchange: request body was incomplete" {
		t.Fatalf(
			"results = %#v",
			results,
		)
	}

	if called {
		t.Fatal(
			"request with an incomplete body was sent",
		)
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
	client := failingResponseClient()

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

func TestRunnerCompactResultDoesNotExposePartialResponseStatus(
	t *testing.T,
) {
	client := failingResponseClient()

	target, err := url.Parse(
		"http://example.test",
	)
	if err != nil {
		t.Fatal(err)
	}

	exchange := recording.Exchange{
		ID: 10,
		Request: recording.Request{
			Method:   http.MethodGet,
			URL:      "/broken",
			Complete: true,
		},
	}

	results, err := (Runner{
		Source: fakeSource{
			exchange: exchange,
		},
		Client: client,
	}).Run(
		context.Background(),
		target,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf(
			"len(results) = %d, want 1",
			len(results),
		)
	}

	result := results[0]

	if result.Err == nil {
		t.Fatal(
			"result error = nil, want response read failure",
		)
	}

	if result.StatusCode != 0 {
		t.Fatalf(
			"StatusCode = %d, want 0",
			result.StatusCode,
		)
	}

	if result.Status != "" {
		t.Fatalf(
			"Status = %q, want empty",
			result.Status,
		)
	}
}

func TestCompactReplayDoesNotRetainResponseBody(
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
					"response body that compact replay does not need",
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
		ID: 11,
		Request: recording.Request{
			Method:   http.MethodGet,
			URL:      "/resource",
			Complete: true,
		},
	}

	execution := execute(
		context.Background(),
		server.Client(),
		target,
		exchange,
		nil,
		DefaultResponseBodyCaptureLimit,
		false,
	)

	if execution.Err != nil {
		t.Fatalf(
			"execution error = %v",
			execution.Err,
		)
	}

	if execution.Response == nil {
		t.Fatal(
			"execution response = nil",
		)
	}

	if execution.Response.StatusCode != http.StatusOK {
		t.Fatalf(
			"StatusCode = %d, want %d",
			execution.Response.StatusCode,
			http.StatusOK,
		)
	}

	if len(execution.Response.Body) != 0 {
		t.Fatalf(
			"retained body length = %d, want 0",
			len(execution.Response.Body),
		)
	}

	if !execution.Response.Complete {
		t.Fatal(
			"successfully drained response was not marked complete",
		)
	}
}

func failingResponseClient() *http.Client {
	return &http.Client{
		Transport: roundTripFunc(
			func(
				*http.Request,
			) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header: http.Header{
						"X-Test": {
							"yes",
						},
					},
					Body: &failingReadCloser{},
				}, nil
			},
		),
	}
}
