package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/ViniTamanhao/graybox-core/internal/config"
	"github.com/ViniTamanhao/graybox-core/internal/sanitize"
)

type secretHeaderFlag []string

func (values *secretHeaderFlag) String() string {
	if values == nil {
		return ""
	}

	return strings.Join(
		*values,
		",",
	)
}

func (values *secretHeaderFlag) Set(
	value string,
) error {
	if strings.TrimSpace(value) == "" {
		return errors.New(
			"secret header mapping must not be empty",
		)
	}

	*values = append(
		*values,
		value,
	)

	return nil
}

type secretHeaderMapping struct {
	Header string
	EnvVar string
}

func resolveSecretHeaders(
	values []string,
) (http.Header, secretRedactor, error) {
	headers, _, redactor, err := resolveRequestConfig(config.Config{}, values)
	return headers, redactor, err
}

func parseSecretHeaderMappings(
	values []string,
) ([]secretHeaderMapping, error) {
	mappings := make(
		[]secretHeaderMapping,
		0,
		len(values),
	)

	seenHeaders := make(
		map[string]struct{},
		len(values),
	)

	for _, value := range values {
		headerName, envVar, found := strings.Cut(
			value,
			"=",
		)
		if !found {
			return nil, usageError{
				fmt.Sprintf(
					"invalid --secret-header mapping %q; expected HEADER=ENV_VAR",
					value,
				),
			}
		}

		headerName = strings.TrimSpace(
			headerName,
		)
		envVar = strings.TrimSpace(
			envVar,
		)

		if headerName == "" {
			return nil, usageError{
				fmt.Sprintf(
					"invalid --secret-header mapping %q; header name must not be empty",
					value,
				),
			}
		}

		if envVar == "" {
			return nil, usageError{
				fmt.Sprintf(
					"invalid --secret-header mapping %q; environment variable name must not be empty",
					value,
				),
			}
		}

		if !validHTTPHeaderName(
			headerName,
		) {
			return nil, usageError{
				fmt.Sprintf(
					"invalid request header name %q in --secret-header",
					headerName,
				),
			}
		}

		if !validEnvironmentVariableName(
			envVar,
		) {
			return nil, usageError{
				fmt.Sprintf(
					"invalid environment variable name %q in --secret-header",
					envVar,
				),
			}
		}

		canonicalHeader := http.CanonicalHeaderKey(
			headerName,
		)

		if unsupportedSecretHeader(
			canonicalHeader,
		) {
			return nil, usageError{
				fmt.Sprintf(
					"--secret-header does not support request header %q",
					canonicalHeader,
				),
			}
		}

		normalizedHeader := strings.ToLower(
			canonicalHeader,
		)

		if _, exists := seenHeaders[normalizedHeader]; exists {
			return nil, usageError{
				fmt.Sprintf(
					"duplicate --secret-header mapping for %q",
					canonicalHeader,
				),
			}
		}

		seenHeaders[normalizedHeader] = struct{}{}

		mappings = append(
			mappings,
			secretHeaderMapping{
				Header: canonicalHeader,
				EnvVar: envVar,
			},
		)
	}

	return mappings, nil
}

var unsupportedSecretHeaders = map[string]struct{}{
	"connection":          {},
	"content-length":      {},
	"host":                {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"proxy-connection":    {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},
}

func unsupportedSecretHeader(
	name string,
) bool {
	_, unsupported := unsupportedSecretHeaders[strings.ToLower(name)]

	return unsupported
}

func validHTTPHeaderName(
	name string,
) bool {
	if name == "" {
		return false
	}

	for index := 0; index < len(name); index++ {
		if !httpTokenByte(
			name[index],
		) {
			return false
		}
	}

	return true
}

func httpTokenByte(
	value byte,
) bool {
	switch {
	case value >= 'a' && value <= 'z':
		return true

	case value >= 'A' && value <= 'Z':
		return true

	case value >= '0' && value <= '9':
		return true
	}

	switch value {
	case '!',
		'#',
		'$',
		'%',
		'&',
		'\'',
		'*',
		'+',
		'-',
		'.',
		'^',
		'_',
		'`',
		'|',
		'~':
		return true
	}

	return false
}

func validHTTPHeaderValue(
	value string,
) bool {
	for index := 0; index < len(value); index++ {
		current := value[index]

		if current == '\t' {
			continue
		}

		if current < 0x20 ||
			current == 0x7f {
			return false
		}
	}

	return true
}

func validEnvironmentVariableName(
	name string,
) bool {
	if name == "" {
		return false
	}

	if !environmentVariableStartByte(
		name[0],
	) {
		return false
	}

	for index := 1; index < len(name); index++ {
		if !environmentVariableByte(
			name[index],
		) {
			return false
		}
	}

	return true
}

func environmentVariableStartByte(
	value byte,
) bool {
	return value == '_' ||
		(value >= 'a' && value <= 'z') ||
		(value >= 'A' && value <= 'Z')
}

func environmentVariableByte(
	value byte,
) bool {
	return environmentVariableStartByte(
		value,
	) ||
		(value >= '0' && value <= '9')
}

type secretRedactor struct {
	replacer *strings.Replacer
	values   []string
}

func newSecretRedactor(
	secrets []string,
) secretRedactor {
	seen := make(
		map[string]struct{},
	)

	var replacements []string

	appendValue := func(
		value string,
	) {
		if value == "" {
			return
		}

		if _, exists := seen[value]; exists {
			return
		}

		seen[value] = struct{}{}

		replacements = append(
			replacements,
			value,
		)
	}

	for _, secret := range secrets {
		// Header comparison normalizes names; protect echoed credential keys
		// in their canonical and lower-case spellings as well.
		if validHTTPHeaderName(secret) {
			appendValue(http.CanonicalHeaderKey(secret))
			appendValue(strings.ToLower(secret))
		}
		appendValue(url.QueryEscape(secret))
		appendValue(lowerPercentEscapes(url.QueryEscape(secret)))
		appendValue(url.PathEscape(secret))
		appendValue(lowerPercentEscapes(url.PathEscape(secret)))
		// Diff locations encode application keys as JSON Pointer tokens.
		appendValue(strings.ReplaceAll(strings.ReplaceAll(secret, "~", "~0"), "/", "~1"))

		// Plain-text form. This handles normal human output and errors.
		appendValue(
			secret,
		)

		// Human diff values are rendered with json.Marshal, which escapes
		// quotes and HTML-sensitive characters such as &, <, and >.
		encoded, err := json.Marshal(
			secret,
		)
		if err == nil &&
			len(encoded) >= 2 {
			appendValue(
				string(
					encoded[1 : len(encoded)-1],
				),
			)
		}

		// Keep the non-HTML-escaped JSON representation too. Graybox's
		// machine JSON writer disables HTML escaping, and this also makes the
		// redactor robust for other JSON-formatted human strings.
		encodedWithoutHTMLEscaping, err := jsonStringWithoutHTMLEscaping(
			secret,
		)
		if err == nil {
			appendValue(
				encodedWithoutHTMLEscaping,
			)
		}
	}

	sort.SliceStable(
		replacements,
		func(
			left,
			right int,
		) bool {
			return len(replacements[left]) >
				len(replacements[right])
		},
	)

	if len(replacements) == 0 {
		return secretRedactor{}
	}

	var pairs []string

	for _, replacement := range replacements {
		pairs = append(
			pairs,
			replacement,
			sanitize.RedactedValue,
		)
	}

	return secretRedactor{
		values: append([]string(nil), secrets...),
		replacer: strings.NewReplacer(
			pairs...,
		),
	}
}

func jsonStringWithoutHTMLEscaping(
	value string,
) (string, error) {
	var buffer bytes.Buffer

	encoder := json.NewEncoder(
		&buffer,
	)

	encoder.SetEscapeHTML(
		false,
	)

	if err := encoder.Encode(
		value,
	); err != nil {
		return "", err
	}

	encoded := bytes.TrimSpace(
		buffer.Bytes(),
	)

	if len(encoded) < 2 ||
		encoded[0] != '"' ||
		encoded[len(encoded)-1] != '"' {
		return "", fmt.Errorf(
			"unexpected JSON string encoding",
		)
	}

	return string(
		encoded[1 : len(encoded)-1],
	), nil
}

func (r secretRedactor) redact(
	value string,
) string {
	if r.replacer == nil {
		return value
	}

	return r.replacer.Replace(
		value,
	)
}

// redactData copies application data, including keys, without touching
// Graybox's output structs. Matching credential scalars become markers.
// Redacted keys can collide; sort source keys to keep output deterministic.
func (r secretRedactor) redactData(value any) any {
	if r.replacer == nil {
		return value
	}
	switch typed := value.(type) {
	case string:
		return r.redact(typed)
	case json.Number:
		if scrubbed := r.redact(string(typed)); scrubbed != string(typed) {
			return sanitize.RedactedValue
		}
		return typed
	case bool:
		if scrubbed := r.redact(strconv.FormatBool(typed)); scrubbed != strconv.FormatBool(typed) {
			return sanitize.RedactedValue
		}
		return typed
	case []string:
		if typed == nil {
			return typed
		}
		copied := make([]string, len(typed))
		for i, value := range typed {
			copied[i] = r.redact(value)
		}
		return copied
	case map[string]any:
		if typed == nil {
			return typed
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		output := make(map[string]any, len(typed))
		for _, key := range keys {
			output[r.redact(key)] = r.redactData(typed[key])
		}
		return output
	case []any:
		if typed == nil {
			return typed
		}
		output := make([]any, len(typed))
		for index, item := range typed {
			output[index] = r.redactData(item)
		}
		return output
	default:
		return value
	}
}

func (r secretRedactor) redactJSON(
	data []byte,
) ([]byte, error) {
	if r.replacer == nil {
		return append(
			[]byte(nil),
			data...,
		), nil
	}

	if !json.Valid(
		bytes.TrimSpace(data),
	) {
		return nil, fmt.Errorf(
			"cannot redact invalid JSON output",
		)
	}

	var output bytes.Buffer

	for index := 0; index < len(data); {
		if data[index] != '"' {
			output.WriteByte(
				data[index],
			)

			index++

			continue
		}

		start := index

		index++

		for index < len(data) {
			switch data[index] {
			case '\\':
				if index+1 >= len(data) {
					return nil, fmt.Errorf(
						"unterminated JSON escape",
					)
				}

				index += 2

			case '"':
				index++

				goto stringComplete

			default:
				index++
			}
		}

		return nil, fmt.Errorf(
			"unterminated JSON string",
		)

	stringComplete:
		raw := data[start:index]

		next := index

		for next < len(data) {
			switch data[next] {
			case ' ', '\t', '\r', '\n':
				next++

			default:
				goto whitespaceComplete
			}
		}

	whitespaceComplete:
		// A JSON string followed by ':' is an object key. Leave keys alone:
		// application data keys are scrubbed before serialization. Rewriting a
		// coincidentally matching key could silently change the JSON schema.
		if next < len(data) &&
			data[next] == ':' {
			output.Write(
				raw,
			)

			continue
		}

		var decoded string

		if err := json.Unmarshal(
			raw,
			&decoded,
		); err != nil {
			return nil, fmt.Errorf(
				"decode JSON string for redaction: %w",
				err,
			)
		}

		redacted := r.redact(
			decoded,
		)

		if redacted == decoded {
			output.Write(
				raw,
			)

			continue
		}

		encoded, err := jsonStringWithoutHTMLEscaping(
			redacted,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"encode redacted JSON string: %w",
				err,
			)
		}

		output.WriteByte(
			'"',
		)

		output.WriteString(
			encoded,
		)

		output.WriteByte(
			'"',
		)
	}

	return output.Bytes(), nil
}

func (r secretRedactor) redactError(
	err error,
) error {
	if err == nil {
		return nil
	}

	return redactedError{
		err:      err,
		redactor: r,
	}
}

func (r secretRedactor) writeOutput(
	writer io.Writer,
	render func(io.Writer) error,
) error {
	var output bytes.Buffer

	if err := render(
		&output,
	); err != nil {
		return r.redactError(
			err,
		)
	}

	_, err := io.WriteString(
		writer,
		// Human JSON-valued displays escape '<' and '>' after data scrubbing.
		// Preserve the established readable placeholder in those displays.
		strings.ReplaceAll(r.redact(output.String()), `\u003cREDACTED\u003e`, sanitize.RedactedValue),
	)

	return r.redactError(
		err,
	)
}

func (r secretRedactor) writeJSONOutput(
	writer io.Writer,
	render func(io.Writer) error,
) error {
	var output bytes.Buffer

	if err := render(
		&output,
	); err != nil {
		return r.redactError(
			err,
		)
	}

	redacted, err := r.redactJSON(
		output.Bytes(),
	)
	if err != nil {
		return r.redactError(
			err,
		)
	}

	_, err = writer.Write(
		redacted,
	)

	return r.redactError(
		err,
	)
}

type redactedError struct {
	err      error
	redactor secretRedactor
}

func (e redactedError) Error() string {
	return e.redactor.redact(
		e.err.Error(),
	)
}

func (e redactedError) Unwrap() error {
	return e.err
}

// Percent escapes are case-insensitive; servers can echo either spelling.
func lowerPercentEscapes(value string) string {
	data := []byte(value)
	for i := 0; i+2 < len(data); i++ {
		if data[i] != '%' {
			continue
		}
		for j := i + 1; j <= i+2; j++ {
			if data[j] >= 'A' && data[j] <= 'F' {
				data[j] += 'a' - 'A'
			}
		}
		i += 2
	}
	return string(data)
}
