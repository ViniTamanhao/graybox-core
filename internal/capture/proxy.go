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

// Recorder persists a completed observed exchange.
type Recorder interface {
	Add(context.Context, recording.Exchange) (int64, error)
}

// Proxy is an HTTP handler that forwards and records traffic.
type Proxy struct {
	proxy    *httputil.ReverseProxy
	recorder Recorder
	onError  func(error)
}

// NewProxy builds a recording reverse proxy for target.
func NewProxy(target *url.URL, recorder Recorder, onError func(error)) *Proxy {
	reverse := httputil.NewSingleHostReverseProxy(target)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Do not inject Accept-Encoding or transparently decompress: the explicit
	// proxy should forward the representation the client asked to receive.
	transport.DisableCompression = true
	reverse.Transport = transport
	reverse.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		if onError != nil {
			onError(fmt.Errorf("forward request: %w", err))
		}
	}
	return &Proxy{proxy: reverse, recorder: recorder, onError: onError}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		p.report(fmt.Errorf("read request body: %w", err))
		return
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))

	captured := &captureWriter{ResponseWriter: w, status: http.StatusOK}
	p.proxy.ServeHTTP(captured, r)
	ended := time.Now()
	exchange := recording.Exchange{
		Protocol:  "http",
		StartedAt: started.UTC(),
		EndedAt:   ended.UTC(),
		Duration:  ended.Sub(started),
		Request: recording.Request{
			Method:  r.Method,
			URL:     r.URL.RequestURI(),
			Headers: sanitize.Headers(r.Header),
			Body:    append([]byte(nil), body...),
		},
		Response: recording.Response{
			StatusCode: captured.status,
			Headers:    sanitize.Headers(captured.Header()),
			Body:       append([]byte(nil), captured.body.Bytes()...),
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

type captureWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	body        bytes.Buffer
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
		_, _ = w.body.Write(data[:n])
	}
	return n, err
}

func (w *captureWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *captureWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
