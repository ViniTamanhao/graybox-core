package sanitize

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Rules supplement the mandatory credential-header protections.
// JSON paths are non-root RFC 6901 JSON Pointers, as in diff ignore rules.
type Rules struct {
	Headers []string
	JSON    []string
	Query   []string
	Form    []string
}

func Pointer(path string) ([]string, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("JSON field path must be a non-root JSON Pointer beginning with /")
	}
	parts := strings.Split(path[1:], "/")
	for i, part := range parts {
		for j := 0; j < len(part); j++ {
			if part[j] == '~' {
				if j+1 == len(part) || (part[j+1] != '0' && part[j+1] != '1') {
					return nil, errors.New("invalid JSON Pointer escape")
				}
				j++
			}
		}
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
	}
	return parts, nil
}

// ReplaceJSON changes existing fields only. Values are JSON strings, preserving
// unrelated JSON scalar types and exact numeric representations.
func ReplaceJSON(body []byte, values map[string]string, required bool) ([]byte, error) {
	if !utf8.Valid(body) || !json.Valid(body) {
		return nil, errors.New("body is not valid UTF-8 JSON")
	}
	result := append([]byte(nil), body...)
	// Sort so overlapping paths have deterministic behavior.
	paths := sortedKeys(values)
	for _, path := range paths {
		parts, err := Pointer(path)
		if err != nil {
			return nil, err
		}
		replacement, _ := marshalJSON(values[path])
		updated, found, err := replaceJSON(result, parts, replacement)
		if err != nil {
			return nil, err
		}
		if !found && required {
			return nil, errors.New("JSON replacement target is missing: " + path)
		}
		result = updated
	}
	return result, nil
}

func replaceJSON(raw json.RawMessage, parts []string, value json.RawMessage) (json.RawMessage, bool, error) {
	if len(parts) == 0 {
		return value, true, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil {
			return nil, false, errors.New("invalid JSON object")
		}
		child, ok := object[parts[0]]
		if !ok {
			return raw, false, nil
		}
		updated, found, err := replaceJSON(child, parts[1:], value)
		if err != nil || !found {
			return raw, found, err
		}
		object[parts[0]] = updated
		encoded, err := marshalJSON(object)
		return encoded, true, err
	}
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var array []json.RawMessage
		if json.Unmarshal(raw, &array) != nil {
			return nil, false, errors.New("invalid JSON array")
		}
		token := parts[0]
		index, err := strconv.Atoi(token)
		if err != nil || index < 0 || strconv.Itoa(index) != token || index >= len(array) {
			return raw, false, nil
		}
		updated, found, err := replaceJSON(array[index], parts[1:], value)
		if err != nil || !found {
			return raw, found, err
		}
		array[index] = updated
		encoded, err := marshalJSON(array)
		return encoded, true, err
	}
	return raw, false, nil
}

// ReplaceValues replaces all occurrences of existing query/form keys.
func ReplaceValues(raw string, values map[string]string, required bool) (string, error) {
	parsed, err := url.ParseQuery(raw)
	if err != nil {
		return "", errors.New("invalid query or form encoding")
	}
	for _, key := range sortedKeys(values) {
		items, ok := parsed[key]
		if !ok {
			if required {
				return "", errors.New("replacement target is missing: " + key)
			}
			continue
		}
		for i := range items {
			items[i] = values[key]
		}
		parsed[key] = items
	}
	return parsed.Encode(), nil
}

func redactions(paths []string) map[string]string {
	values := make(map[string]string, len(paths))
	for _, path := range paths {
		values[path] = RedactedValue
	}
	return values
}

func (r Rules) ApplyHeaders(src http.Header) http.Header {
	dst := Headers(src)
	for name, values := range dst {
		for _, sensitive := range r.Headers {
			if strings.EqualFold(name, sensitive) {
				for i := range values {
					values[i] = RedactedValue
				}
			}
		}
	}
	return dst
}

func (r Rules) URL(raw string) string {
	if len(r.Query) == 0 {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return RedactedValue
	}
	query, err := ReplaceValues(parsed.RawQuery, redactions(r.Query), false)
	if err != nil {
		parsed.RawQuery = url.Values{"redacted": {RedactedValue}}.Encode()
	} else {
		parsed.RawQuery = query
	}
	return parsed.String()
}

// Body fails closed when configured body rules cannot safely be applied.
// CaptureBody adjusts persisted metadata for the sanitized representation.
func (r Rules) Body(body []byte, headers http.Header, complete bool) []byte {
	if len(body) == 0 || (len(r.JSON) == 0 && len(r.Form) == 0) {
		return append([]byte(nil), body...)
	}
	withheld := []byte(RedactedValue)
	if !complete || headers.Get("Content-Encoding") != "" {
		return withheld
	}
	kind, _, _ := mime.ParseMediaType(headers.Get("Content-Type"))
	var result []byte
	var err error
	switch {
	case kind == "application/json" || strings.HasSuffix(kind, "+json"):
		if len(r.JSON) == 0 {
			return append([]byte(nil), body...)
		}
		result, err = ReplaceJSON(body, redactions(r.JSON), false)
	case kind == "application/x-www-form-urlencoded":
		if len(r.Form) == 0 {
			return append([]byte(nil), body...)
		}
		if !utf8.Valid(body) {
			return withheld
		}
		var encoded string
		encoded, err = ReplaceValues(string(body), redactions(r.Form), false)
		result = []byte(encoded)
	default:
		return withheld
	}
	if err != nil {
		return withheld
	}
	return result
}

// CaptureBody preserves schema-1 size invariants after body transformation.
// Sizes describe the sanitized representation plus uncaptured wire bytes.
// Withheld bodies are incomplete so replay cannot send the omission marker.
func (r Rules) CaptureBody(body []byte, headers http.Header, observed int64, truncated, complete bool) ([]byte, int64, bool) {
	sanitized := r.Body(body, headers, complete && !truncated)
	if bytes.Equal(sanitized, []byte(RedactedValue)) && (len(r.JSON) > 0 || len(r.Form) > 0) && len(body) > 0 {
		complete = false
	}
	if !bytes.Equal(body, sanitized) {
		omitted := max(int64(0), observed-int64(len(body)))
		if truncated && omitted == 0 {
			omitted = 1
		}
		observed = int64(len(sanitized)) + omitted
	}
	return sanitized, observed, complete
}

func marshalJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

// Configured reports whether additional capture protection is enabled.
func (r Rules) Configured() bool {
	return len(r.Headers)+len(r.JSON)+len(r.Query)+len(r.Form) > 0
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
