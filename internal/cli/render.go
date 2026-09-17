package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/opemori/graybox-core/internal/recording"
)

type bodyJSON struct {
	ContentType  string `json:"content_type,omitempty"`
	OriginalSize int64  `json:"original_size"`
	CapturedSize int    `json:"captured_size"`
	Truncated    bool   `json:"truncated"`
	Encoding     string `json:"encoding"`
	Data         string `json:"data"`
}

type requestJSON struct {
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Headers http.Header `json:"headers"`
	Body    bodyJSON    `json:"body"`
}

type responseJSON struct {
	Status  int         `json:"status"`
	Headers http.Header `json:"headers"`
	Body    bodyJSON    `json:"body"`
}

type exchangeJSON struct {
	ID         int64        `json:"id"`
	Protocol   string       `json:"protocol"`
	StartedAt  string       `json:"started_at"`
	DurationMS float64      `json:"duration_ms"`
	ProxyError string       `json:"proxy_error,omitempty"`
	Request    requestJSON  `json:"request"`
	Response   responseJSON `json:"response"`
}

func toExchangeJSON(ex recording.Exchange) exchangeJSON {
	return exchangeJSON{
		ID: ex.ID, Protocol: ex.Protocol, StartedAt: ex.StartedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		DurationMS: durationMS(ex.Duration), ProxyError: ex.ProxyError,
		Request: requestJSON{Method: ex.Request.Method, URL: ex.Request.URL, Headers: ex.Request.Headers,
			Body: makeBodyJSON(ex.Request.Body, ex.Request.BodySize, ex.Request.BodyTruncated, ex.Request.Headers.Get("Content-Type"))},
		Response: responseJSON{Status: ex.Response.StatusCode, Headers: ex.Response.Headers,
			Body: makeBodyJSON(ex.Response.Body, ex.Response.BodySize, ex.Response.BodyTruncated, ex.Response.Headers.Get("Content-Type"))},
	}
}

func makeBodyJSON(body []byte, originalSize int64, truncated bool, contentType string) bodyJSON {
	if originalSize == 0 && len(body) > 0 {
		originalSize = int64(len(body))
	}
	result := bodyJSON{ContentType: contentType, OriginalSize: originalSize, CapturedSize: len(body),
		Truncated: truncated || originalSize > int64(len(body)), Encoding: "utf8", Data: string(body)}
	if !isText(body, contentType) {
		result.Encoding = "base64"
		result.Data = base64.StdEncoding.EncodeToString(body)
	}
	return result
}

func isText(body []byte, contentType string) bool {
	if !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return false
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	return strings.HasPrefix(mediaType, "text/") || strings.Contains(mediaType, "json") ||
		strings.Contains(mediaType, "xml") || strings.Contains(mediaType, "javascript") ||
		mediaType == "application/x-www-form-urlencoded" || mediaType == ""
}

func writeHeaders(w interface{ Write([]byte) (int, error) }, headers http.Header) {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range headers[name] {
			fmt.Fprintf(w, "  %s: %s\n", name, value)
		}
	}
}

func writeBody(w interface{ Write([]byte) (int, error) }, body []byte, originalSize int64, truncated bool, contentType string) {
	if originalSize == 0 && len(body) > 0 {
		originalSize = int64(len(body))
	}
	if truncated || originalSize > int64(len(body)) {
		fmt.Fprintf(w, "[truncated: captured %d of %d bytes]\n", len(body), originalSize)
	}
	if len(body) == 0 {
		fmt.Fprintln(w, "(empty)")
		return
	}
	if json.Valid(body) {
		var pretty bytes.Buffer
		if json.Indent(&pretty, body, "", "  ") == nil {
			fmt.Fprintln(w, pretty.String())
			return
		}
	}
	if !isText(body, contentType) {
		fmt.Fprintf(w, "<binary body: %d bytes, content-type %s>\n", len(body), valueOr(contentType, "unknown"))
		return
	}
	fmt.Fprintln(w, string(body))
}

func requestPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.RequestURI()
}

func durationMS(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
