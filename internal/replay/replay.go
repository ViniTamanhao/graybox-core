// Package replay executes recorded requests against a selected HTTP target.
package replay

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
	"github.com/ViniTamanhao/graybox-core/internal/sanitize"
)

// DefaultResponseBodyCaptureLimit bounds the replayed response bytes retained
// for downstream inspection. The response is still drained completely after
// the retained prefix reaches this limit.
const DefaultResponseBodyCaptureLimit int64 = 10 << 20

// Source supplies recorded exchanges.
type Source interface {
	Get(context.Context, int64) (recording.Exchange, error)
	List(context.Context, recording.Filter) ([]recording.Summary, error)
}

// Result describes one compact replay attempt.
//
// Result deliberately does not retain response bodies. Runner.Run may replay
// many exchanges, so keeping results compact prevents memory use from growing
// with the combined size of all replayed responses.
type Result struct {
	ExchangeID int64
	Method     string
	Path       string
	TargetURL  string
	StatusCode int
	Status     string
	Duration   time.Duration
	Err        error
}

// Execution describes one replay attempt with the observed HTTP response.
//
// Response is nil when Graybox never received an HTTP response, for example
// when request construction or transport establishment failed. When a response
// was received but its body failed while being read, Response remains present
// with Complete=false and Err describes the read failure.
//
// Executions may retain a bounded response-body prefix and should therefore be
// consumed one at a time rather than accumulated for large replay suites.
type Execution struct {
	ExchangeID int64
	Method     string
	Path       string
	TargetURL  string
	Status     string
	Duration   time.Duration
	Response   *recording.Response
	Err        error
}

// VisitFunc consumes one recorded exchange and its replay execution.
//
// Runner.RunEach invokes the visitor synchronously before loading the next full
// exchange, allowing callers such as the diff engine to inspect response bodies
// without retaining every replayed body in memory.
type VisitFunc func(recording.Exchange, Execution) error

// Runner replays requests sequentially with an HTTP client.
type Runner struct {
	Source            Source
	Client            *http.Client
	ResponseBodyLimit int64
}

// Run replays all exchanges, or only id when it is non-nil.
//
// Results remain compact even though replay internally captures each response
// for downstream consumers.
func (r Runner) Run(
	ctx context.Context,
	target *url.URL,
	id *int64,
) ([]Result, error) {
	var results []Result

	err := r.RunEach(
		ctx,
		target,
		id,
		func(_ recording.Exchange, execution Execution) error {
			results = append(
				results,
				execution.result(),
			)
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// RunEach replays all exchanges, or only id when it is non-nil, and invokes
// visit synchronously for each execution.
//
// Only one full recorded exchange and one bounded replay response need to be
// retained at a time. Returning an error from visit stops iteration.
func (r Runner) RunEach(
	ctx context.Context,
	target *url.URL,
	id *int64,
	visit VisitFunc,
) error {
	client := replayClient(r.Client)
	bodyLimit := r.responseBodyLimit()

	if id != nil {
		exchange, err := r.Source.Get(
			ctx,
			*id,
		)
		if err != nil {
			return err
		}

		return visit(
			exchange,
			execute(
				ctx,
				client,
				target,
				exchange,
				bodyLimit,
			),
		)
	}

	summaries, err := r.Source.List(
		ctx,
		recording.Filter{},
	)
	if err != nil {
		return err
	}

	// Load and execute one full exchange at a time so replay never retains all
	// recorded bodies in memory. Summaries are intentionally compact.
	for _, summary := range summaries {
		exchange, err := r.Source.Get(
			ctx,
			summary.ID,
		)
		if err != nil {
			return err
		}

		if err := visit(
			exchange,
			execute(
				ctx,
				client,
				target,
				exchange,
				bodyLimit,
			),
		); err != nil {
			return err
		}
	}

	return nil
}

func (r Runner) responseBodyLimit() int64 {
	if r.ResponseBodyLimit > 0 {
		return r.ResponseBodyLimit
	}

	return DefaultResponseBodyCaptureLimit
}

func (e Execution) result() Result {
	statusCode := 0

	if e.Response != nil {
		statusCode = e.Response.StatusCode
	}

	return Result{
		ExchangeID: e.ExchangeID,
		Method:     e.Method,
		Path:       e.Path,
		TargetURL:  e.TargetURL,
		StatusCode: statusCode,
		Status:     e.Status,
		Duration:   e.Duration,
		Err:        e.Err,
	}
}

func replayClient(base *http.Client) *http.Client {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	if base != nil {
		*client = *base

		if client.Timeout == 0 {
			client.Timeout = 30 * time.Second
		}
	}

	if transport, ok := client.Transport.(*http.Transport); ok {
		transport = transport.Clone()
		transport.DisableCompression = true
		client.Transport = transport
	} else if client.Transport == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DisableCompression = true
		client.Transport = transport
	}

	client.CheckRedirect = func(
		_ *http.Request,
		_ []*http.Request,
	) error {
		return http.ErrUseLastResponse
	}

	return client
}

func execute(
	ctx context.Context,
	client *http.Client,
	target *url.URL,
	exchange recording.Exchange,
	bodyLimit int64,
) Execution {
	result := Execution{
		ExchangeID: exchange.ID,
		Method:     exchange.Request.Method,
		Path:       exchange.Request.URL,
	}

	targetURL, err := BuildURL(
		target,
		exchange.Request.URL,
	)
	if err != nil {
		result.Err = err
		return result
	}

	result.TargetURL = targetURL.String()

	if exchange.Request.Truncated {
		result.Err = fmt.Errorf(
			"cannot replay exchange: request body was truncated",
		)
		return result
	}

	if !exchange.Request.Complete {
		result.Err = fmt.Errorf(
			"cannot replay exchange: request body was incomplete",
		)
		return result
	}

	request, err := http.NewRequestWithContext(
		ctx,
		exchange.Request.Method,
		result.TargetURL,
		bytes.NewReader(exchange.Request.Body),
	)
	if err != nil {
		result.Err = fmt.Errorf(
			"build replay request: %w",
			err,
		)
		return result
	}

	copyReplayHeaders(
		request.Header,
		exchange.Request.Headers,
	)

	started := time.Now()

	response, err := client.Do(request)

	// Preserve the existing replay duration semantics. This currently measures
	// until response headers are received rather than until the body has been
	// fully consumed.
	result.Duration = time.Since(started)

	if err != nil {
		result.Err = fmt.Errorf(
			"send replay request: %w",
			err,
		)
		return result
	}

	defer response.Body.Close()

	result.Status = response.Status

	observed := &recording.Response{
		StatusCode: response.StatusCode,

		// Recorded response headers are sanitized before persistence. Apply the
		// same policy to replayed headers before they can enter a diff result,
		// otherwise secrets such as Set-Cookie could be surfaced by comparison.
		Headers: sanitize.Headers(response.Header),
	}

	result.Response = observed

	body, observedSize, truncated, err := captureResponseBody(
		response.Body,
		bodyLimit,
	)

	observed.Body = body
	observed.ObservedSize = observedSize
	observed.Truncated = truncated
	observed.Complete = err == nil

	if err != nil {
		result.Err = fmt.Errorf(
			"read replay response: %w",
			err,
		)
		return result
	}

	return result
}

type responseBodyCapture struct {
	limit    int64
	observed int64
	body     bytes.Buffer
}

func (c *responseBodyCapture) Write(data []byte) (int, error) {
	c.observed += int64(len(data))

	remaining := c.limit - int64(c.body.Len())

	if remaining > 0 {
		captured := data

		if int64(len(captured)) > remaining {
			captured = captured[:int(remaining)]
		}

		_, _ = c.body.Write(captured)
	}

	// The capture limit bounds retained memory, not replay traffic. Report the
	// complete write so io.Copy continues draining the response body.
	return len(data), nil
}

func (c *responseBodyCapture) bytes() []byte {
	return append(
		[]byte(nil),
		c.body.Bytes()...,
	)
}

func (c *responseBodyCapture) truncated() bool {
	return c.observed > int64(c.body.Len())
}

func captureResponseBody(
	body io.Reader,
	limit int64,
) ([]byte, int64, bool, error) {
	capture := &responseBodyCapture{
		limit: limit,
	}

	_, err := io.Copy(
		capture,
		body,
	)

	return capture.bytes(),
		capture.observed,
		capture.truncated(),
		err
}

var omittedHeaders = map[string]struct{}{
	"connection":          {},
	"content-length":      {},
	"host":                {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},
	"proxy-connection":    {},
}

func copyReplayHeaders(
	dst,
	src http.Header,
) {
	omitted := make(
		map[string]struct{},
		len(omittedHeaders),
	)

	for name := range omittedHeaders {
		omitted[name] = struct{}{}
	}

	for _, value := range src.Values("Connection") {
		for token := range strings.SplitSeq(
			value,
			",",
		) {
			if token = strings.TrimSpace(token); token != "" {
				omitted[strings.ToLower(token)] = struct{}{}
			}
		}
	}

	for name, values := range src {
		if _, omit := omitted[strings.ToLower(name)]; omit {
			continue
		}

		redacted := false

		for _, value := range values {
			if sanitize.IsRedacted(value) {
				redacted = true
				break
			}
		}

		if redacted {
			continue
		}

		for _, value := range values {
			dst.Add(
				name,
				value,
			)
		}
	}
}

// BuildURL replaces the recorded host with target while retaining path/query.
// A path prefix on target is prepended to the recorded request path.
func BuildURL(
	target *url.URL,
	recordedURL string,
) (*url.URL, error) {
	if target == nil ||
		(target.Scheme != "http" && target.Scheme != "https") ||
		target.Host == "" {
		return nil, fmt.Errorf(
			"invalid replay target",
		)
	}

	recorded, err := url.Parse(recordedURL)
	if err != nil {
		return nil, fmt.Errorf(
			"parse recorded request URL: %w",
			err,
		)
	}

	result := *target
	result.Fragment = ""
	result.RawQuery = recorded.RawQuery
	result.ForceQuery = recorded.ForceQuery
	result.Path = joinPath(
		target.Path,
		recorded.Path,
	)
	result.RawPath = joinPath(
		target.EscapedPath(),
		recorded.EscapedPath(),
	)

	if result.RawPath == result.Path {
		result.RawPath = ""
	}

	return &result, nil
}

func joinPath(
	left,
	right string,
) string {
	if left == "" || left == "/" {
		if right == "" {
			return "/"
		}

		if strings.HasPrefix(
			right,
			"/",
		) {
			return right
		}

		return "/" + right
	}

	if right == "" || right == "/" {
		return strings.TrimSuffix(
			left,
			"/",
		) + "/"
	}

	return strings.TrimSuffix(
		left,
		"/",
	) + "/" + strings.TrimPrefix(
		right,
		"/",
	)
}
