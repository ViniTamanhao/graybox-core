package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestResolveSecretHeaders(
	t *testing.T,
) {
	t.Setenv(
		"GRAYBOX_TEST_AUTH",
		"Bearer abc123",
	)

	t.Setenv(
		"GRAYBOX_TEST_API_KEY",
		"api-key-456",
	)

	headers, redactor, err := resolveSecretHeaders(
		[]string{
			"Authorization=GRAYBOX_TEST_AUTH",
			"X-API-Key=GRAYBOX_TEST_API_KEY",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if got := headers.Get(
		"Authorization",
	); got != "Bearer abc123" {
		t.Fatalf(
			"Authorization = %q",
			got,
		)
	}

	if got := headers.Get(
		"X-API-Key",
	); got != "api-key-456" {
		t.Fatalf(
			"X-API-Key = %q",
			got,
		)
	}

	output := redactor.redact(
		"authorization=Bearer abc123 key=api-key-456",
	)

	if strings.Contains(
		output,
		"Bearer abc123",
	) ||
		strings.Contains(
			output,
			"api-key-456",
		) {
		t.Fatalf(
			"redacted output leaked a secret: %q",
			output,
		)
	}

	if !strings.Contains(
		output,
		sanitize.RedactedValue,
	) {
		t.Fatalf(
			"redacted output = %q",
			output,
		)
	}
}

func TestParseSecretHeaderMappingsRejectsInvalidMappings(
	t *testing.T,
) {
	tests := []struct {
		name   string
		values []string
	}{
		{
			name: "missing separator",
			values: []string{
				"Authorization",
			},
		},
		{
			name: "empty header",
			values: []string{
				"=API_AUTH",
			},
		},
		{
			name: "empty environment variable",
			values: []string{
				"Authorization=",
			},
		},
		{
			name: "invalid header name",
			values: []string{
				"Bad Header=API_AUTH",
			},
		},
		{
			name: "invalid environment variable name",
			values: []string{
				"Authorization=BAD-NAME",
			},
		},
		{
			name: "host",
			values: []string{
				"Host=API_AUTH",
			},
		},
		{
			name: "content length",
			values: []string{
				"Content-Length=API_AUTH",
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(
				t *testing.T,
			) {
				_, err := parseSecretHeaderMappings(
					test.values,
				)

				if err == nil {
					t.Fatal(
						"error = nil",
					)
				}

				var usage usageError

				if !errors.As(
					err,
					&usage,
				) {
					t.Fatalf(
						"error type = %T, want usageError",
						err,
					)
				}
			},
		)
	}
}

func TestParseSecretHeaderMappingsRejectsCaseInsensitiveDuplicates(
	t *testing.T,
) {
	_, err := parseSecretHeaderMappings(
		[]string{
			"Authorization=FIRST_AUTH",
			"authorization=SECOND_AUTH",
		},
	)

	if err == nil {
		t.Fatal(
			"error = nil",
		)
	}

	var usage usageError

	if !errors.As(
		err,
		&usage,
	) {
		t.Fatalf(
			"error type = %T, want usageError",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"duplicate",
	) {
		t.Fatalf(
			"error = %q",
			err,
		)
	}
}

func TestResolveSecretHeadersRejectsMissingAndEmptyEnvironmentVariables(
	t *testing.T,
) {
	const missingVariable = "GRAYBOX_TEST_SECRET_DOES_NOT_EXIST"

	_ = os.Unsetenv(
		missingVariable,
	)

	t.Setenv(
		"GRAYBOX_TEST_EMPTY_SECRET",
		"",
	)

	tests := []struct {
		name    string
		mapping string
		want    string
	}{
		{
			name: "missing",
			mapping: "Authorization=" +
				missingVariable,
			want: "is not set",
		},
		{
			name: "empty",
			mapping: "Authorization=" +
				"GRAYBOX_TEST_EMPTY_SECRET",
			want: "is empty",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(
				t *testing.T,
			) {
				_, _, err := resolveSecretHeaders(
					[]string{
						test.mapping,
					},
				)

				if err == nil {
					t.Fatal(
						"error = nil",
					)
				}

				var usage usageError

				if !errors.As(
					err,
					&usage,
				) {
					t.Fatalf(
						"error type = %T, want usageError",
						err,
					)
				}

				if !strings.Contains(
					err.Error(),
					test.want,
				) {
					t.Fatalf(
						"error = %q, want %q",
						err,
						test.want,
					)
				}
			},
		)
	}
}

func TestSecretRedactorHandlesHumanJSONAndErrors(
	t *testing.T,
) {
	const secret = `Bearer a"b&c\token`

	redactor := newSecretRedactor(
		[]string{
			secret,
		},
	)

	human := redactor.redact(
		"request failed with " + secret,
	)

	if strings.Contains(
		human,
		secret,
	) {
		t.Fatalf(
			"human output leaked secret: %q",
			human,
		)
	}

	var output bytes.Buffer

	err := redactor.writeJSONOutput(
		&output,
		func(
			writer io.Writer,
		) error {
			return json.NewEncoder(
				writer,
			).Encode(
				map[string]string{
					"value": secret,
				},
			)
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if !json.Valid(
		output.Bytes(),
	) {
		t.Fatalf(
			"JSON is invalid after redaction: %q",
			output.String(),
		)
	}

	if strings.Contains(
		output.String(),
		secret,
	) {
		t.Fatalf(
			"JSON output leaked raw secret: %q",
			output.String(),
		)
	}

	var decoded map[string]string

	if err := json.Unmarshal(
		output.Bytes(),
		&decoded,
	); err != nil {
		t.Fatalf(
			"decode redacted JSON: %v\n%s",
			err,
			output.String(),
		)
	}

	if got := decoded["value"]; got != sanitize.RedactedValue {
		t.Fatalf(
			"redacted JSON value = %q, want %q",
			got,
			sanitize.RedactedValue,
		)
	}

	baseError := errors.New(
		"simulated request failure",
	)

	wrapped := redactor.redactError(
		fmt.Errorf(
			"%w using %s",
			baseError,
			secret,
		),
	)

	if strings.Contains(
		wrapped.Error(),
		secret,
	) {
		t.Fatalf(
			"error leaked secret: %q",
			wrapped,
		)
	}

	if !errors.Is(
		wrapped,
		baseError,
	) {
		t.Fatal(
			"redacted error did not preserve its error chain",
		)
	}
}

func TestSecretRedactorJSONOnlyRedactsStringValues(
	t *testing.T,
) {
	const secret = "1"

	redactor := newSecretRedactor(
		[]string{
			secret,
		},
	)

	var output bytes.Buffer

	err := redactor.writeJSONOutput(
		&output,
		func(
			writer io.Writer,
		) error {
			return json.NewEncoder(
				writer,
			).Encode(
				map[string]any{
					"exchange_id": 1,
					"total":       1,
					"successful":  true,
					"missing":     nil,
					"echo":        secret,
					"message":     "token=1",
				},
			)
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if !json.Valid(
		output.Bytes(),
	) {
		t.Fatalf(
			"JSON became invalid after redaction: %q",
			output.String(),
		)
	}

	var decoded struct {
		ExchangeID int    `json:"exchange_id"`
		Total      int    `json:"total"`
		Successful bool   `json:"successful"`
		Missing    any    `json:"missing"`
		Echo       string `json:"echo"`
		Message    string `json:"message"`
	}

	if err := json.Unmarshal(
		output.Bytes(),
		&decoded,
	); err != nil {
		t.Fatalf(
			"decode JSON: %v\n%s",
			err,
			output.String(),
		)
	}

	if decoded.ExchangeID != 1 {
		t.Fatalf(
			"exchange_id = %d, want 1",
			decoded.ExchangeID,
		)
	}

	if decoded.Total != 1 {
		t.Fatalf(
			"total = %d, want 1",
			decoded.Total,
		)
	}

	if !decoded.Successful {
		t.Fatal(
			"successful = false, want true",
		)
	}

	if decoded.Missing != nil {
		t.Fatalf(
			"missing = %#v, want nil",
			decoded.Missing,
		)
	}

	if decoded.Echo != sanitize.RedactedValue {
		t.Fatalf(
			"echo = %q, want %q",
			decoded.Echo,
			sanitize.RedactedValue,
		)
	}

	wantMessage := "token=" +
		sanitize.RedactedValue

	if decoded.Message != wantMessage {
		t.Fatalf(
			"message = %q, want %q",
			decoded.Message,
			wantMessage,
		)
	}
}

func TestReplayInjectsSecretHeadersAndDoesNotPersistThem(
	t *testing.T,
) {
	const authorization = "Bearer replay-runtime-secret"
	const apiKey = "runtime-api-key"

	var receivedAuthorization string
	var receivedAPIKey string

	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				receivedAuthorization = r.Header.Get(
					"Authorization",
				)

				receivedAPIKey = r.Header.Get(
					"X-API-Key",
				)

				w.WriteHeader(
					http.StatusNoContent,
				)
			},
		),
	)
	defer server.Close()

	recordedHeaders := make(
		http.Header,
	)

	recordedHeaders.Set(
		"Authorization",
		sanitize.RedactedValue,
	)

	recordedHeaders.Set(
		"X-API-Key",
		"recorded-api-key",
	)

	path := createSecretHeaderTestRecording(
		t,
		server.URL,
		recordedHeaders,
		recording.Response{
			StatusCode: http.StatusNoContent,
			Complete:   true,
		},
	)

	t.Setenv(
		"GRAYBOX_REPLAY_AUTH",
		authorization,
	)

	t.Setenv(
		"GRAYBOX_REPLAY_API_KEY",
		apiKey,
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := (App{
		Stdout: &stdout,
		Stderr: &stderr,
	}).Run(
		context.Background(),
		[]string{
			"replay",
			path,
			"--secret-header",
			"Authorization=GRAYBOX_REPLAY_AUTH",
			"--secret-header",
			"X-API-Key=GRAYBOX_REPLAY_API_KEY",
		},
	)

	if code != ExitSuccess {
		t.Fatalf(
			"exit = %d, stdout = %q, stderr = %q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}

	if receivedAuthorization != authorization {
		t.Fatalf(
			"Authorization = %q",
			receivedAuthorization,
		)
	}

	if receivedAPIKey != apiKey {
		t.Fatalf(
			"X-API-Key = %q",
			receivedAPIKey,
		)
	}

	for _, secret := range []string{
		authorization,
		apiKey,
	} {
		if strings.Contains(
			stdout.String(),
			secret,
		) ||
			strings.Contains(
				stderr.String(),
				secret,
			) {
			t.Fatalf(
				"secret %q leaked into output",
				secret,
			)
		}
	}

	store, err := storage.OpenReadOnly(
		context.Background(),
		path,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	exchange, err := store.Get(
		context.Background(),
		1,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got := exchange.Request.Headers.Get(
		"Authorization",
	); got != sanitize.RedactedValue {
		t.Fatalf(
			"persisted Authorization = %q",
			got,
		)
	}

	if got := exchange.Request.Headers.Get(
		"X-API-Key",
	); got != "recorded-api-key" {
		t.Fatalf(
			"persisted X-API-Key = %q",
			got,
		)
	}
}

func TestReplaySecretHeaderUsageErrorsSendNoRequest(
	t *testing.T,
) {
	var requestCount atomic.Int64

	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				_ *http.Request,
			) {
				requestCount.Add(
					1,
				)

				w.WriteHeader(
					http.StatusNoContent,
				)
			},
		),
	)
	defer server.Close()

	path := createSecretHeaderTestRecording(
		t,
		server.URL,
		nil,
		recording.Response{
			StatusCode: http.StatusNoContent,
			Complete:   true,
		},
	)

	const missingVariable = "GRAYBOX_TEST_MISSING_RUNTIME_SECRET"

	_ = os.Unsetenv(
		missingVariable,
	)

	t.Setenv(
		"GRAYBOX_EMPTY_RUNTIME_SECRET",
		"",
	)

	t.Setenv(
		"GRAYBOX_DUPLICATE_ONE",
		"first-secret",
	)

	t.Setenv(
		"GRAYBOX_DUPLICATE_TWO",
		"second-secret",
	)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing environment variable",
			args: []string{
				"--secret-header",
				"Authorization=" + missingVariable,
			},
			want: "is not set",
		},
		{
			name: "empty environment variable",
			args: []string{
				"--secret-header",
				"Authorization=GRAYBOX_EMPTY_RUNTIME_SECRET",
			},
			want: "is empty",
		},
		{
			name: "duplicate mapping",
			args: []string{
				"--secret-header",
				"Authorization=GRAYBOX_DUPLICATE_ONE",
				"--secret-header",
				"authorization=GRAYBOX_DUPLICATE_TWO",
			},
			want: "duplicate",
		},
		{
			name: "malformed mapping",
			args: []string{
				"--secret-header",
				"Authorization",
			},
			want: "expected HEADER=ENV_VAR",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(
				t *testing.T,
			) {
				var stdout bytes.Buffer
				var stderr bytes.Buffer

				args := []string{
					"replay",
					path,
				}

				args = append(
					args,
					test.args...,
				)

				code := (App{
					Stdout: &stdout,
					Stderr: &stderr,
				}).Run(
					context.Background(),
					args,
				)

				if code != ExitUsage {
					t.Fatalf(
						"exit = %d, stdout = %q, stderr = %q",
						code,
						stdout.String(),
						stderr.String(),
					)
				}

				if stdout.Len() != 0 {
					t.Fatalf(
						"stdout = %q, want empty",
						stdout.String(),
					)
				}

				if !strings.Contains(
					stderr.String(),
					test.want,
				) {
					t.Fatalf(
						"stderr = %q, want %q",
						stderr.String(),
						test.want,
					)
				}
			},
		)
	}

	if got := requestCount.Load(); got != 0 {
		t.Fatalf(
			"server received %d requests, want 0",
			got,
		)
	}
}

func TestDiffInjectsAuthorizationSecretHeader(
	t *testing.T,
) {
	const authorization = "Bearer diff-runtime-secret"

	var receivedAuthorization string

	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				receivedAuthorization = r.Header.Get(
					"Authorization",
				)

				w.Header().Set(
					"Content-Type",
					"application/json",
				)

				if receivedAuthorization != authorization {
					w.WriteHeader(
						http.StatusUnauthorized,
					)

					_, _ = io.WriteString(
						w,
						`{"ok":false}`,
					)

					return
				}

				w.WriteHeader(
					http.StatusOK,
				)

				_, _ = io.WriteString(
					w,
					`{"ok":true}`,
				)
			},
		),
	)
	defer server.Close()

	requestHeaders := make(
		http.Header,
	)

	requestHeaders.Set(
		"Authorization",
		sanitize.RedactedValue,
	)

	responseHeaders := make(
		http.Header,
	)

	responseHeaders.Set(
		"Content-Type",
		"application/json",
	)

	path := createSecretHeaderTestRecording(
		t,
		server.URL,
		requestHeaders,
		recording.Response{
			StatusCode: http.StatusOK,
			Headers:    responseHeaders,
			Body: []byte(
				`{"ok":true}`,
			),
			ObservedSize: int64(
				len(`{"ok":true}`),
			),
			Complete: true,
		},
	)

	t.Setenv(
		"GRAYBOX_DIFF_AUTH",
		authorization,
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := (App{
		Stdout: &stdout,
		Stderr: &stderr,
	}).Run(
		context.Background(),
		[]string{
			"diff",
			path,
			"--secret-header",
			"Authorization=GRAYBOX_DIFF_AUTH",
			"--json",
		},
	)

	if code != ExitSuccess {
		t.Fatalf(
			"exit = %d, stdout = %q, stderr = %q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}

	if receivedAuthorization != authorization {
		t.Fatalf(
			"Authorization = %q",
			receivedAuthorization,
		)
	}

	if strings.Contains(
		stdout.String(),
		authorization,
	) ||
		strings.Contains(
			stderr.String(),
			authorization,
		) {
		t.Fatalf(
			"diff output leaked secret: stdout=%q stderr=%q",
			stdout.String(),
			stderr.String(),
		)
	}

	if !json.Valid(
		stdout.Bytes(),
	) {
		t.Fatalf(
			"invalid JSON output: %q",
			stdout.String(),
		)
	}
}

func TestDiffRedactsEchoedRuntimeSecretFromHumanAndJSONOutput(
	t *testing.T,
) {
	const authorization = `Bearer diff-"echo"&secret`

	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				w.Header().Set(
					"Content-Type",
					"application/json",
				)

				_ = json.NewEncoder(
					w,
				).Encode(
					map[string]string{
						"echo": r.Header.Get(
							"Authorization",
						),
					},
				)
			},
		),
	)
	defer server.Close()

	requestHeaders := make(
		http.Header,
	)

	requestHeaders.Set(
		"Authorization",
		sanitize.RedactedValue,
	)

	responseHeaders := make(
		http.Header,
	)

	responseHeaders.Set(
		"Content-Type",
		"application/json",
	)

	baselineBody := []byte(
		`{"echo":"baseline"}`,
	)

	path := createSecretHeaderTestRecording(
		t,
		server.URL,
		requestHeaders,
		recording.Response{
			StatusCode:   http.StatusOK,
			Headers:      responseHeaders,
			Body:         baselineBody,
			ObservedSize: int64(len(baselineBody)),
			Complete:     true,
		},
	)

	t.Setenv(
		"GRAYBOX_DIFF_ECHO_AUTH",
		authorization,
	)

	tests := []struct {
		name string
		json bool
	}{
		{
			name: "human",
			json: false,
		},
		{
			name: "json",
			json: true,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(
				t *testing.T,
			) {
				args := []string{
					"diff",
					path,
					"--secret-header",
					"Authorization=GRAYBOX_DIFF_ECHO_AUTH",
				}

				if test.json {
					args = append(
						args,
						"--json",
					)
				}

				var stdout bytes.Buffer
				var stderr bytes.Buffer

				code := (App{
					Stdout: &stdout,
					Stderr: &stderr,
				}).Run(
					context.Background(),
					args,
				)

				if code != ExitBehaviorChanged {
					t.Fatalf(
						"exit = %d, stdout = %q, stderr = %q",
						code,
						stdout.String(),
						stderr.String(),
					)
				}

				if stderr.Len() != 0 {
					t.Fatalf(
						"stderr = %q",
						stderr.String(),
					)
				}

				if strings.Contains(
					stdout.String(),
					authorization,
				) {
					t.Fatalf(
						"output leaked raw secret: %q",
						stdout.String(),
					)
				}

				if !strings.Contains(
					stdout.String(),
					sanitize.RedactedValue,
				) {
					t.Fatalf(
						"output did not contain redaction marker: %q",
						stdout.String(),
					)
				}

				if test.json &&
					!json.Valid(
						stdout.Bytes(),
					) {
					t.Fatalf(
						"JSON became invalid after redaction: %q",
						stdout.String(),
					)
				}
			},
		)
	}
}

func createSecretHeaderTestRecording(
	t *testing.T,
	target string,
	requestHeaders http.Header,
	response recording.Response,
) string {
	t.Helper()

	ctx := context.Background()

	path := filepath.Join(
		t.TempDir(),
		"secret-headers.graybox",
	)

	store, err := storage.Create(
		ctx,
		path,
		"test",
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SetMetadata(
		ctx,
		"target_url",
		target,
	); err != nil {
		t.Fatal(err)
	}

	if err := store.SetMetadata(
		ctx,
		"body_capture_limit",
		"1024",
	); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()

	_, err = store.Add(
		ctx,
		recording.Exchange{
			Protocol:  "http",
			StartedAt: now,
			EndedAt: now.Add(
				time.Millisecond,
			),
			Duration: time.Millisecond,
			Request: recording.Request{
				Method:   http.MethodGet,
				URL:      "/resource",
				Headers:  requestHeaders,
				Complete: true,
			},
			Response: response,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	return path
}
