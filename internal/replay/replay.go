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

	"github.com/opemori/graybox-core/internal/recording"
	"github.com/opemori/graybox-core/internal/sanitize"
)

// Source supplies recorded exchanges.
type Source interface {
	Get(context.Context, int64) (recording.Exchange, error)
	List(context.Context, recording.Filter) ([]recording.Summary, error)
}

// Result describes one replay attempt.
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

// Runner replays requests sequentially with an HTTP client.
type Runner struct {
	Source Source
	Client *http.Client
}

// Run replays all exchanges, or only id when it is non-nil.
func (r Runner) Run(ctx context.Context, target *url.URL, id *int64) ([]Result, error) {
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if id != nil {
		ex, err := r.Source.Get(ctx, *id)
		if err != nil {
			return nil, err
		}
		return []Result{execute(ctx, client, target, ex)}, nil
	}

	summaries, err := r.Source.List(ctx, recording.Filter{})
	if err != nil {
		return nil, err
	}
	// Load and execute one full exchange at a time so replay never retains all
	// recorded bodies in memory. Summaries are intentionally compact.
	results := make([]Result, 0, len(summaries))
	for _, summary := range summaries {
		ex, err := r.Source.Get(ctx, summary.ID)
		if err != nil {
			return nil, err
		}
		results = append(results, execute(ctx, client, target, ex))
	}
	return results, nil
}

func execute(ctx context.Context, client *http.Client, target *url.URL, ex recording.Exchange) Result {
	targetURL, err := BuildURL(target, ex.Request.URL)
	result := Result{ExchangeID: ex.ID, Method: ex.Request.Method, Path: ex.Request.URL}
	if err != nil {
		result.Err = err
		return result
	}
	result.TargetURL = targetURL.String()
	req, err := http.NewRequestWithContext(ctx, ex.Request.Method, result.TargetURL, bytes.NewReader(ex.Request.Body))
	if err != nil {
		result.Err = fmt.Errorf("build replay request: %w", err)
		return result
	}
	copyReplayHeaders(req.Header, ex.Request.Headers)
	started := time.Now()
	resp, err := client.Do(req)
	result.Duration = time.Since(started)
	if err != nil {
		result.Err = fmt.Errorf("send replay request: %w", err)
		return result
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		result.Err = fmt.Errorf("read replay response: %w", err)
		return result
	}
	result.StatusCode = resp.StatusCode
	result.Status = resp.Status
	return result
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

func copyReplayHeaders(dst, src http.Header) {
	omitted := make(map[string]struct{}, len(omittedHeaders))
	for name := range omittedHeaders {
		omitted[name] = struct{}{}
	}
	for _, value := range src.Values("Connection") {
		for token := range strings.SplitSeq(value, ",") {
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
			dst.Add(name, value)
		}
	}
}

// BuildURL replaces the recorded host with target while retaining path/query.
// A path prefix on target is prepended to the recorded request path.
func BuildURL(target *url.URL, recordedURL string) (*url.URL, error) {
	if target == nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return nil, fmt.Errorf("invalid replay target")
	}
	recorded, err := url.Parse(recordedURL)
	if err != nil {
		return nil, fmt.Errorf("parse recorded request URL: %w", err)
	}
	result := *target
	result.Fragment = ""
	result.RawQuery = recorded.RawQuery
	result.ForceQuery = recorded.ForceQuery
	result.Path = joinPath(target.Path, recorded.Path)
	result.RawPath = joinPath(target.EscapedPath(), recorded.EscapedPath())
	if result.RawPath == result.Path {
		result.RawPath = ""
	}
	return &result, nil
}

func joinPath(left, right string) string {
	if left == "" || left == "/" {
		if right == "" {
			return "/"
		}
		if strings.HasPrefix(right, "/") {
			return right
		}
		return "/" + right
	}
	if right == "" || right == "/" {
		return strings.TrimSuffix(left, "/") + "/"
	}
	return strings.TrimSuffix(left, "/") + "/" + strings.TrimPrefix(right, "/")
}
