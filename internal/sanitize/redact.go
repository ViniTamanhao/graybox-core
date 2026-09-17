// Package sanitize applies predictable, deliberately small V0 redaction rules.
package sanitize

import (
	"net/http"
	"strings"
)

const RedactedValue = "<REDACTED>"

var sensitiveHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
}

// Headers returns a deep copy with known credential-bearing values redacted.
func Headers(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for name, values := range src {
		copied := append([]string(nil), values...)
		if _, sensitive := sensitiveHeaders[strings.ToLower(name)]; sensitive {
			for i := range copied {
				copied[i] = RedactedValue
			}
		}
		dst[name] = copied
	}
	return dst
}

// IsRedacted reports whether a persisted header value must not be replayed.
func IsRedacted(value string) bool { return value == RedactedValue }
