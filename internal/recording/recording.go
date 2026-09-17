// Package recording contains Graybox's transport-independent recording model.
package recording

import (
	"net/http"
	"time"
)

// SchemaVersion is the recording schema written by this build.
const SchemaVersion = 1

// Exchange is one observed HTTP request and response.
type Exchange struct {
	ID        int64
	Protocol  string
	StartedAt time.Time
	EndedAt   time.Time
	Duration  time.Duration
	Request   Request
	Response  Response
}

// Request contains the replayable parts of an HTTP request.
type Request struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

// Response contains the observed HTTP response.
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// Filter restricts exchange listing. Path is an exact URL-path match unless it
// contains a query string, in which case it matches the complete request URI.
type Filter struct {
	Status int
	Method string
	Path   string
}

// Summary is the compact view used by list output and replay selection.
type Summary struct {
	ID         int64
	Protocol   string
	Method     string
	URL        string
	StatusCode int
	StartedAt  time.Time
	Duration   time.Duration
}
