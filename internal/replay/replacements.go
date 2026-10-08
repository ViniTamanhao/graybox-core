package replay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/ViniTamanhao/graybox-core/internal/sanitize"
)

// Replacements contains execution-only values; never persist or log it.
// All replacement body values are strings. Headers may add new fields; body
// and query targets must already exist. No wildcard or template evaluation.
type Replacements struct {
	JSON  map[string]string
	Query map[string]string
	Form  map[string]string
}

// Applicable selects rules for existing fields. Empty bodies and absent fields
// are unrelated; nonempty bodies with unknown encodings cannot be classified.
// This same check is used during CLI credential preflight and execution.
func (r Replacements) Applicable(request *http.Request, body []byte) (Replacements, error) {
	var matched Replacements
	var err error
	if len(r.Query) > 0 {
		if sanitize.QueryWithheld(request.URL.RawQuery) {
			return matched, errors.New("cannot determine replacement fields: request query was withheld by redaction")
		}
		matched.Query, err = sanitize.ExistingValues(request.URL.RawQuery, r.Query)
		if err != nil {
			return matched, err
		}
	}
	if len(body) == 0 || len(r.JSON)+len(r.Form) == 0 {
		return matched, nil
	}
	if request.Header.Get("Content-Encoding") != "" {
		return matched, errors.New("cannot determine replacement fields in an encoded request body")
	}
	kind, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		return matched, errors.New("cannot determine replacement fields: invalid or missing request Content-Type")
	}
	switch {
	case kind == "application/json" || strings.HasSuffix(kind, "+json"):
		if len(r.JSON) > 0 {
			matched.JSON, err = sanitize.ExistingJSON(body, r.JSON)
		}
	case kind == "application/x-www-form-urlencoded":
		if len(r.Form) > 0 {
			if !utf8.Valid(body) {
				return matched, errors.New("form request body is not valid UTF-8")
			}
			matched.Form, err = sanitize.ExistingValues(string(body), r.Form)
		}
	default:
		return matched, errors.New("cannot determine replacement fields in an unsupported request body Content-Type")
	}
	return matched, err
}

func (r Replacements) apply(request *http.Request, body []byte) error {
	matched, err := r.Applicable(request, body)
	if err != nil {
		return err
	}
	for _, group := range []map[string]string{matched.Query, matched.JSON, matched.Form} {
		for field, value := range group {
			if value == "" {
				return fmt.Errorf("runtime replacement value is missing for %s", field)
			}
		}
	}
	if len(matched.Query) > 0 {
		query, err := sanitize.ReplaceValues(request.URL.RawQuery, matched.Query, true)
		if err != nil {
			return err
		}
		request.URL.RawQuery = query
	}
	if len(matched.JSON)+len(matched.Form) == 0 {
		return nil
	}
	var updated []byte
	if len(matched.JSON) > 0 {
		updated, err = sanitize.ReplaceJSON(body, matched.JSON, true)
	} else {
		var encoded string
		encoded, err = sanitize.ReplaceValues(string(body), matched.Form, true)
		updated = []byte(encoded)
	}
	if err != nil {
		return err
	}
	request.Body = io.NopCloser(bytes.NewReader(updated))
	request.ContentLength = int64(len(updated))
	request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(updated)), nil }
	return nil
}
