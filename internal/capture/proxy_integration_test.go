package capture_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ViniTamanhao/graybox-core/internal/capture"
	"github.com/ViniTamanhao/graybox-core/internal/recording"
	"github.com/ViniTamanhao/graybox-core/internal/replay"
	"github.com/ViniTamanhao/graybox-core/internal/sanitize"
	"github.com/ViniTamanhao/graybox-core/internal/storage"
)

func TestProxyRecordsAndRecordedRequestReplays(t *testing.T) {
	ctx := context.Background()
	var upstreamHost, upstreamURI, forwardedFor, forwardedHost, forwardedProto, removedHop string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHost = r.Host
		upstreamURI = r.URL.RequestURI()
		forwardedFor = r.Header.Get("X-Forwarded-For")
		forwardedHost = r.Header.Get("X-Forwarded-Host")
		forwardedProto = r.Header.Get("X-Forwarded-Proto")
		removedHop = r.Header.Get("X-Remove-Me")
		w.Header().Add("X-Repeated", "first")
		w.Header().Add("X-Repeated", "second")
		w.Header().Set("Set-Cookie", "secret=one")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte{0, 1, 255})
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)

	path := filepath.Join(t.TempDir(), "integration.graybox")
	store, err := storage.Create(ctx, path, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	failTest := func(err error) { t.Errorf("proxy error: %v", err) }
	proxy := httptest.NewServer(capture.NewProxy(upstreamURL, store, capture.ErrorHandlers{Transport: failTest, Persistence: failTest}))
	defer proxy.Close()

	const requestURI = "/checkout/a%2Fb?attempt=2;mode=raw&encoded=%2f%2F&attempt=3"
	req, err := http.NewRequest(
		http.MethodPost,
		proxy.URL+requestURI,
		bytes.NewReader([]byte{9, 8, 0, 7}),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer very-secret")
	req.Header.Set("Cookie", "session=very-secret")
	req.Host = "client.example"
	req.Header.Add("X-Multi", "one")
	req.Header.Add("X-Multi", "two")
	req.Header.Set("X-Forwarded-For", "spoofed")
	req.Header.Set("X-Forwarded-Host", "spoofed.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Connection", "X-Remove-Me")
	req.Header.Set("X-Remove-Me", "spoofed-hop")
	resp, err := proxy.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || !bytes.Equal(responseBody, []byte{0, 1, 255}) {
		t.Fatalf("proxy response = %d %v", resp.StatusCode, responseBody)
	}
	if upstreamHost != upstreamURL.Host {
		t.Fatalf("upstream Host = %q, want %q", upstreamHost, upstreamURL.Host)
	}
	if upstreamURI != requestURI {
		t.Fatalf("upstream request URI = %q", upstreamURI)
	}
	if forwardedFor == "" || strings.Contains(forwardedFor, "spoofed") || forwardedHost != "client.example" || forwardedProto != "http" {
		t.Fatalf("forwarded headers = for %q, host %q, proto %q", forwardedFor, forwardedHost, forwardedProto)
	}
	if removedHop != "" {
		t.Fatalf("upstream received connection-nominated header %q", removedHop)
	}

	ex, err := store.Get(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Request.Method != http.MethodPost || ex.Request.URL != requestURI ||
		!bytes.Equal(ex.Request.Body, []byte{9, 8, 0, 7}) {
		t.Fatalf("recorded request = %#v", ex.Request)
	}
	if ex.Request.ObservedSize != 4 || ex.Request.Truncated || !ex.Request.Complete {
		t.Fatalf("recorded request body state = %#v", ex.Request)
	}
	if got := ex.Request.Headers.Get("X-Forwarded-For"); got != forwardedFor || strings.Contains(got, "spoofed") {
		t.Fatalf("recorded X-Forwarded-For = %q, upstream received %q", got, forwardedFor)
	}
	if ex.Request.Headers.Get("X-Forwarded-Host") != forwardedHost || ex.Request.Headers.Get("X-Forwarded-Proto") != forwardedProto {
		t.Fatalf("recorded forwarding headers = %#v", ex.Request.Headers)
	}
	if ex.Request.Headers.Get("Connection") != "" || ex.Request.Headers.Get("X-Remove-Me") != "" {
		t.Fatalf("recording retained hop-by-hop headers: %#v", ex.Request.Headers)
	}
	if ex.Request.Headers.Get("Authorization") != sanitize.RedactedValue || ex.Request.Headers.Get("Cookie") != sanitize.RedactedValue {
		t.Fatalf("credentials not redacted: %#v", ex.Request.Headers)
	}
	if len(ex.Request.Headers.Values("X-Multi")) != 2 || len(ex.Response.Headers.Values("X-Repeated")) != 2 {
		t.Fatalf("repeated headers not preserved: request=%#v response=%#v", ex.Request.Headers, ex.Response.Headers)
	}
	if ex.Response.Headers.Get("Set-Cookie") != sanitize.RedactedValue || !bytes.Equal(ex.Response.Body, responseBody) {
		t.Fatalf("recorded response = %#v", ex.Response)
	}
	if ex.Response.ObservedSize != 3 || ex.Response.Truncated || !ex.Response.Complete {
		t.Fatalf("recorded response body state = %#v", ex.Response)
	}

	var replayedBody []byte
	var replayedAuth, replayedForwardedFor, replayedURI string
	replayTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replayedBody, _ = io.ReadAll(r.Body)
		replayedAuth = r.Header.Get("Authorization")
		replayedForwardedFor = r.Header.Get("X-Forwarded-For")
		replayedURI = r.URL.RequestURI()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer replayTarget.Close()
	replayURL, _ := url.Parse(replayTarget.URL)
	id := int64(1)
	results, err := (replay.Runner{Source: store, Client: replayTarget.Client()}).Run(ctx, replayURL, &id)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Err != nil || results[0].StatusCode != http.StatusNoContent {
		t.Fatalf("replay results = %#v", results)
	}
	if !bytes.Equal(replayedBody, ex.Request.Body) || replayedAuth != "" {
		t.Fatalf("replayed body/auth = %v/%q", replayedBody, replayedAuth)
	}
	if replayedForwardedFor != forwardedFor || strings.Contains(replayedForwardedFor, "spoofed") {
		t.Fatalf("replayed X-Forwarded-For = %q, recorded %q", replayedForwardedFor, forwardedFor)
	}
	if replayedURI != requestURI {
		t.Fatalf("replayed request URI = %q", replayedURI)
	}
}

func TestProxyPersistsTransportFailureDistinctFromUpstream502(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "errors.graybox")
	store, err := storage.Create(ctx, path, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	target, _ := url.Parse(dead.URL)
	dead.Close()
	proxyHandler := capture.NewProxy(target, store, capture.ErrorHandlers{Transport: func(error) {}})
	proxy := httptest.NewServer(proxyHandler)
	resp, err := proxy.Client().Get(proxy.URL + "/unavailable")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	proxy.Close()

	ex, err := store.Get(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Response.StatusCode != http.StatusBadGateway || ex.ProxyError == "" {
		t.Fatalf("transport failure = status %d, proxy error %q", ex.Response.StatusCode, ex.ProxyError)
	}
	if !ex.Request.Complete || !ex.Response.Complete {
		t.Fatalf("transport failure body state = request %#v, response %#v", ex.Request, ex.Response)
	}
	if proxyHandler.PersistenceFailures() != 0 {
		t.Fatalf("transport failure counted as %d persistence failures", proxyHandler.PersistenceFailures())
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	proxy = httptest.NewServer(capture.NewProxy(upstreamURL, store, capture.ErrorHandlers{}))
	defer proxy.Close()
	resp, err = proxy.Client().Get(proxy.URL + "/real-502")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	ex, err = store.Get(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Response.StatusCode != http.StatusBadGateway || ex.ProxyError != "" {
		t.Fatalf("upstream 502 = status %d, proxy error %q", ex.Response.StatusCode, ex.ProxyError)
	}
	if !ex.Request.Complete || !ex.Response.Complete {
		t.Fatalf("upstream 502 body state = request %#v, response %#v", ex.Request, ex.Response)
	}
}

func TestProxyDoesNotPersistInboundHeadersWhenNoRequestReachedTransport(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Create(ctx, filepath.Join(t.TempDir(), "rejected.graybox"), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	target, _ := url.Parse("http://unused.invalid")
	var transportErrors atomic.Uint64
	handler := capture.NewProxy(target, store, capture.ErrorHandlers{
		Transport: func(error) { transportErrors.Add(1) },
	})
	request := httptest.NewRequest(http.MethodGet, "http://proxy.invalid/rejected", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header["Upgrade"] = []string{"invalid\x01upgrade"}
	request.Header.Set("X-Forwarded-For", "spoofed")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || transportErrors.Load() != 1 {
		t.Fatalf("response/errors = %d/%d", response.Code, transportErrors.Load())
	}
	exchange, err := store.Get(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if exchange.ProxyError == "" || len(exchange.Request.Headers) != 0 {
		t.Fatalf("proxy error/recorded headers = %q/%#v", exchange.ProxyError, exchange.Request.Headers)
	}
	if handler.PersistenceFailures() != 0 {
		t.Fatalf("rejected request counted as persistence failure")
	}
}

func TestProxyStreamsBodiesAndPersistsTruncationMetadata(t *testing.T) {
	ctx := context.Background()
	requestData := []byte("0123456789")
	responseData := []byte("abcdefghij")
	var received []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		_, _ = w.Write(responseData)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	store, err := storage.Create(ctx, filepath.Join(t.TempDir(), "limited.graybox"), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	proxy := httptest.NewServer(capture.NewProxyWithBodyLimit(target, store, 4, capture.ErrorHandlers{}))
	defer proxy.Close()
	resp, err := proxy.Client().Post(proxy.URL+"/large", "application/octet-stream", bytes.NewReader(requestData))
	if err != nil {
		t.Fatal(err)
	}
	gotResponse, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !bytes.Equal(received, requestData) || !bytes.Equal(gotResponse, responseData) {
		t.Fatalf("proxy truncated traffic: request %q response %q", received, gotResponse)
	}
	ex, err := store.Get(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(ex.Request.Body) != "0123" || ex.Request.ObservedSize != 10 ||
		!ex.Request.Truncated || !ex.Request.Complete {
		t.Fatalf("request capture = %#v", ex.Request)
	}
	if string(ex.Response.Body) != "abcd" || ex.Response.ObservedSize != 10 ||
		!ex.Response.Truncated || !ex.Response.Complete {
		t.Fatalf("response capture = %#v", ex.Response)
	}
}

type interruptedReader struct {
	data []byte
}

func (r *interruptedReader) Read(buffer []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, errors.New("request stream interrupted")
	}
	n := copy(buffer, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestProxyMarksInterruptedRequestBodyIncomplete(t *testing.T) {
	ctx := context.Background()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	store, err := storage.Create(ctx, filepath.Join(t.TempDir(), "incomplete-request.graybox"), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := capture.NewProxy(target, store, capture.ErrorHandlers{Transport: func(error) {}})

	request := httptest.NewRequest(
		http.MethodPost,
		"http://proxy.invalid/incomplete",
		&interruptedReader{data: []byte("partial")},
	)
	request.ContentLength = 10
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	exchange, err := store.Get(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(exchange.Request.Body) != "partial" || exchange.Request.ObservedSize != 7 ||
		exchange.Request.Truncated || exchange.Request.Complete {
		t.Fatalf("request body state = %#v", exchange.Request)
	}
}

func TestProxyPersistsAbortedResponse(t *testing.T) {
	const partialBody = "partial-response"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		connection, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream response: %v", err)
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprintf(
			buffered,
			"HTTP/1.1 200 OK\r\nContent-Length: 64\r\nContent-Type: text/plain\r\n\r\n%s",
			partialBody,
		)
		_ = buffered.Flush()
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)

	for _, test := range []struct {
		name          string
		limit         int64
		wantBody      string
		wantTruncated bool
	}{
		{name: "retains all observed bytes", limit: 64, wantBody: partialBody},
		{name: "capture limit remains independent", limit: 4, wantBody: "part", wantTruncated: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store, err := storage.Create(
				ctx,
				filepath.Join(t.TempDir(), "aborted-response.graybox"),
				"test",
			)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			var transportErrors atomic.Uint64
			handler := capture.NewProxyWithBodyLimit(
				target,
				store,
				test.limit,
				capture.ErrorHandlers{Transport: func(error) { transportErrors.Add(1) }},
			)
			proxy := httptest.NewServer(handler)

			response, requestErr := proxy.Client().Get(proxy.URL + "/aborted")
			if response != nil {
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
			}
			proxy.Close()
			if requestErr == nil && response == nil {
				t.Fatal("aborted response returned neither a response nor an error")
			}

			exchange, err := store.Get(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			if string(exchange.Response.Body) != test.wantBody ||
				exchange.Response.ObservedSize != int64(len(partialBody)) ||
				exchange.Response.Truncated != test.wantTruncated || exchange.Response.Complete {
				t.Fatalf("aborted response body state = %#v", exchange.Response)
			}
			if exchange.ProxyError == "" {
				t.Fatal("aborted response has no proxy error")
			}
			if handler.PersistenceFailures() != 0 {
				t.Fatalf("aborted response counted as a persistence failure")
			}
			if transportErrors.Load() != 1 {
				t.Fatalf("transport error callbacks = %d, want 1", transportErrors.Load())
			}
		})
	}
}

type failingRecorder struct{ calls atomic.Uint64 }

func (r *failingRecorder) Add(context.Context, recording.Exchange) (int64, error) {
	r.calls.Add(1)
	return 0, errors.New("disk full")
}

func TestProxyContinuesTrafficAndTracksPersistenceFailures(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	recorder := new(failingRecorder)
	var transportErrors, persistenceErrors atomic.Uint64
	handler := capture.NewProxy(target, recorder, capture.ErrorHandlers{
		Transport:   func(error) { transportErrors.Add(1) },
		Persistence: func(error) { persistenceErrors.Add(1) },
	})
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	resp, err := proxy.Client().Get(proxy.URL + "/still-proxied")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("response status = %d", resp.StatusCode)
	}
	if recorder.calls.Load() != 1 || handler.PersistenceFailures() != 1 || persistenceErrors.Load() != 1 {
		t.Fatalf("calls/failures/callbacks = %d/%d/%d", recorder.calls.Load(), handler.PersistenceFailures(), persistenceErrors.Load())
	}
	if transportErrors.Load() != 0 {
		t.Fatalf("persistence failure reported as %d transport errors", transportErrors.Load())
	}
}
