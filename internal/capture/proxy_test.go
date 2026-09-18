package capture

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"

	"github.com/opemori/graybox-core/internal/recording"
)

func TestCaptureReadCloserCompletion(t *testing.T) {
	knownLength := newCaptureReadCloser(
		io.NopCloser(bytes.NewReader([]byte("body"))),
		DefaultBodyCaptureLimit,
	)
	buffer := make([]byte, 4)
	if n, err := knownLength.Read(buffer); n != 4 || err != nil {
		t.Fatalf("known-length read = %d, %v", n, err)
	}
	if !knownLength.complete(4) {
		t.Fatal("known-length body was not complete after its expected bytes")
	}

	unknownLength := newCaptureReadCloser(
		io.NopCloser(bytes.NewReader([]byte("body"))),
		DefaultBodyCaptureLimit,
	)
	if n, err := unknownLength.Read(buffer); n != 4 || err != nil {
		t.Fatalf("unknown-length data read = %d, %v", n, err)
	}
	if unknownLength.complete(-1) {
		t.Fatal("unknown-length body was complete before EOF")
	}
	if n, err := unknownLength.Read(buffer); n != 0 || err != io.EOF {
		t.Fatalf("unknown-length EOF read = %d, %v", n, err)
	}
	if !unknownLength.complete(-1) {
		t.Fatal("unknown-length body was incomplete after EOF")
	}

	readFailure := newCaptureReadCloser(
		io.NopCloser(errorReader{data: []byte("part")}),
		DefaultBodyCaptureLimit,
	)
	if n, err := readFailure.Read(buffer); n != 4 || err == nil {
		t.Fatalf("failed read = %d, %v", n, err)
	}
	if readFailure.complete(4) {
		t.Fatal("body with a read error was marked complete")
	}
}

type errorReader struct {
	data []byte
}

func (r errorReader) Read(buffer []byte) (int, error) {
	return copy(buffer, r.data), errors.New("read failed")
}

type shortResponseWriter struct {
	header http.Header
}

func (w *shortResponseWriter) Header() http.Header {
	return w.header
}

func (*shortResponseWriter) Write(data []byte) (int, error) {
	return len(data) - 1, nil
}

func (*shortResponseWriter) WriteHeader(int) {}

func TestCaptureWriterTracksObservedBytesAndShortWrite(t *testing.T) {
	underlying := &shortResponseWriter{header: make(http.Header)}
	writer := &captureWriter{
		ResponseWriter: underlying,
		status:         http.StatusOK,
		limit:          3,
	}

	n, err := writer.Write([]byte("hello"))
	if n != 4 || err != nil {
		t.Fatalf("write = %d, %v", n, err)
	}
	if writer.size != 5 || writer.body.String() != "hel" || !writer.truncated() {
		t.Fatalf("captured response = size %d, body %q, truncated %v", writer.size, writer.body.String(), writer.truncated())
	}
	if !errors.Is(writer.writeErr, io.ErrShortWrite) || writer.complete() {
		t.Fatalf("write error/completion = %v/%v", writer.writeErr, writer.complete())
	}
}

type countingRecorder struct {
	calls int
}

func (r *countingRecorder) Add(context.Context, recording.Exchange) (int64, error) {
	r.calls++
	return int64(r.calls), nil
}

func TestProxyDoesNotSwallowOrPersistProgrammingPanic(t *testing.T) {
	panicValue := errors.New("programming panic")
	recorder := new(countingRecorder)
	proxy := &Proxy{
		proxy: &httputil.ReverseProxy{
			Rewrite: func(*httputil.ProxyRequest) {
				panic(panicValue)
			},
		},
		recorder:  recorder,
		bodyLimit: DefaultBodyCaptureLimit,
	}

	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		proxy.ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "http://proxy.invalid/", nil),
		)
	}()

	if recovered != panicValue {
		t.Fatalf("recovered panic = %v, want %v", recovered, panicValue)
	}
	if recorder.calls != 0 {
		t.Fatalf("programming panic persisted %d exchanges", recorder.calls)
	}
}
