package cli

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/opemori/graybox-core/internal/recording"
)

func TestExchangeJSONBodyEncodings(t *testing.T) {
	ex := recording.Exchange{
		ID:        1,
		Protocol:  "http",
		StartedAt: time.Unix(0, 0).UTC(),
		Duration:  1500 * time.Microsecond,
		Request: recording.Request{
			Headers:      http.Header{"Content-Type": {"application/json"}},
			Body:         []byte(`{"ok":true}`),
			ObservedSize: 11,
			Complete:     true,
		},
		Response: recording.Response{
			Headers:      http.Header{"Content-Type": {"application/octet-stream"}},
			Body:         []byte{0, 255},
			ObservedSize: 5,
			Truncated:    true,
			Complete:     false,
		},
	}
	got := toExchangeJSON(ex)
	if got.DurationMS != 1.5 || got.Request.Body.Encoding != "utf8" || got.Request.Body.Data != `{"ok":true}` {
		t.Fatalf("request JSON = %#v", got)
	}
	if got.Response.Body.Encoding != "base64" || got.Response.Body.Data != base64.StdEncoding.EncodeToString([]byte{0, 255}) {
		t.Fatalf("response JSON body = %#v", got.Response.Body)
	}
	if got.Response.Body.ObservedSize != 5 || got.Response.Body.CapturedSize != 2 ||
		!got.Response.Body.Truncated || got.Response.Body.Complete {
		t.Fatalf("response body metadata = %#v", got.Response.Body)
	}
	if !got.Request.Body.Complete {
		t.Fatalf("request body metadata = %#v", got.Request.Body)
	}
}

func TestWriteBodyLabelsTruncationAndIncompleteness(t *testing.T) {
	var output bytes.Buffer
	writeBody(&output, []byte("part"), 10, true, false, "text/plain")
	if !bytes.Contains(output.Bytes(), []byte("[capture truncated: captured 4 of 10 observed bytes]")) ||
		!bytes.Contains(output.Bytes(), []byte("[incomplete: body stream did not finish normally]")) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestWriteBodyPrettyPrintsJSONWithoutTrustingContentType(t *testing.T) {
	var output bytes.Buffer
	writeBody(
		&output,
		[]byte(`{"answer":{"value":42}}`),
		23,
		false,
		true,
		"application/octet-stream",
	)
	if output.String() != "{\n  \"answer\": {\n    \"value\": 42\n  }\n}\n" {
		t.Fatalf("output = %q", output.String())
	}
}
