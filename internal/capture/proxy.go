// Package capture implements Graybox's explicit HTTP reverse proxy.
package capture

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/opemori/graybox-core/internal/recording"
	"github.com/opemori/graybox-core/internal/sanitize"
)

// DefaultBodyCaptureLimit bounds the bytes retained for each request and
// response body. Traffic continues to stream after this limit is reached.
const DefaultBodyCaptureLimit int64 = 10 << 20

// Recorder persists a completed observed exchange.
type Recorder interface {
	Add(context.Context, recording.Exchange) (int64, error)
}

// Proxy is an HTTP handler that forwards and records traffic.
type Proxy struct {
	proxy     *httputil.ReverseProxy
	recorder  Recorder
	onError   func(error)
	bodyLimit int64
}

// NewProxy builds a recording reverse proxy with the default body limit.
func NewProxy(target *url.URL, recorder Recorder, onError func(error)) *Proxy {
	return NewProxyWithBodyLimit(target, recorder, DefaultBodyCaptureLimit, onError)
}

// NewProxyWithBodyLimit builds a recording reverse proxy with a bounded body
// capture. The complete request and response still stream through the proxy.
func NewProxyWithBodyLimit(target *url.URL, recorder Recorder, bodyLimit int64, onError func(error)) *Proxy {
	reverse := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			request.Out.Host = target.Host
			// Rewrite removes client-supplied forwarding headers. Rebuild them
			// explicitly from the connection Graybox actually received.
			request.SetXForwarded()
		},
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Do not inject Accept-Encoding or transparently decompress: the explicit
	// proxy should forward the representation the client asked to receive.
	transport.DisableCompression = true
	reverse.Transport = transport
	reverse.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		if captured, ok := w.(*captureWriter); ok {
			captured.proxyError = err.Error()
		}
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		if onError != nil {
			onError(fmt.Errorf("forward request: %w", err))
		}
	}
	return &Proxy{proxy: reverse, recorder: recorder, onError: onError, bodyLimit: bodyLimit}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	requestBody := newCaptureReadCloser(r.Body, p.bodyLimit)
	r.Body = requestBody

	captured := &captureWriter{ResponseWriter: w, status: http.StatusOK, limit: p.bodyLimit}
	p.proxy.ServeHTTP(captured, r)
	ended := time.Now()
	requestSize := requestBody.size
	requestTruncated := requestBody.truncated()
	if r.ContentLength > requestSize {
		requestSize = r.ContentLength
		requestTruncated = true
	}
	exchange := recording.Exchange{
		Protocol:   "http",
		StartedAt:  started.UTC(),
		EndedAt:    ended.UTC(),
		Duration:   ended.Sub(started),
		ProxyError: captured.proxyError,
		Request: recording.Request{
			Method:        r.Method,
			URL:           r.URL.RequestURI(),
			Headers:       sanitize.Headers(r.Header),
			Body:          requestBody.bytes(),
			BodySize:      requestSize,
			BodyTruncated: requestTruncated,
		},
		Response: recording.Response{
			StatusCode:    captured.status,
			Headers:       sanitize.Headers(captured.Header()),
			Body:          append([]byte(nil), captured.body.Bytes()...),
			BodySize:      captured.size,
			BodyTruncated: captured.truncated(),
		},
	}
	if _, err := p.recorder.Add(context.WithoutCancel(r.Context()), exchange); err != nil {
		p.report(fmt.Errorf("record exchange: %w", err))
	}
}

func (p *Proxy) report(err error) {
	if p.onError != nil {
		p.onError(err)
	}
}

type captureReadCloser struct {
	reader io.ReadCloser
	limit  int64
	size   int64
	body   bytes.Buffer
}

func newCaptureReadCloser(reader io.ReadCloser, limit int64) *captureReadCloser {
	return &captureReadCloser{reader: reader, limit: limit}
}

func (r *captureReadCloser) Read(data []byte) (int, error) {
	if r.reader == nil {
		return 0, io.EOF
	}
	n, err := r.reader.Read(data)
	if n > 0 {
		r.size += int64(n)
		r.capture(data[:n])
	}
	return n, err
}

func (r *captureReadCloser) capture(data []byte) {
	remaining := r.limit - int64(r.body.Len())
	if remaining <= 0 {
		return
	}
	if int64(len(data)) > remaining {
		data = data[:remaining]
	}
	_, _ = r.body.Write(data)
}

func (r *captureReadCloser) Close() error {
	if r.reader == nil {
		return nil
	}
	return r.reader.Close()
}

func (r *captureReadCloser) bytes() []byte   { return append([]byte(nil), r.body.Bytes()...) }
func (r *captureReadCloser) truncated() bool { return r.size > int64(r.body.Len()) }

type captureWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	limit       int64
	size        int64
	body        bytes.Buffer
	proxyError  string
}

func (w *captureWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *captureWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	if n > 0 {
		w.size += int64(n)
		remaining := w.limit - int64(w.body.Len())
		captured := data[:n]
		if remaining > 0 {
			if int64(len(captured)) > remaining {
				captured = captured[:remaining]
			}
			_, _ = w.body.Write(captured)
		}
	}
	return n, err
}

func (w *captureWriter) truncated() bool { return w.size > int64(w.body.Len()) }

func (w *captureWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *captureWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
