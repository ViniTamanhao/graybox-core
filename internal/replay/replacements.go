package replay

import (
	"bytes"
	"errors"
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

func (r Replacements) apply(request *http.Request, body []byte) error {
	if len(r.Query) > 0 {
		query, err := sanitize.ReplaceValues(request.URL.RawQuery, r.Query, true)
		if err != nil {
			return err
		}
		request.URL.RawQuery = query
	}
	if len(r.JSON) == 0 && len(r.Form) == 0 {
		return nil
	}
	if request.Header.Get("Content-Encoding") != "" {
		return errors.New("cannot replace fields in an encoded request body")
	}
	kind, _, _ := mime.ParseMediaType(request.Header.Get("Content-Type"))
	var updated []byte
	var err error
	if (kind == "application/json" || strings.HasSuffix(kind, "+json")) && len(r.JSON) > 0 {
		updated, err = sanitize.ReplaceJSON(body, r.JSON, true)
	} else if kind == "application/x-www-form-urlencoded" && len(r.Form) > 0 {
		if !utf8.Valid(body) {
			return errors.New("form request body is not valid UTF-8")
		}
		var encoded string
		encoded, err = sanitize.ReplaceValues(string(body), r.Form, true)
		updated = []byte(encoded)
	} else {
		return errors.New("body replacements require a matching JSON or form request Content-Type")
	}
	if err != nil {
		return err
	}
	request.Body = io.NopCloser(bytes.NewReader(updated))
	request.ContentLength = int64(len(updated))
	request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(updated)), nil }
	return nil
}
