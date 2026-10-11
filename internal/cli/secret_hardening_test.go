package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
)

func TestDiffRedactsApplicationObjectKeys(t *testing.T) {
	for _, secret := range []string{"Bearer object-key-secret", "1", "value", "results", "path", "error", "summary"} {
		t.Run(secret, func(t *testing.T) {
			t.Setenv("GRAYBOX_KEY_SECRET", secret)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				token := r.Header.Get("Authorization")
				_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{token: "visible value", "prefix-" + token + "-suffix": "embedded"}}, "scalars": []any{1, true, nil, token}})
			}))
			defer server.Close()
			baseline := []byte(`{}`)
			path := createSecretHeaderTestRecording(t, server.URL, nil, recording.Response{StatusCode: 200, Headers: http.Header{"Content-Type": {"application/json"}}, Body: baseline, ObservedSize: int64(len(baseline)), Complete: true})
			var stdout, stderr bytes.Buffer
			code := (App{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), []string{"diff", path, "--json", "--secret-header", "Authorization=GRAYBOX_KEY_SECRET"})
			if code != ExitBehaviorChanged || stderr.Len() != 0 {
				t.Fatalf("code=%d stderr=%s stdout=%s", code, &stderr, &stdout)
			}
			if secret == "Bearer object-key-secret" && strings.Contains(stdout.String(), secret) {
				t.Fatal("secret leaked")
			}
			var report diffReportJSON
			if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if report.Summary.Changed != 1 || report.Summary.Failed != 0 || len(report.Results) != 1 {
				t.Fatalf("bad summary: %+v", report)
			}
			result := report.Results[0]
			if result.ExchangeID != 1 || result.Outcome != "changed" || result.Path != "/resource" || len(result.Differences) != 2 {
				t.Fatalf("bad result: %+v", result)
			}
			for _, difference := range result.Differences {
				if difference.Kind != "field_added" || difference.Before.Present || !difference.After.Present {
					t.Fatalf("bad difference: %+v", difference)
				}
				switch difference.Location.Path {
				case "/items":
					object := difference.After.Value.([]any)[0].(map[string]any)
					if !reflect.DeepEqual(object, map[string]any{"<REDACTED>": strings.ReplaceAll("visible value", secret, "<REDACTED>"), "prefix-<REDACTED>-suffix": "embedded"}) {
						t.Fatalf("bad keys: %#v", object)
					}
				case "/scalars":
					expectedNumber := any(float64(1))
					if secret == "1" {
						expectedNumber = "<REDACTED>"
					}
					if !reflect.DeepEqual(difference.After.Value, []any{expectedNumber, true, nil, "<REDACTED>"}) {
						t.Fatalf("bad scalars: %#v", difference.After.Value)
					}
				default:
					t.Fatalf("bad location: %+v", difference.Location)
				}
			}
			var schema map[string]json.RawMessage
			if err := json.Unmarshal(stdout.Bytes(), &schema); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"results", "summary", "recording", "target", "ignored", "body_capture_limit"} {
				if _, ok := schema[key]; !ok {
					t.Fatalf("missing schema key %q", key)
				}
			}
		})
	}
}

func FuzzStructuredSecretRedaction(f *testing.F) {
	for _, seed := range []string{`{"1":[1,true,null,"1"]}`, `{"prefix-Bearer object-key-secret-suffix":"Bearer object-key-secret"}`, `{"value":{"results":"value"}}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		decoder := json.NewDecoder(strings.NewReader(input))
		decoder.UseNumber()
		var value any
		if decoder.Decode(&value) != nil {
			return
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return
		}
		redactor := newSecretRedactor([]string{"1", "Bearer object-key-secret", "value", "results"})
		original, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		redacted := redactor.redactData(value)
		var check func(any, any)
		check = func(before, after any) {
			switch typed := before.(type) {
			case string:
				if after != redactor.redact(typed) {
					t.Fatal("string not scrubbed")
				}
			case map[string]any:
				counts := make(map[string]int)
				for key := range typed {
					counts[redactor.redact(key)]++
				}
				for key, item := range typed {
					if counts[redactor.redact(key)] == 1 {
						check(item, after.(map[string]any)[redactor.redact(key)])
					}
				}
			case []any:
				for i, item := range typed {
					check(item, after.([]any)[i])
				}
			case json.Number:
				expected := any(typed)
				if redactor.redact(string(typed)) != string(typed) {
					expected = "<REDACTED>"
				}
				if !reflect.DeepEqual(expected, after) {
					t.Fatal("numeric credential not scrubbed")
				}
			default:
				if !reflect.DeepEqual(before, after) {
					t.Fatal("scalar changed")
				}
			}
		}
		// Key collisions intentionally collapse output entries; check values only
		// when the mapping is injective, and always inspect all output strings/keys.
		var inspect func(any)
		inspect = func(v any) {
			switch x := v.(type) {
			case string:
				if strings.Contains(x, "Bearer object-key-secret") || strings.Contains(x, "1") || strings.Contains(x, "value") || strings.Contains(x, "results") {
					t.Fatal("secret leaked")
				}
			case map[string]any:
				for k, v := range x {
					inspect(k)
					inspect(v)
				}
			case []any:
				for _, v := range x {
					inspect(v)
				}
			}
		}
		inspect(redacted)
		check(value, redacted)
		current, _ := json.Marshal(value)
		if !bytes.Equal(original, current) {
			t.Fatal("source mutated")
		}
		var output bytes.Buffer
		if err := redactor.writeJSONOutput(&output, func(w io.Writer) error {
			return writeJSON(w, map[string]any{"results": redacted, "exchange_id": 1, "value": true, "summary": nil})
		}); err != nil {
			t.Fatal(err)
		}
		var schema map[string]json.RawMessage
		if json.Unmarshal(output.Bytes(), &schema) != nil || string(schema["exchange_id"]) != "1" || string(schema["value"]) != "true" || string(schema["summary"]) != "null" || schema["results"] == nil {
			t.Fatalf("invalid schema: %s", &output)
		}
	})
}

func FuzzJSONPointer(f *testing.F) {
	for _, seed := range []string{"/", "/a~0b/~1", "/bad~", "/bad~2", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, pointer string) {
		valid := strings.HasPrefix(pointer, "/")
		for _, token := range strings.Split(pointer, "~")[1:] {
			if len(token) == 0 || (token[0] != '0' && token[0] != '1') {
				valid = false
			}
		}
		if validJSONPointer(pointer) != valid {
			t.Fatalf("pointer validation mismatch: %q", pointer)
		}
	})
}

func TestStructuredSecretRedactionCopiesDataAndHandlesCollisions(t *testing.T) {
	redactor := newSecretRedactor([]string{"secret"})
	source := map[string]any{"secret": []any{"secret", json.Number("1"), true, nil}, "<REDACTED>": "collision"}
	before, _ := json.Marshal(source)
	got := redactor.redactData(source)
	want := map[string]any{"<REDACTED>": []any{"<REDACTED>", json.Number("1"), true, nil}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	after, _ := json.Marshal(source)
	if !bytes.Equal(before, after) {
		t.Fatal("source mutated")
	}
	for _, value := range []any{map[string]any(nil), []any(nil), nil} {
		if !reflect.DeepEqual(value, redactor.redactData(value)) {
			t.Fatal("nil changed")
		}
	}
}

func TestRuntimeSecretRedactsReplayOutputAndCommandErrors(t *testing.T) {
	const secret = "runtime-output-secret"
	t.Setenv("GRAYBOX_OUTPUT_SECRET", secret)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	path := createSecretHeaderTestRecording(t, server.URL, nil, recording.Response{StatusCode: http.StatusNoContent, Complete: true})
	for _, jsonOutput := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		args := []string{"replay", path, "--target", server.URL + "/" + secret, "--secret-header", "Authorization=GRAYBOX_OUTPUT_SECRET"}
		if jsonOutput {
			args = append(args, "--json")
		}
		code := (App{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), args)
		if code != ExitSuccess || stderr.Len() != 0 || strings.Contains(stdout.String(), secret) || !strings.Contains(stdout.String(), "<REDACTED>") {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
		}
		if jsonOutput && !json.Valid(stdout.Bytes()) {
			t.Fatal("invalid JSON")
		}
	}
	for _, command := range []string{"replay", "diff"} {
		var stdout, stderr bytes.Buffer
		code := (App{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), []string{command, t.TempDir() + "/" + secret + ".graybox", "--secret-header", "Authorization=GRAYBOX_OUTPUT_SECRET"})
		if code == ExitSuccess || strings.Contains(stderr.String(), secret) || !strings.Contains(stderr.String(), "<REDACTED>") {
			t.Fatalf("code=%d stderr=%s", code, &stderr)
		}
	}
}
