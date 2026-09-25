package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

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
	mappings, err := parseSecretHeaderMappings(
		values,
	)
	if err != nil {
		return nil, secretRedactor{}, err
	}

	headers := make(
		http.Header,
		len(mappings),
	)

	secrets := make(
		[]string,
		0,
		len(mappings),
	)

	for _, mapping := range mappings {
		value, exists := os.LookupEnv(
			mapping.EnvVar,
		)
		if !exists {
			return nil, secretRedactor{}, usageError{
				fmt.Sprintf(
					"environment variable %q for --secret-header %s is not set",
					mapping.EnvVar,
					mapping.Header,
				),
			}
		}

		if value == "" {
			return nil, secretRedactor{}, usageError{
				fmt.Sprintf(
					"environment variable %q for --secret-header %s is empty",
					mapping.EnvVar,
					mapping.Header,
				),
			}
		}

		if !validHTTPHeaderValue(
			value,
		) {
			return nil, secretRedactor{}, usageError{
				fmt.Sprintf(
					"environment variable %q contains an invalid HTTP header value for %s",
					mapping.EnvVar,
					mapping.Header,
				),
			}
		}

		headers.Set(
			mapping.Header,
			value,
		)

		secrets = append(
			secrets,
			value,
		)
	}

	return headers,
		newSecretRedactor(
			secrets,
		),
		nil
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
}

func newSecretRedactor(
	secrets []string,
) secretRedactor {
	seen := make(
		map[string]struct{},
		len(secrets)*3,
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
		// Human/plain-text representation.
		appendValue(
			secret,
		)

		// Standard encoding/json representation. json.Marshal escapes HTML
		// characters such as &, <, and >.
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

		// Graybox's JSON output may disable HTML escaping. Retain that escaped
		// representation as well so secrets containing characters such as &
		// are still removed from machine-readable output.
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

	pairs := make(
		[]string,
		0,
		len(replacements)*2,
	)

	for _, replacement := range replacements {
		pairs = append(
			pairs,
			replacement,
			sanitize.RedactedValue,
		)
	}

	return secretRedactor{
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
		r.redact(
			output.String(),
		),
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
