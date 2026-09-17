package capture_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opemori/graybox-core/internal/capture"
	"github.com/opemori/graybox-core/internal/replay"
	"github.com/opemori/graybox-core/internal/sanitize"
	"github.com/opemori/graybox-core/internal/storage"
)

func TestProxyRecordsAndRecordedRequestReplays(t *testing.T) {
	ctx := context.Background()
	var upstreamHost, forwardedFor, forwardedHost, forwardedProto string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHost = r.Host
		forwardedFor = r.Header.Get("X-Forwarded-For")
		forwardedHost = r.Header.Get("X-Forwarded-Host")
		forwardedProto = r.Header.Get("X-Forwarded-Proto")
		if r.URL.RequestURI() != "/checkout?attempt=2" {
			t.Errorf("upstream request URI = %q", r.URL.RequestURI())
		}
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
	proxy := httptest.NewServer(capture.NewProxy(upstreamURL, store, func(err error) { t.Errorf("proxy error: %v", err) }))
	defer proxy.Close()

	req, err := http.NewRequest(http.MethodPost, proxy.URL+"/checkout?attempt=2", bytes.NewReader([]byte{9, 8, 0, 7}))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer very-secret")
	req.Header.Set("Cookie", "session=very-secret")
	req.Host = "client.example"
	req.Header.Add("X-Multi", "one")
	req.Header.Add("X-Multi", "two")
	req.Header.Set("X-Forwarded-For", "spoofed")
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
	if forwardedFor == "" || strings.Contains(forwardedFor, "spoofed") || forwardedHost != "client.example" || forwardedProto != "http" {
		t.Fatalf("forwarded headers = for %q, host %q, proto %q", forwardedFor, forwardedHost, forwardedProto)
	}

	ex, err := store.Get(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Request.Method != http.MethodPost || ex.Request.URL != "/checkout?attempt=2" || !bytes.Equal(ex.Request.Body, []byte{9, 8, 0, 7}) {
		t.Fatalf("recorded request = %#v", ex.Request)
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

	var replayedBody []byte
	var replayedAuth string
	replayTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replayedBody, _ = io.ReadAll(r.Body)
		replayedAuth = r.Header.Get("Authorization")
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
	proxy := httptest.NewServer(capture.NewProxy(target, store, func(error) {}))
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

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	proxy = httptest.NewServer(capture.NewProxy(upstreamURL, store, nil))
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
	proxy := httptest.NewServer(capture.NewProxyWithBodyLimit(target, store, 4, nil))
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
	if string(ex.Request.Body) != "0123" || ex.Request.BodySize != 10 || !ex.Request.BodyTruncated {
		t.Fatalf("request capture = %#v", ex.Request)
	}
	if string(ex.Response.Body) != "abcd" || ex.Response.BodySize != 10 || !ex.Response.BodyTruncated {
		t.Fatalf("response capture = %#v", ex.Response)
	}
}
