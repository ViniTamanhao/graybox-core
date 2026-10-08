package replay

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRequestReplacements(t *testing.T) {
	for _, tc := range []struct {
		kind, body  string
		replacement Replacements
		expected    string
	}{
		{"application/json", `{"credentials":{"key":"old"},"count":7}`, Replacements{JSON: map[string]string{"/credentials/key": "new&+key"}, Query: map[string]string{"token": "new&+query"}}, `"key":"new&+key"`},
		{"application/x-www-form-urlencoded", "secret=old&secret=other&scope=a%2Bb", Replacements{Form: map[string]string{"secret": "new&+key"}, Query: map[string]string{"token": "new&+query"}}, "secret=new%26%2Bkey&secret=new%26%2Bkey"},
	} {
		req, _ := http.NewRequest("POST", "http://localhost/oauth?token=old&scope=a%2Bb", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.kind)
		body := []byte(tc.body)
		if err := tc.replacement.apply(req, body); err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(req.Body)
		if !bytes.Contains(got, []byte(tc.expected)) || req.ContentLength != int64(len(got)) || req.URL.Query().Get("token") != "new&+query" || req.URL.Query().Get("scope") != "a+b" || string(body) != tc.body {
			t.Fatalf("incorrect replacement: %s %s", got, req.URL)
		}
		copy, err := req.GetBody()
		if err != nil {
			t.Fatal(err)
		}
		again, _ := io.ReadAll(copy)
		copy.Close()
		if !bytes.Equal(got, again) {
			t.Fatal("GetBody mismatch")
		}
	}
}

func TestRequestReplacementErrorsDoNotExposeValues(t *testing.T) {
	secret := "runtime-sensitive-token"
	for _, tc := range []struct {
		body, kind, query, encoding string
		replacements                Replacements
	}{
		{`{"existing":"old"}`, "application/json", "", "", Replacements{JSON: map[string]string{"/missing": secret}}},
		{`{"key":"old"`, "application/json", "", "", Replacements{JSON: map[string]string{"/key": secret}}},
		{`{"key":"old"}`, "application/json", "", "gzip", Replacements{JSON: map[string]string{"/key": secret}}},
		{"key=old%zz", "application/x-www-form-urlencoded", "", "", Replacements{Form: map[string]string{"key": secret}}},
		{"key=old", "application/x-www-form-urlencoded", "", "", Replacements{Form: map[string]string{"missing": secret}}},
		{"\xffkey=old", "application/x-www-form-urlencoded", "", "", Replacements{Form: map[string]string{"key": secret}}},
		{"opaque", "application/octet-stream", "", "", Replacements{JSON: map[string]string{"/key": secret}}},
		{"", "text/plain", "other=old", "", Replacements{Query: map[string]string{"missing": secret}}},
		{"", "text/plain", "key=old%zz", "", Replacements{Query: map[string]string{"key": secret}}},
	} {
		req := &http.Request{URL: &url.URL{RawQuery: tc.query}, Header: http.Header{"Content-Type": {tc.kind}, "Content-Encoding": {tc.encoding}}}
		err := tc.replacements.apply(req, []byte(tc.body))
		if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), tc.body) && len(tc.body) > 0 {
			t.Fatalf("unsafe replacement error: %v", err)
		}
	}
}
