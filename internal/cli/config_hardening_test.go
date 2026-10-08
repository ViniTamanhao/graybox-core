package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
	"github.com/ViniTamanhao/graybox-core/internal/sanitize"
	"github.com/ViniTamanhao/graybox-core/internal/storage"
)

func hardeningRecording(t *testing.T, target string, requests []recording.Request, responses []recording.Response) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.graybox")
	store, err := storage.Create(context.Background(), path, "v0.2.2")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SetMetadata(context.Background(), "target_url", target); err != nil {
		t.Fatal(err)
	}
	for i, request := range requests {
		response := responses[i]
		request.ObservedSize = int64(len(request.Body))
		if request.Truncated {
			request.ObservedSize++
		}
		response.ObservedSize = int64(len(response.Body))
		if response.Truncated {
			response.ObservedSize++
		}
		if _, err := store.Add(context.Background(), recording.Exchange{Protocol: "http", StartedAt: time.Unix(0, 0).UTC(), EndedAt: time.Unix(0, 0).UTC(), Request: request, Response: response}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeHardeningConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "graybox.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func runHardeningCLI(t *testing.T, command, path, configPath string, jsonOutput bool, extra ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	args := []string{command, path, "--config", configPath}
	if jsonOutput {
		args = append(args, "--json")
	}
	args = append(args, extra...)
	code := (App{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), args)
	if jsonOutput && stdout.Len() > 0 && !json.Valid(stdout.Bytes()) {
		t.Fatalf("invalid output JSON: %s", &stdout)
	}
	return code, stdout.String() + stderr.String()
}

func TestMixedSessionReplacementApplicabilityCLI(t *testing.T) {
	const refresh = "mixed-refresh&+?/token"
	const key = "mixed-nested-key"
	const auth = "Bearer mixed-auth"
	t.Setenv("GRAYBOX_MIXED_REFRESH", refresh)
	t.Setenv("GRAYBOX_MIXED_KEY", key)
	t.Setenv("GRAYBOX_MIXED_AUTH", auth)
	t.Setenv("GRAYBOX_MIXED_UNUSED", "")
	configPath := writeHardeningConfig(t, `replay:
  headers:
    Authorization: "${GRAYBOX_MIXED_AUTH}"
  json:
    /refresh_token: "${GRAYBOX_MIXED_REFRESH}"
    /credentials/api_key: "${GRAYBOX_MIXED_KEY}"
    /items/0/token: "${GRAYBOX_MIXED_KEY}"
    /unused: "${GRAYBOX_MIXED_UNUSED}"
  form:
    refresh_token: "${GRAYBOX_MIXED_REFRESH}"
    unused: "${GRAYBOX_MIXED_UNUSED}"
  query:
    access_token: "${GRAYBOX_MIXED_REFRESH}"
    unused: "${GRAYBOX_MIXED_UNUSED}"
`)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != auth {
			t.Error("header replacement changed")
		}
		body, _ := io.ReadAll(r.Body)
		if r.ContentLength > 0 && r.ContentLength != int64(len(body)) {
			t.Error("wrong request content length")
		}
		switch r.URL.Path {
		case "/oauth/json":
			var object map[string]json.RawMessage
			if json.Unmarshal(body, &object) != nil {
				t.Error("invalid reconstructed JSON")
			}
			var token string
			_ = json.Unmarshal(object["refresh_token"], &token)
			if token != refresh || string(object["number"]) != "9007199254740993" || !bytes.Contains(object["credentials"], []byte(key)) || !bytes.Contains(object["items"], []byte(key)) {
				t.Error("JSON fields not restored")
			}
		case "/oauth/form":
			form, err := url.ParseQuery(string(body))
			if err != nil || form.Get("refresh_token") != refresh || len(form["refresh_token"]) != 2 || form.Get("scope") != "a+b" {
				t.Error("form fields not restored")
			}
		case "/oauth/query":
			if r.URL.Query().Get("access_token") != refresh || len(r.URL.Query()["access_token"]) != 2 || r.URL.Query().Get("scope") != "a+b" {
				t.Error("query fields not restored")
			}
		case "/orders":
			if string(body) != `{ "item": "book" }` || r.URL.RawQuery != "scope=a%2bb" {
				t.Error("unrelated JSON/query changed")
			}
		case "/orders/bulk":
			if string(body) != `[{"item":"book"}]` {
				t.Error("unrelated root JSON array changed")
			}
		case "/orders/form":
			if string(body) != "item=book&scope=a%2bb" {
				t.Error("unrelated form changed")
			}
		case "/users/me":
			if len(body) != 0 || r.URL.RawQuery != "scope=a%2bb" {
				t.Error("ordinary GET changed")
			}
		default:
			t.Error("unexpected request")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"public":"ok"}`)
	}))
	defer server.Close()
	request := func(method, location, kind, body string) recording.Request {
		return recording.Request{Method: method, URL: location, Headers: http.Header{"Content-Type": {kind}}, Body: []byte(body), Complete: true}
	}
	requests := []recording.Request{
		request("POST", "/oauth/json", "application/json", `{"refresh_token":"<REDACTED>","credentials":{"api_key":"<REDACTED>"},"items":[{"token":"<REDACTED>"}],"number":9007199254740993}`),
		request("GET", "/users/me?scope=a%2bb", "", ""),
		request("POST", "/orders?scope=a%2bb", "application/json", `{ "item": "book" }`),
		request("POST", "/orders/bulk", "application/json", `[{"item":"book"}]`),
		request("POST", "/oauth/form", "application/x-www-form-urlencoded", "refresh_token=%3CREDACTED%3E&refresh_token=%3CREDACTED%3E&scope=a%2Bb"),
		request("POST", "/orders/form", "application/x-www-form-urlencoded", "item=book&scope=a%2bb"),
		request("GET", "/oauth/query?access_token=%3CREDACTED%3E&access_token=%3CREDACTED%3E&scope=a%2Bb", "application/octet-stream", ""),
	}
	responses := make([]recording.Response, len(requests))
	for i := range responses {
		responses[i] = recording.Response{StatusCode: 200, Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"public":"ok"}`), Complete: true}
	}
	path := hardeningRecording(t, server.URL, requests, responses)
	before, _ := os.ReadFile(path)
	for _, command := range []string{"replay", "diff"} {
		for _, jsonOutput := range []bool{false, true} {
			code, output := runHardeningCLI(t, command, path, configPath, jsonOutput)
			if code != ExitSuccess {
				t.Fatalf("%s code=%d output=%s", command, code, output)
			}
			for _, secret := range []string{refresh, key, auth, url.QueryEscape(refresh)} {
				if strings.Contains(output, secret) {
					t.Fatal("runtime credential leaked")
				}
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("recording changed")
			}
		}
	}
	if calls.Load() != int32(4*len(requests)) {
		t.Fatalf("sent %d requests", calls.Load())
	}
	// Selection must not demand credentials used only by other exchanges.
	t.Setenv("GRAYBOX_MIXED_REFRESH", "")
	t.Setenv("GRAYBOX_MIXED_KEY", "")
	for _, command := range []string{"replay", "diff"} {
		code, output := runHardeningCLI(t, command, path, configPath, true, "--id", "2")
		if code != ExitSuccess {
			t.Fatalf("unrelated selection %d %s", code, output)
		}
	}
}

func TestApplicableMissingCredentialsCLIPreflight(t *testing.T) {
	t.Setenv("GRAYBOX_REQUIRED_CREDENTIAL", "")
	for _, location := range []string{"json", "form", "query"} {
		t.Run(location, func(t *testing.T) {
			if location == "form" {
				if err := os.Unsetenv("GRAYBOX_REQUIRED_CREDENTIAL"); err != nil {
					t.Fatal(err)
				}
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(204) }))
			defer server.Close()
			matching := recording.Request{Method: "POST", URL: "/oauth", Headers: http.Header{}, Complete: true}
			mapping := "secret"
			switch location {
			case "json":
				mapping = "/secret"
				matching.Headers.Set("Content-Type", "application/json")
				matching.Body = []byte(`{"secret":"<REDACTED>"}`)
			case "form":
				matching.Headers.Set("Content-Type", "application/x-www-form-urlencoded")
				matching.Body = []byte("secret=%3CREDACTED%3E")
			case "query":
				matching.URL += "?secret=%3CREDACTED%3E"
			}
			path := hardeningRecording(t, server.URL, []recording.Request{{Method: "GET", URL: "/ordinary", Complete: true}, matching}, []recording.Response{{StatusCode: 204, Complete: true}, {StatusCode: 204, Complete: true}})
			cfg := writeHardeningConfig(t, "replay:\n  "+location+":\n    "+mapping+": '${GRAYBOX_REQUIRED_CREDENTIAL}'\n")
			for _, command := range []string{"replay", "diff"} {
				for _, jsonOutput := range []bool{false, true} {
					code, output := runHardeningCLI(t, command, path, cfg, jsonOutput)
					if code != ExitUsage || !strings.Contains(output, "GRAYBOX_REQUIRED_CREDENTIAL") || calls.Load() != 0 {
						t.Fatalf("missing credential %d %s", code, output)
					}
				}
			}
		})
	}
}

func TestUnsafeReplacementApplicabilityCLI(t *testing.T) {
	t.Setenv("GRAYBOX_UNSAFE_UNUSED", "")
	cfg := writeHardeningConfig(t, `replay:
  json:
    /credentials/key: "${GRAYBOX_UNSAFE_UNUSED}"
  form:
    secret: "${GRAYBOX_UNSAFE_UNUSED}"
  query:
    secret: "${GRAYBOX_UNSAFE_UNUSED}"
`)
	for _, tc := range []struct {
		name, kind, body, query, encoding string
		truncated, incomplete             bool
	}{
		{name: "malformed JSON", kind: "application/json", body: `{"credentials":{"key":"unsafe-original"}`},
		{name: "duplicate keys", kind: "application/json", body: `{"credentials":{"key":"unsafe-original"},"credentials":{}}`},
		{name: "hidden ancestor", kind: "application/json", body: `{"credentials":"<REDACTED>"}`},
		{name: "malformed form", kind: "application/x-www-form-urlencoded", body: "secret=unsafe-original%zz"},
		{name: "malformed query", query: "secret=unsafe-original%zz"},
		{name: "withheld query", query: "redacted=%3CREDACTED%3E"},
		{name: "binary", kind: "application/octet-stream", body: "\xffunsafe-original"},
		{name: "unsupported", kind: "text/plain", body: "unsafe-original"},
		{name: "missing type", body: "unsafe-original"},
		{name: "compressed", kind: "application/json", encoding: "gzip", body: "unsafe-original"},
		{name: "truncated", kind: "application/json", body: `{"credentials":{"key":"unsafe-original"}}`, truncated: true},
		{name: "incomplete", kind: "application/json", body: `{"credentials":{"key":"unsafe-original"}}`, incomplete: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1) }))
			defer server.Close()
			path := hardeningRecording(t, server.URL, []recording.Request{{Method: "POST", URL: "/oauth?" + tc.query, Headers: http.Header{"Content-Type": {tc.kind}, "Content-Encoding": {tc.encoding}}, Body: []byte(tc.body), Complete: !tc.incomplete, Truncated: tc.truncated}}, []recording.Response{{StatusCode: 200, Complete: true}})
			before, _ := os.ReadFile(path)
			for _, command := range []string{"replay", "diff"} {
				for _, jsonOutput := range []bool{false, true} {
					code, output := runHardeningCLI(t, command, path, cfg, jsonOutput)
					if code != ExitNetwork || calls.Load() != 0 || strings.Contains(output, "unsafe-original") {
						t.Fatalf("unsafe replay %d %s", code, output)
					}
					after, _ := os.ReadFile(path)
					if !bytes.Equal(before, after) {
						t.Fatal("failed execution modified recording")
					}
				}
			}
		})
	}
}

func TestDiffRedactionPolicyConsistencyCLI(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		capture, current       sanitize.Rules
		kind, oldBody, newBody string
		metadata               bool
	}{
		{name: "current redaction on old raw baseline", current: sanitize.Rules{Headers: []string{"X-Secret"}, JSON: []string{"/credentials/api_key", "/items/0/token"}}, kind: "application/json", oldBody: `{"credentials":{"api_key":"capture-secret"},"items":[{"token":"capture-token"}],"public":"ok"}`, newBody: `{"credentials":{"api_key":"live-secret"},"items":[{"token":"live-token"}],"public":"ok"}`},
		{name: "legacy marker inference without current rules", capture: sanitize.Rules{Headers: []string{"X-Secret"}, JSON: []string{"/credentials/api_key", "/items/0/token"}}, kind: "application/json", oldBody: `{"credentials":{"api_key":"capture-secret"},"items":[{"token":"capture-token"}],"public":"ok"}`, newBody: `{"credentials":{"api_key":"live-secret"},"items":[{"token":"live-token"}],"public":"ok"}`},
		{name: "different policies with protected ancestor", capture: sanitize.Rules{Headers: []string{"X-Secret"}, JSON: []string{"/credentials"}}, current: sanitize.Rules{JSON: []string{"/credentials/api_key", "/items/0/token"}}, kind: "application/json", oldBody: `{"credentials":{"api_key":"capture-secret"},"items":[{"token":"capture-token"}],"public":"ok"}`, newBody: `{"credentials":{"api_key":"live-secret"},"items":[{"token":"live-token"}],"public":"ok"}`, metadata: true},
		{name: "form marker inference", capture: sanitize.Rules{Headers: []string{"X-Secret"}, Form: []string{"secret"}}, kind: "application/x-www-form-urlencoded", oldBody: "secret=capture-secret&secret=capture-token&public=ok", newBody: "secret=live-secret&secret=live-token&public=ok"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.kind)
				w.Header().Set("X-Secret", "live-header-secret")
				w.Header().Set("Set-Cookie", "live-cookie-secret")
				_, _ = io.WriteString(w, tc.newBody)
			}))
			defer server.Close()
			headers := http.Header{"Content-Type": {tc.kind}, "X-Secret": {"capture-header-secret"}, "Set-Cookie": {"capture-cookie-secret"}}
			response := recording.Response{StatusCode: 200, Headers: tc.capture.ApplyHeaders(headers), Body: tc.capture.Body([]byte(tc.oldBody), headers, true), Complete: true}
			path := hardeningRecording(t, server.URL, []recording.Request{{Method: "GET", URL: "/resource", Complete: true}}, []recording.Response{response})
			if tc.metadata {
				addPolicyMetadata(t, path, tc.capture)
			}
			cfg := writeHardeningConfig(t, policyConfig(tc.current))
			before, _ := os.ReadFile(path)
			for _, jsonOutput := range []bool{false, true} {
				code, output := runHardeningCLI(t, "diff", path, cfg, jsonOutput)
				if code != ExitSuccess {
					t.Fatalf("policy-only change reported as regression: %d %s", code, output)
				}
				for _, secret := range []string{"capture-secret", "capture-token", "live-secret", "live-token", "capture-header-secret", "live-header-secret", "capture-cookie-secret", "live-cookie-secret"} {
					if strings.Contains(output, secret) {
						t.Fatal("comparison leaked a protected value")
					}
				}
				after, _ := os.ReadFile(path)
				if !bytes.Equal(before, after) {
					t.Fatal("normalizing baseline modified recording")
				}
			}
		})
	}
}

func policyConfig(rules sanitize.Rules) string {
	encoded, _ := json.Marshal(rules)
	return `{"redact":` + string(encoded) + `}`
}

func addPolicyMetadata(t *testing.T, path string, rules sanitize.Rules) {
	t.Helper()
	// Creating fixtures through Create already closed the writable connection;
	// use the public writable opener only for test setup.
	store, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	encoded, _ := json.Marshal(rules)
	if err := store.SetMetadata(context.Background(), "redaction_policy", string(encoded)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCapturePolicyProtectsNewLiveFieldsAndReportsLimitations(t *testing.T) {
	capture := sanitize.Rules{Headers: []string{"X-Future-Secret"}, JSON: []string{"/future/token"}, Query: []string{"access_token"}}
	for _, tc := range []struct {
		name, kind, body string
		want             int
		extra            []string
	}{
		{"new field", "application/json", `{"public":"ok","future":{"token":"future-sensitive-token"}}`, ExitBehaviorChanged, nil},
		{"malformed", "application/json", `{"future":{"token":"future-sensitive-token"}`, ExitComparisonFailed, nil},
		{"binary", "application/octet-stream", "\xfffuture-sensitive-token", ExitComparisonFailed, nil},
		{"ignored unsafe body", "application/octet-stream", "\xfffuture-sensitive-token", ExitBehaviorChanged, []string{"--ignore", "response.body"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.kind)
				w.Header().Set("X-Future-Secret", "future-sensitive-header")
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			path := hardeningRecording(t, server.URL, []recording.Request{{Method: "GET", URL: "/resource?access_token=original-query-sensitive", Complete: true}}, []recording.Response{{StatusCode: 200, Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"public":"ok"}`), Complete: true}})
			addPolicyMetadata(t, path, capture)
			cfg := writeHardeningConfig(t, "{}")
			before, _ := os.ReadFile(path)
			for _, jsonOutput := range []bool{false, true} {
				code, output := runHardeningCLI(t, "diff", path, cfg, jsonOutput, tc.extra...)
				if code != tc.want {
					t.Fatalf("comparison %d %s", code, output)
				}
				for _, secret := range []string{"future-sensitive-token", "future-sensitive-header", "original-query-sensitive"} {
					if strings.Contains(output, secret) {
						t.Fatal("capture policy was lost")
					}
				}
				if tc.want == ExitComparisonFailed && !strings.Contains(output, "withheld by redaction") {
					t.Fatal("missing redaction limitation")
				}
				after, _ := os.ReadFile(path)
				if !bytes.Equal(before, after) {
					t.Fatal("metadata/recording modified")
				}
			}
		})
	}
}

func TestCredentialOutputScrubBeforeTruncationAndSerialization(t *testing.T) {
	for _, secret := range []string{"runtime&/token~+\"value", "MiXeD-CaSe-CrEdEnTiAl", strings.Repeat("long-credential-part-", 50), "1234567890123456789", "true"} {
		t.Run(secret[:min(len(secret), 20)], func(t *testing.T) {
			t.Setenv("GRAYBOX_OUTPUT_HARDENING", secret)
			cfg := writeHardeningConfig(t, `replay:
  json:
    /token: "${GRAYBOX_OUTPUT_HARDENING}"
`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var request map[string]string
				_ = json.Unmarshal(body, &request)
				token := request["token"]
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Echo", token)
				if validHTTPHeaderName(token) {
					w.Header().Set("X-Echo-"+token, "ok")
				}
				data := map[string]any{"echo": token, "encoded": lowerPercentEscapes(url.QueryEscape(token)), "nested_json": string(body), token: "echo-key"}
				if token == "1234567890123456789" {
					data["numeric_echo"] = json.Number(token)
				}
				if token == "true" {
					data["boolean_echo"] = true
				}
				_ = json.NewEncoder(w).Encode(data)
			}))
			defer server.Close()
			path := hardeningRecording(t, server.URL, []recording.Request{{Method: "POST", URL: "/oauth", Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"token":"<REDACTED>"}`), Complete: true}}, []recording.Response{{StatusCode: 200, Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`), Complete: true}})
			before, _ := os.ReadFile(path)
			for _, jsonOutput := range []bool{false, true} {
				code, output := runHardeningCLI(t, "diff", path, cfg, jsonOutput)
				if code != ExitBehaviorChanged {
					t.Fatalf("%d %s", code, output)
				}
				// Metadata like duration numbers and booleans may coincidentally match a
				// short credential; inspect application data rather than the schema.
				if secret != "true" && (strings.Contains(output, secret) || strings.Contains(output, lowerPercentEscapes(url.QueryEscape(secret))) || strings.Contains(output, secret[:min(len(secret), 40)])) {
					t.Fatal("runtime credential or truncated prefix leaked")
				}
				if validHTTPHeaderName(secret) && len(secret) > 4 && strings.Contains(output, strings.ToLower(secret)) {
					t.Fatal("credential leaked through normalized header location")
				}
				if jsonOutput {
					var report diffReportJSON
					if json.Unmarshal([]byte(output), &report) != nil {
						t.Fatal("bad report")
					}
					for _, difference := range report.Results[0].Differences {
						if difference.Location.Path == "/boolean_echo" && difference.After.Value != sanitize.RedactedValue {
							t.Fatal("boolean credential leaked")
						}
						if difference.Location.Path == "/numeric_echo" && difference.After.Value != sanitize.RedactedValue {
							t.Fatal("numeric credential leaked")
						}
					}
				}
				after, _ := os.ReadFile(path)
				if !bytes.Equal(before, after) {
					t.Fatal("runtime value entered recording")
				}
			}
		})
	}
}

func TestRecordedPolicyValidationAndTransportErrors(t *testing.T) {
	cfg := writeHardeningConfig(t, `redact:
  query: [access_token]
`)
	// An invalid policy cannot silently remove capture-time protections.
	path := hardeningRecording(t, "http://localhost", []recording.Request{{Method: "GET", URL: "/resource", Complete: true}}, []recording.Response{{StatusCode: 200, Complete: true}})
	store, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetMetadata(context.Background(), "redaction_policy", `{"json":["invalid-sensitive-path"]}`); err != nil {
		t.Fatal(err)
	}
	store.Close()
	for _, command := range []string{"replay", "diff"} {
		code, output := runHardeningCLI(t, command, path, cfg, true)
		if code != ExitInvalidRecording || !strings.Contains(output, "invalid recorded redaction policy") || strings.Contains(output, "invalid-sensitive-path") {
			t.Fatalf("%d %s", code, output)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	target := server.URL
	server.Close()
	path = hardeningRecording(t, target, []recording.Request{{Method: "GET", URL: "/oauth?access_token=original-sensitive-query", Complete: true}}, []recording.Response{{StatusCode: 200, Complete: true}})
	for _, command := range []string{"replay", "diff"} {
		for _, jsonOutput := range []bool{false, true} {
			code, output := runHardeningCLI(t, command, path, cfg, jsonOutput)
			if code != ExitNetwork || strings.Contains(output, "original-sensitive-query") || !strings.Contains(output, "withheld by redaction policy") {
				t.Fatalf("transport leak %d %s", code, output)
			}
		}
	}
}
