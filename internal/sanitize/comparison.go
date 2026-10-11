package sanitize

import (
	"bytes"
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// WithRecordedMarkers supplements the current policy with exact historical
// redaction locations recoverable from the baseline. It only inspects data in
// memory. Literal markers are conservatively protected.
func (r Rules) WithRecordedMarkers(headers http.Header, body []byte) Rules {
	r.Headers = append([]string(nil), r.Headers...)
	r.JSON = append([]string(nil), r.JSON...)
	r.Form = append([]string(nil), r.Form...)
	for name, values := range headers {
		if _, mandatory := sensitiveHeaders[strings.ToLower(name)]; mandatory {
			continue
		}
		for _, value := range values {
			if IsRedacted(value) {
				r.Headers = append(r.Headers, name)
				break
			}
		}
	}
	kind, _, _ := mime.ParseMediaType(headers.Get("Content-Type"))
	switch {
	case kind == "application/json" || strings.HasSuffix(kind, "+json"):
		if !validJSON(body) {
			break
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var value any
		if decoder.Decode(&value) == nil {
			r.JSON = append(r.JSON, markerPointers(value, "")...)
		}
	case kind == "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(string(body))
		if err != nil {
			break
		}
		for key, items := range values {
			for _, item := range items {
				if IsRedacted(item) {
					r.Form = append(r.Form, key)
					break
				}
			}
		}
	}
	// Stable rules make overlap handling and normalization reproducible.
	sort.Strings(r.Headers)
	sort.Strings(r.JSON)
	sort.Strings(r.Form)
	return r
}

// WithRecordedQuery protects marker-bearing request parameters in reports.
func (r Rules) WithRecordedQuery(raw string) Rules {
	r.Query = append([]string(nil), r.Query...)
	parsed, err := url.Parse(raw)
	if err != nil {
		return r
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return r
	}
	for key, items := range values {
		for _, item := range items {
			if IsRedacted(item) {
				r.Query = append(r.Query, key)
				break
			}
		}
	}
	sort.Strings(r.Query)
	return r
}

func markerPointers(value any, pointer string) []string {
	switch typed := value.(type) {
	case string:
		if pointer != "" && IsRedacted(typed) {
			return []string{pointer}
		}
	case map[string]any:
		var paths []string
		for key, child := range typed {
			escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			paths = append(paths, markerPointers(child, pointer+"/"+escaped)...)
		}
		return paths
	case []any:
		var paths []string
		for index, child := range typed {
			paths = append(paths, markerPointers(child, pointer+"/"+strconv.Itoa(index))...)
		}
		return paths
	}
	return nil
}
