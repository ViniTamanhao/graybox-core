// Package capture implements Graybox's explicit HTTP reverse proxy.
package capture

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opemori/graybox-core/internal/recording"
	"github.com/opemori/graybox-core/internal/sanitize"
)

// DefaultBodyCaptureLimit bounds the bytes retained for each request and
// response body. Traffic continues to stream after this limit is reached.
const DefaultBodyCaptureLimit int64 = 10 << 20

// Recorder persists an observed exchange.
type Recorder interface {
	Add(context.Context, recording.Exchange) (int64, error)
}

// ErrorHandlers keeps upstream transport diagnostics separate from failures
// to persist an observed exchange.
type ErrorHandlers struct {
	Transport   func(error)
	Persistence func(error)
}

// Proxy is an HTTP handler that forwards and records traffic.
type Proxy struct {
	proxy     *httputil.ReverseProxy
	recorder  Recorder
	errors    ErrorHandlers
	bodyLimit int64

	persistenceFailures atomic.Uint64
}

// NewProxy builds a recording reverse proxy with the default body limit.
func NewProxy(target *url.URL, recorder Recorder, handlers ErrorHandlers) *Proxy {
	return NewProxyWithBodyLimit(target, recorder, DefaultBodyCaptureLimit, handlers)
}

// NewProxyWithBodyLimit builds a recording reverse proxy with a bounded body
// capture. Reaching the capture limit does not truncate proxied traffic.
func NewProxyWithBodyLimit(target *url.URL, recorder Recorder, bodyLimit int64, handlers ErrorHandlers) *Proxy {
	reverse := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			request.Out.URL.RawQuery = request.In.URL.RawQuery
			request.Out.URL.ForceQuery = request.In.URL.ForceQuery
			request.Out.Host = target.Host
			request.SetXForwarded()
		},
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Do not inject Accept-Encoding or transparently decompress: the explicit
	// proxy should forward the representation the client asked to receive.
	transport.DisableCompression = true
	reverse.Transport = captureTransport{base: transport}
	reverse.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		if captured, ok := w.(*captureWriter); ok {
			captured.proxyError = err.Error()
		}
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		if handlers.Transport != nil {
			handlers.Transport(fmt.Errorf("forward request: %w", err))
		}
	}
	return &Proxy{proxy: reverse, recorder: recorder, errors: handlers, bodyLimit: bodyLimit}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()

	requestBody := newCaptureReadCloser(r.Body, p.bodyLimit)
	r.Body = requestBody

	outbound := new(outboundRequest)
	r = r.WithContext(context.WithValue(
		r.Context(),
		outboundRequestKey{},
		outbound,
	))

	captured := &captureWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
		limit:          p.bodyLimit,
	}

	defer func() {
		recovered := recover()
		if recovered != nil && recovered != http.ErrAbortHandler {
			panic(recovered)
		}

		responseComplete := recovered == nil && captured.complete()
		if !responseComplete && captured.proxyError == "" {
			var streamErr error
			if captured.writeErr != nil {
				streamErr = fmt.Errorf("write response: %w", captured.writeErr)
			} else {
				streamErr = errors.New("response stream aborted")
			}
			captured.proxyError = streamErr.Error()
			if p.errors.Transport != nil {
				p.errors.Transport(streamErr)
			}
		}

		p.persistExchange(
			r,
			started,
			requestBody,
			outbound,
			captured,
			responseComplete,
		)

		if recovered != nil {
			panic(recovered)
		}
	}()

	p.proxy.ServeHTTP(captured, r)
}

// PersistenceFailures reports how many observed exchanges could not be
// committed. Upstream HTTP and transport failures are not included.
func (p *Proxy) PersistenceFailures() uint64 { return p.persistenceFailures.Load() }

type outboundRequestKey struct{}

type outboundRequest struct {
	mu      sync.Mutex
	headers http.Header
}

func (r *outboundRequest) setHeaders(headers http.Header) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.headers = headers.Clone()
}

func (r *outboundRequest) snapshot() http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.headers == nil {
		// ReverseProxy can reject a malformed request before it reaches the
		// transport. In that case there was no effective upstream header set;
		// never fall back to persisting the untrusted inbound headers.
		return make(http.Header)
	}
	return r.headers.Clone()
}

type captureTransport struct{ base http.RoundTripper }

func (t captureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if captured, ok := request.Context().Value(outboundRequestKey{}).(*outboundRequest); ok {
		// ReverseProxy has already removed hop-by-hop and spoofed forwarding
		// headers and applied Rewrite before invoking its transport.
		captured.setHeaders(request.Header)
	}
	return t.base.RoundTrip(request)
}

type captureReadCloser struct {
	reader  io.ReadCloser
	limit   int64
	size    int64
	body    bytes.Buffer
	sawEOF  bool
	readErr error
}

func newCaptureReadCloser(reader io.ReadCloser, limit int64) *captureReadCloser {
	return &captureReadCloser{reader: reader, limit: limit}
}

func (r *captureReadCloser) Read(data []byte) (int, error) {
	if r.reader == nil {
		r.sawEOF = true
		return 0, io.EOF
	}

	n, err := r.reader.Read(data)
	if n > 0 {
		r.size += int64(n)
		r.capture(data[:n])
	}

	switch {
	case err == io.EOF:
		r.sawEOF = true
	case err != nil:
		r.readErr = err
	}

	return n, err
}

func (r *captureReadCloser) complete(contentLength int64) bool {
	if r.readErr != nil {
		return false
	}
	if contentLength >= 0 {
		return r.size >= contentLength
	}
	return r.sawEOF
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
	writeErr    error
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

	w.size += int64(len(data))
	w.capture(data)

	n, err := w.ResponseWriter.Write(data)
	if err != nil && w.writeErr == nil {
		w.writeErr = err
	}
	if n < len(data) && err == nil && w.writeErr == nil {
		w.writeErr = io.ErrShortWrite
	}
	return n, err
}

func (w *captureWriter) capture(data []byte) {
	remaining := w.limit - int64(w.body.Len())
	if remaining <= 0 {
		return
	}
	if int64(len(data)) > remaining {
		data = data[:remaining]
	}
	_, _ = w.body.Write(data)
}

func (w *captureWriter) complete() bool {
	return w.writeErr == nil
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

func (p *Proxy) persistExchange(
	r *http.Request,
	started time.Time,
	requestBody *captureReadCloser,
	outbound *outboundRequest,
	captured *captureWriter,
	responseComplete bool,
) {
	ended := time.Now()

	exchange := recording.Exchange{
		Protocol:   "http",
		StartedAt:  started.UTC(),
		EndedAt:    ended.UTC(),
		Duration:   ended.Sub(started),
		ProxyError: captured.proxyError,
		Request: recording.Request{
			Method:       r.Method,
			URL:          r.URL.RequestURI(),
			Headers:      sanitize.Headers(outbound.snapshot()),
			Body:         requestBody.bytes(),
			ObservedSize: requestBody.size,
			Truncated:    requestBody.truncated(),
			Complete:     requestBody.complete(r.ContentLength),
		},
		Response: recording.Response{
			StatusCode:   captured.status,
			Headers:      sanitize.Headers(captured.Header()),
			Body:         append([]byte(nil), captured.body.Bytes()...),
			ObservedSize: captured.size,
			Truncated:    captured.truncated(),
			Complete:     responseComplete,
		},
	}

	if _, err := p.recorder.Add(
		context.WithoutCancel(r.Context()),
		exchange,
	); err != nil {
		p.persistenceFailures.Add(1)
		if p.errors.Persistence != nil {
			p.errors.Persistence(
				fmt.Errorf("record exchange: %w", err),
			)
		}
	}
}
