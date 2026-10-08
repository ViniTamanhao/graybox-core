package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViniTamanhao/graybox-core/internal/capture"
	"github.com/ViniTamanhao/graybox-core/internal/config"
	"github.com/ViniTamanhao/graybox-core/internal/recording"
	"github.com/ViniTamanhao/graybox-core/internal/sanitize"
	"github.com/ViniTamanhao/graybox-core/internal/storage"
)

func TestConfigValidationAndPrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, body := range []string{
		"redact:\n  json: [credentials.key]", "redact:\n  json: ['/bad~2']", "redact:\n  headers: ['bad header']", "replay:\n  headers:\n    Host: '${TOKEN}'", "replay:\n  headers:\n    X-Key: '${TOKEN}'\n    x-key: '${TOKEN}'", "replay:\n  json:\n    /key: literal-secret", "replay:\n  json:\n    /key: '${BAD-NAME}'", "replay:\n  json:\n    /key: '${ONE}${TWO}'", "replay:\n  json:\n    /key: '${ONE}'\n    /key/child: '${TWO}'",
	} {
		if err := os.WriteFile("graybox.yaml", []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadConfig(""); err == nil || strings.Contains(err.Error(), "literal-secret") {
			t.Fatalf("unsafe config accepted/error: %v", err)
		}
	}
	t.Setenv("GRAYBOX_CONFIG_OVERRIDE", "Bearer cli-token")
	cfg := config.Config{Replay: config.Replay{Headers: map[string]string{"authorization": "Bearer ${GRAYBOX_UNUSED_MISSING}"}}}
	headers, _, _, err := resolveRequestConfig(cfg, []string{"Authorization=GRAYBOX_CONFIG_OVERRIDE"})
	if err != nil || headers.Get("Authorization") != "Bearer cli-token" || cfg.Replay.Headers["authorization"] != "Bearer ${GRAYBOX_UNUSED_MISSING}" {
		t.Fatalf("override: %v %v", headers, err)
	}
	for _, env := range []string{"GRAYBOX_CONFIG_ABSENT", "GRAYBOX_CONFIG_EMPTY"} {
		t.Setenv(env, "")
		cfg.Replay.JSON = map[string]string{"/secret": "${" + env + "}"}
		if _, _, _, err := resolveRequestConfig(cfg, nil); err == nil {
			t.Fatal("empty variable accepted")
		}
	}
	t.Setenv("GRAYBOX_CONFIG_BAD_HEADER", "bad\ncredential")
	cfg.Replay.JSON = nil
	cfg.Replay.Headers = map[string]string{"Authorization": "${GRAYBOX_CONFIG_BAD_HEADER}"}
	_, _, redactor, err := resolveRequestConfig(cfg, nil)
	if err == nil || strings.Contains(redactor.redactError(err).Error(), "credential") {
		t.Fatal("invalid header value accepted or leaked")
	}
}

func TestConfiguredOAuthRecordReplayDiff(t *testing.T) {
	for _, format := range []string{"json", "form"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			t.Setenv("GRAYBOX_CLIENT_SECRET", "runtime-client&+?secret")
			t.Setenv("GRAYBOX_REFRESH_TOKEN", "runtime-refresh/token")
			t.Setenv("GRAYBOX_QUERY_TOKEN", "runtime-query&+token")
			t.Setenv("GRAYBOX_CUSTOM_HEADER", "runtime-header-token")
			t.Setenv("GRAYBOX_CLI_AUTH", "Bearer cli-auth-token")
			configText := `redact:
  headers: [X-Custom-Secret]
  query: [access_token]
  json: [/client_secret, /refresh_token, /credentials/api_key, /items/0/token]
  form: [client_secret, refresh_token]
replay:
  headers:
    X-Custom-Secret: "${GRAYBOX_CUSTOM_HEADER}"
    Authorization: "Bearer ${GRAYBOX_UNUSED_MISSING}"
  query:
    access_token: "${GRAYBOX_QUERY_TOKEN}"
`
			if format == "json" {
				configText += `  json:
    /client_secret: "${GRAYBOX_CLIENT_SECRET}"
    /refresh_token: "${GRAYBOX_REFRESH_TOKEN}"
    /credentials/api_key: "${GRAYBOX_CUSTOM_HEADER}"
    /items/0/token: "${GRAYBOX_REFRESH_TOKEN}"
`
			} else {
				configText += `  form:
    client_secret: "${GRAYBOX_CLIENT_SECRET}"
    refresh_token: "${GRAYBOX_REFRESH_TOKEN}"
`
			}
			if err := os.WriteFile("graybox.yaml", []byte(configText), 0600); err != nil {
				t.Fatal(err)
			}
			_, rules, err := loadConfig("")
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				values := map[string]string{}
				if format == "json" {
					var object map[string]any
					if err := json.Unmarshal(raw, &object); err != nil {
						t.Error(err)
						http.Error(w, "invalid JSON", 400)
						return
					}
					values["client_secret"], _ = object["client_secret"].(string)
					values["refresh_token"], _ = object["refresh_token"].(string)
					if object["client_id"] != "my-app" || object["number"] != float64(17) {
						t.Error("unrelated JSON values changed")
					}
					values["nested"] = object["credentials"].(map[string]any)["api_key"].(string)
					values["array"] = object["items"].([]any)[0].(map[string]any)["token"].(string)
				} else {
					form, err := url.ParseQuery(string(raw))
					if err != nil {
						t.Error(err)
					}
					values["client_secret"] = form.Get("client_secret")
					values["refresh_token"] = form.Get("refresh_token")
					if form.Get("client_id") != "my-app" {
						t.Error("form changed")
					}
				}
				if r.ContentLength != int64(len(raw)) {
					t.Error("wrong content length")
				}
				n := calls.Add(1)
				if n == 1 {
					if values["client_secret"] != "original-secret" || values["refresh_token"] != "original-token" || r.URL.Query().Get("access_token") != "original-query" || r.Header.Get("X-Custom-Secret") != "original-header" {
						t.Error("redaction changed proxied traffic")
					}
				} else {
					if values["client_secret"] != os.Getenv("GRAYBOX_CLIENT_SECRET") || values["refresh_token"] != os.Getenv("GRAYBOX_REFRESH_TOKEN") || r.URL.Query().Get("access_token") != os.Getenv("GRAYBOX_QUERY_TOKEN") || r.Header.Get("X-Custom-Secret") != os.Getenv("GRAYBOX_CUSTOM_HEADER") || r.Header.Get("Authorization") != os.Getenv("GRAYBOX_CLI_AUTH") {
						t.Error("runtime replacements missing")
					}
					if format == "json" && (values["nested"] != os.Getenv("GRAYBOX_CUSTOM_HEADER") || values["array"] != os.Getenv("GRAYBOX_REFRESH_TOKEN")) {
						t.Error("nested replacements missing")
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Custom-Secret", values["client_secret"])
				response := map[string]string{"client_secret": values["client_secret"], "refresh_token": values["refresh_token"], "public": "ok"}
				if n > 1 {
					response["echo"] = values["client_secret"] + " " + r.URL.RawQuery + " " + r.Header.Get("Authorization")
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			target, _ := url.Parse(server.URL)
			path := filepath.Join(dir, "session.graybox")
			store, err := storage.Create(context.Background(), path, "v0.2.2")
			if err != nil {
				t.Fatal(err)
			}
			if err := store.SetMetadata(context.Background(), "target_url", server.URL); err != nil {
				t.Fatal(err)
			}
			proxy := capture.NewProxy(target, store, capture.ErrorHandlers{Transport: func(err error) { t.Error(err) }, Persistence: func(err error) { t.Error(err) }})
			proxy.SetRedaction(rules)
			finished := make(chan struct{})
			recorder := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxy.ServeHTTP(w, r); close(finished) }))
			body := `{"client_id":"my-app","client_secret":"original-secret","refresh_token":"original-token","credentials":{"api_key":"original-nested"},"items":[{"token":"original-array"}],"number":17}`
			contentType := "application/json"
			if format == "form" {
				body = "client_id=my-app&client_secret=original-secret&refresh_token=original-token"
				contentType = "application/x-www-form-urlencoded"
			}
			req, _ := http.NewRequest("POST", recorder.URL+"/oauth?access_token=original-query&scope=a%2Bb", strings.NewReader(body))
			req.Header.Set("Content-Type", contentType)
			req.Header.Set("X-Custom-Secret", "original-header")
			req.Header.Set("Authorization", "Bearer original-auth")
			response, err := recorder.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			<-finished
			recorder.Close()
			exchange, err := store.Get(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(exchange.Request.Body, []byte(sanitize.RedactedValue)) && format == "json" {
				t.Fatal("body not redacted")
			}
			if exchange.Request.Headers.Get("Authorization") != sanitize.RedactedValue || exchange.Request.Headers.Get("X-Custom-Secret") != sanitize.RedactedValue {
				t.Fatal("header not redacted")
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"original-secret", "original-token", "original-query", "original-header", "original-auth", "original-nested", "original-array"} {
				if bytes.Contains(before, []byte(secret)) {
					t.Fatalf("recording leaked configured %s", secret)
				}
			}
			for _, command := range []string{"replay", "diff"} {
				for _, jsonOutput := range []bool{false, true} {
					var stdout, stderr bytes.Buffer
					args := []string{command, path, "--secret-header", "Authorization=GRAYBOX_CLI_AUTH"}
					if jsonOutput {
						args = append(args, "--json")
					}
					if command == "diff" {
						args = append(args, "--config", filepath.Join(dir, "graybox.yaml"))
					}
					code := (App{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), args)
					expected := ExitSuccess
					if command == "diff" {
						expected = ExitBehaviorChanged
					}
					if code != expected || stderr.Len() != 0 {
						t.Fatalf("%s: code %d stderr %s stdout %s", command, code, &stderr, &stdout)
					}
					if jsonOutput && !json.Valid(stdout.Bytes()) {
						t.Fatal("invalid JSON output")
					}
					output := stdout.String() + stderr.String()
					for _, name := range []string{"GRAYBOX_CLIENT_SECRET", "GRAYBOX_REFRESH_TOKEN", "GRAYBOX_QUERY_TOKEN", "GRAYBOX_CUSTOM_HEADER", "GRAYBOX_CLI_AUTH"} {
						secret := os.Getenv(name)
						if strings.Contains(output, secret) || strings.Contains(output, url.QueryEscape(secret)) {
							t.Fatal("runtime credential leaked")
						}
					}
					after, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(before, after) {
						t.Fatal("recording modified")
					}
				}
			}
			if calls.Load() != 5 {
				t.Fatal("replay/diff did not send identical number of requests")
			}
		})
	}
}

func TestConfigMissingVariablePreventsExecution(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("graybox.yaml", []byte("replay:\n  json:\n    /secret: '${GRAYBOX_V022_MISSING}'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRAYBOX_V022_MISSING", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Error("missing credential sent a request") }))
	defer server.Close()
	path := hardeningRecording(t, server.URL, []recording.Request{{Method: "POST", URL: "/oauth", Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"secret":"<REDACTED>"}`), Complete: true}}, []recording.Response{{StatusCode: 200, Body: []byte(`{}`), Complete: true}})
	for _, command := range []string{"replay", "diff"} {
		var stdout, stderr bytes.Buffer
		code := (App{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), []string{command, path, "--json"})
		if code != ExitUsage || !strings.Contains(stderr.String(), "GRAYBOX_V022_MISSING") || stdout.Len() != 0 {
			t.Fatalf("%d %s", code, &stderr)
		}
	}
}

func TestConfiguredRecordCommandDoesNotResolveRuntimeSecrets(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("graybox.yaml", []byte("redact:\n  json: [/client_secret]\nreplay:\n  json:\n    /client_secret: '${GRAYBOX_RECORD_UNUSED}'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRAYBOX_RECORD_UNUSED", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.Copy(w, r.Body)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan net.Listener, 1)
	var stdout, stderr bytes.Buffer
	app := App{Stdout: &stdout, Stderr: &stderr, listen: func(network, address string) (net.Listener, error) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err == nil {
			ready <- listener
		}
		return listener, err
	}}
	done := make(chan int, 1)
	go func() {
		done <- app.Run(ctx, []string{"record", "--target", server.URL, "--output", "record.graybox", "--config", "graybox.yaml"})
	}()
	var listener net.Listener
	select {
	case listener = <-ready:
	case code := <-done:
		t.Fatalf("record failed before startup: %d %s", code, &stderr)
	}
	req, _ := http.NewRequest("POST", "http://"+listener.Addr().String()+"/oauth", strings.NewReader(`{"client_secret":"record-original-secret","client_id":"my-app"}`))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	cancel()
	if code := <-done; code != ExitSuccess {
		t.Fatalf("record code %d stderr %s", code, &stderr)
	}
	store, err := storage.OpenReadOnly(context.Background(), "record.graybox")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exchange, err := store.Get(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := store.Metadata(context.Background(), "redaction_policy")
	if err != nil || !strings.Contains(policy, "/client_secret") || strings.Contains(policy, "GRAYBOX_RECORD_UNUSED") || strings.Contains(policy, "record-original-secret") {
		t.Fatal("capture policy missing or contains execution configuration")
	}
	for _, body := range [][]byte{exchange.Request.Body, exchange.Response.Body} {
		if bytes.Contains(body, []byte("record-original-secret")) || !bytes.Contains(body, []byte(sanitize.RedactedValue)) {
			t.Fatal("record command did not apply configuration")
		}
	}
}

func TestRuntimeSecretScrubbingIncludesJSONPointerLocations(t *testing.T) {
	secret := "runtime/slash~token&+value"
	t.Setenv("GRAYBOX_POINTER_SECRET", secret)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{r.Header.Get("Authorization"): "echo"})
	}))
	defer server.Close()
	path := createSecretHeaderTestRecording(t, server.URL, nil, recording.Response{StatusCode: 200, Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`), ObservedSize: 2, Complete: true})
	escaped := strings.ReplaceAll(strings.ReplaceAll(secret, "~", "~0"), "/", "~1")
	for _, jsonOutput := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		args := []string{"diff", path, "--secret-header", "Authorization=GRAYBOX_POINTER_SECRET"}
		if jsonOutput {
			args = append(args, "--json")
		}
		code := (App{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), args)
		if code != ExitBehaviorChanged || stderr.Len() != 0 {
			t.Fatalf("%d %s", code, &stderr)
		}
		if strings.Contains(stdout.String(), secret) || strings.Contains(stdout.String(), escaped) {
			t.Fatal("credential leaked through diff location")
		}
		if jsonOutput && !json.Valid(stdout.Bytes()) {
			t.Fatal("invalid JSON")
		}
	}
}
