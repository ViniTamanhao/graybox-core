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
	ex := recording.Exchange{ID: 1, Protocol: "http", StartedAt: time.Unix(0, 0).UTC(), Duration: 1500 * time.Microsecond,
		Request:  recording.Request{Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"ok":true}`)},
		Response: recording.Response{Headers: http.Header{"Content-Type": {"application/octet-stream"}}, Body: []byte{0, 255}}}
	got := toExchangeJSON(ex)
	if got.DurationMS != 1.5 || got.Request.Body.Encoding != "utf8" || got.Request.Body.Data != `{"ok":true}` {
		t.Fatalf("request JSON = %#v", got)
	}
	if got.Response.Body.Encoding != "base64" || got.Response.Body.Data != base64.StdEncoding.EncodeToString([]byte{0, 255}) {
		t.Fatalf("response JSON body = %#v", got.Response.Body)
	}
}

func TestWriteBodyPrettyPrintsJSONWithoutTrustingContentType(t *testing.T) {
	var output bytes.Buffer
	writeBody(&output, []byte(`{"answer":{"value":42}}`), "application/octet-stream")
	if output.String() != "{\n  \"answer\": {\n    \"value\": 42\n  }\n}\n" {
		t.Fatalf("output = %q", output.String())
	}
}
