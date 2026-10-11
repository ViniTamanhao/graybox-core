package sanitize

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestConfiguredRedaction(t *testing.T) {
	rules := Rules{Headers: []string{"x-custom-secret"}, JSON: []string{"/credentials/api_key", "/items/0/token", "/a~1b/~0key"}, Query: []string{"access_token"}, Form: []string{"client_secret", "refresh_token"}}
	headers := http.Header{"Authorization": {"original"}, "Cookie": {"cookie"}, "X-Custom-Secret": {"one", "two"}, "Accept": {"application/json"}}
	got := rules.ApplyHeaders(headers)
	if got.Get("Authorization") != RedactedValue || got.Get("Cookie") != RedactedValue || got.Get("X-Custom-Secret") != RedactedValue || got.Get("Accept") != "application/json" || headers.Get("Authorization") != "original" {
		t.Fatalf("bad header sanitization: %v", got)
	}
	original := []byte(`{"credentials":{"api_key":"secret","user":"public"},"items":[{"token":"token"},{"token":"keep"}],"a/b":{"~key":"hidden"},"number":9007199254740993}`)
	body := rules.Body(original, http.Header{"Content-Type": {"application/json"}}, true)
	for _, secret := range []string{`"secret"`, `"token":"token"`, `"hidden"`} {
		if bytes.Contains(body, []byte(secret)) {
			t.Fatal("secret leaked")
		}
	}
	if !json.Valid(body) || !bytes.Contains(body, []byte("9007199254740993")) || !bytes.Contains(body, []byte(`"keep"`)) || !bytes.Contains(original, []byte(`"secret"`)) {
		t.Fatalf("bad JSON redaction: %s", body)
	}
	parsed, _ := url.Parse(rules.URL("/oauth?access_token=one&access_token=two&scope=a%2Bb"))
	values := parsed.Query()
	if len(values["access_token"]) != 2 || values.Get("access_token") != RedactedValue || values.Get("scope") != "a+b" {
		t.Fatal(values)
	}
	form := rules.Body([]byte("client_secret=s%26e&refresh_token=tok&scope=a%2Bb"), http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, true)
	fields, _ := url.ParseQuery(string(form))
	if fields.Get("client_secret") != RedactedValue || fields.Get("refresh_token") != RedactedValue || fields.Get("scope") != "a+b" {
		t.Fatal(fields)
	}
}

func TestBodyRedactionFailsClosed(t *testing.T) {
	rules := Rules{JSON: []string{"/secret"}, Form: []string{"secret"}, Query: []string{"access_token"}}
	for _, tc := range []struct {
		body, kind, encoding string
		complete             bool
	}{
		{`{"secret":"leak"`, "application/json", "", true},
		{`{"secret":"leak"}`, "application/json", "", false},
		{"\x00\xffleak", "application/octet-stream", "", true},
		{"secret=leak%zz", "application/x-www-form-urlencoded", "", true},
		{`{"secret":"leak"}`, "application/json", "gzip", true},
		{"leak", "text/plain", "", true},
	} {
		headers := http.Header{"Content-Type": {tc.kind}, "Content-Encoding": {tc.encoding}}
		if got := string(rules.Body([]byte(tc.body), headers, tc.complete)); got != RedactedValue {
			t.Fatalf("unsafe body: %q", got)
		}
		if got := string((Rules{}).Body([]byte(tc.body), headers, tc.complete)); got != tc.body {
			t.Fatal("default behavior changed")
		}
	}
	if got := rules.URL("/oauth?secret=leak&access_token=%zz"); strings.Contains(got, "leak") {
		t.Fatal("malformed query leaked")
	}
}

func TestJSONPathsAndReplacements(t *testing.T) {
	for _, path := range []string{"", "credentials.key", "/bad~", "/bad~2"} {
		if _, err := Pointer(path); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	body := []byte(`{"items":[{"token":"<REDACTED>"}],"enabled":true,"count":17,"nil":null}`)
	replaced, err := ReplaceJSON(body, map[string]string{"/items/0/token": "new&+token", "/count": "42"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if json.Unmarshal(replaced, &got) != nil || got["count"] != "42" || got["enabled"] != true {
		t.Fatal(string(replaced))
	}
	for _, path := range []string{"/missing", "/items/1/token", "/items/00/token", "/items/-/token", "/enabled/token"} {
		if _, err := ReplaceJSON(body, map[string]string{path: "secret"}, true); err == nil {
			t.Fatalf("missing %s accepted", path)
		}
	}
	if !bytes.Contains(body, []byte(RedactedValue)) {
		t.Fatal("original mutated")
	}
}

func TestSanitizedCaptureMetadata(t *testing.T) {
	rules := Rules{JSON: []string{"/key"}}
	headers := http.Header{"Content-Type": {"application/json"}}
	for _, original := range []string{`{"key":"x"}`, `{"key":"a-much-longer-original-secret"}`} {
		body, size, complete := rules.CaptureBody([]byte(original), headers, int64(len(original)), false, true)
		if !complete || size != int64(len(body)) || bytes.Contains(body, []byte("original-secret")) {
			t.Fatal("invalid sanitized metadata")
		}
	}
	for _, truncated := range []bool{true, false} {
		original := []byte(`{"key":"secret`)
		observed := int64(len(original))
		if truncated {
			observed += 100
		}
		body, size, complete := rules.CaptureBody(original, headers, observed, truncated, true)
		if complete || string(body) != RedactedValue || truncated != (size > int64(len(body))) {
			t.Fatal("unsafe withheld metadata")
		}
	}
}

func TestDuplicateJSONKeysFailClosed(t *testing.T) {
	rules := Rules{JSON: []string{"/credentials/api_key"}}
	for _, body := range []string{
		`{"credentials":{"api_key":"original-sensitive"},"credentials":{}}`,
		`{"credentials":{"api_key":"original-sensitive","api_key":"second-sensitive"}}`,
		`{"unrelated":{"a":1,"a":2},"credentials":{"api_key":"original-sensitive"}}`,
	} {
		result, _, complete := rules.CaptureBody([]byte(body), http.Header{"Content-Type": {"application/json"}}, int64(len(body)), false, true)
		if complete || string(result) != RedactedValue {
			t.Fatal("ambiguous JSON leaked or remained replayable")
		}
		if _, err := ExistingJSON([]byte(body), map[string]string{"/credentials/api_key": "new-sensitive"}); err == nil {
			t.Fatal("ambiguous JSON accepted for replacement")
		}
	}
}
