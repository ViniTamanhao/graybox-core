package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ViniTamanhao/graybox-core/internal/diff"
	"github.com/ViniTamanhao/graybox-core/internal/recording"
	"github.com/ViniTamanhao/graybox-core/internal/storage"
)

func TestDiffSemanticJSONEquivalent(
	t *testing.T,
) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				_ *http.Request,
			) {
				w.Header().Set(
					"Content-Type",
					"application/json",
				)

				w.Header().Set(
					"X-Version",
					"1",
				)

				_, _ = w.Write(
					[]byte(`{
						"value": 1.0
					}`),
				)
			},
		),
	)
	defer server.Close()

	baselineBody := []byte(
		`{"value":1}`,
	)

	exchange := diffTestExchange(
		baselineBody,
		http.StatusOK,
		http.Header{
			"Content-Type": {
				"application/json",
			},
			"X-Version": {
				"1",
			},
			"Date": {
				"Mon, 01 Jan 2001 00:00:00 GMT",
			},
			"Content-Length": {
				strconv.Itoa(
					len(baselineBody),
				),
			},
		},
	)

	path := createDiffTestRecording(
		t,
		server.URL,
		1024,
		exchange,
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

	if stderr.Len() != 0 {
		t.Fatalf(
			"stderr = %q",
			stderr.String(),
		)
	}

	var report diffReportJSON

	if err := json.Unmarshal(
		stdout.Bytes(),
		&report,
	); err != nil {
		t.Fatalf(
			"decode JSON: %v\n%s",
			err,
			stdout.String(),
		)
	}

	if report.Summary.Total != 1 ||
		report.Summary.Equivalent != 1 ||
		report.Summary.Changed != 0 ||
		report.Summary.Failed != 0 {
		t.Fatalf(
			"summary = %#v",
			report.Summary,
		)
	}

	if report.BodyCaptureLimit != 1024 {
		t.Fatalf(
			"body capture limit = %d, want 1024",
			report.BodyCaptureLimit,
		)
	}

	if len(report.Ignored) != 2 ||
		report.Ignored[0] != "response.headers.date" ||
		report.Ignored[1] != "response.headers.content-length" {
		t.Fatalf(
			"ignored = %#v",
			report.Ignored,
		)
	}

	if len(report.Results) != 1 ||
		report.Results[0].Outcome !=
			string(diff.OutcomeEquivalent) {
		t.Fatalf(
			"results = %#v",
			report.Results,
		)
	}
}

func TestDiffReportsBehaviorChanges(
	t *testing.T,
) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				_ *http.Request,
			) {
				w.Header().Set(
					"Content-Type",
					"application/json",
				)

				w.Header().Set(
					"X-Version",
					"2",
				)

				w.WriteHeader(
					http.StatusCreated,
				)

				_, _ = w.Write(
					[]byte(`{"value":2}`),
				)
			},
		),
	)
	defer server.Close()

	exchange := diffTestExchange(
		[]byte(`{"value":1}`),
		http.StatusOK,
		http.Header{
			"Content-Type": {
				"application/json",
			},
			"X-Version": {
				"1",
			},
		},
	)

	path := createDiffTestRecording(
		t,
		server.URL,
		1024,
		exchange,
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
		},
	)

	if code != ExitBehaviorChanged {
		t.Fatalf(
			"exit = %d, stdout = %q, stderr = %q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}

	for _, expected := range []string{
		"changed",
		"response.status",
		"response.headers.x-version",
		"response.body#/value",
		"1 changed",
		"0 failed",
	} {
		if !strings.Contains(
			stdout.String(),
			expected,
		) {
			t.Fatalf(
				"stdout missing %q:\n%s",
				expected,
				stdout.String(),
			)
		}
	}

	if stderr.Len() != 0 {
		t.Fatalf(
			"stderr = %q",
			stderr.String(),
		)
	}
}

func TestDiffCustomIgnoreMakesReplayEquivalent(
	t *testing.T,
) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				_ *http.Request,
			) {
				w.Header().Set(
					"Content-Type",
					"application/json",
				)

				_, _ = w.Write(
					[]byte(
						`{"request_id":"new","stable":1}`,
					),
				)
			},
		),
	)
	defer server.Close()

	exchange := diffTestExchange(
		[]byte(
			`{"request_id":"old","stable":1}`,
		),
		http.StatusOK,
		http.Header{
			"Content-Type": {
				"application/json",
			},
		},
	)

	path := createDiffTestRecording(
		t,
		server.URL,
		1024,
		exchange,
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
			"--ignore",
			"response.body#/request_id",
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

	if !strings.Contains(
		stdout.String(),
		"1 equivalent",
	) {
		t.Fatalf(
			"stdout = %q",
			stdout.String(),
		)
	}
}

func TestDiffUsesRecordedBodyCaptureLimit(
	t *testing.T,
) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				_ *http.Request,
			) {
				w.Header().Set(
					"Content-Type",
					"text/plain",
				)

				_, _ = w.Write(
					[]byte("abcdef"),
				)
			},
		),
	)
	defer server.Close()

	exchange := diffTestExchange(
		[]byte("abcde"),
		http.StatusOK,
		http.Header{
			"Content-Type": {
				"text/plain",
			},
		},
	)

	path := createDiffTestRecording(
		t,
		server.URL,
		5,
		exchange,
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
		},
	)

	if code != ExitNetwork {
		t.Fatalf(
			"exit = %d, stdout = %q, stderr = %q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}

	if !strings.Contains(
		stdout.String(),
		"failed",
	) ||
		!strings.Contains(
			stdout.String(),
			"capture is truncated",
		) {
		t.Fatalf(
			"stdout = %q",
			stdout.String(),
		)
	}
}

func TestDiffMissingExchangeUsesInvalidRecordingExit(
	t *testing.T,
) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				http.ResponseWriter,
				*http.Request,
			) {
			},
		),
	)
	defer server.Close()

	exchange := diffTestExchange(
		[]byte(`{"ok":true}`),
		http.StatusOK,
		http.Header{
			"Content-Type": {
				"application/json",
			},
		},
	)

	path := createDiffTestRecording(
		t,
		server.URL,
		1024,
		exchange,
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
			"--id",
			"99",
		},
	)

	if code != ExitInvalidRecording {
		t.Fatalf(
			"exit = %d, stdout = %q, stderr = %q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}

	if !strings.Contains(
		stderr.String(),
		"has no exchange 99",
	) {
		t.Fatalf(
			"stderr = %q",
			stderr.String(),
		)
	}
}

func TestDiffProtectsSavedRemoteTarget(
	t *testing.T,
) {
	ctx := context.Background()

	path := filepath.Join(
		t.TempDir(),
		"remote.graybox",
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
		"https://api.example.com",
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

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := (App{
		Stdout: &stdout,
		Stderr: &stderr,
	}).Run(
		ctx,
		[]string{
			"diff",
			path,
		},
	)

	if code != ExitUsage ||
		!strings.Contains(
			stderr.String(),
			"is not loopback",
		) {
		t.Fatalf(
			"exit/stdout/stderr = %d/%q/%q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}

	stdout.Reset()
	stderr.Reset()

	code = (App{
		Stdout: &stdout,
		Stderr: &stderr,
	}).Run(
		ctx,
		[]string{
			"diff",
			path,
			"--unsafe-original-target",
		},
	)

	if code != ExitSuccess ||
		!strings.Contains(
			stdout.String(),
			"Diffing 0 exchanges",
		) {
		t.Fatalf(
			"unsafe exit/stdout/stderr = %d/%q/%q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestDiffJSONBodySnapshotUsesHexDigest(
	t *testing.T,
) {
	beforeDigest := sha256.Sum256(
		[]byte("before"),
	)

	afterDigest := sha256.Sum256(
		[]byte("after"),
	)

	report := diff.Report{
		Results: []diff.ExchangeResult{
			{
				ExchangeID: 1,
				Method:     http.MethodGet,
				Path:       "/binary",
				Comparison: diff.Comparison{
					Differences: []diff.Difference{
						{
							Kind: diff.KindBodyChanged,
							Location: diff.BodyLocation(
								"",
							),
							Before: diff.ValueOf(
								diff.BodySnapshot{
									Size:   6,
									SHA256: beforeDigest,
								},
							),
							After: diff.ValueOf(
								diff.BodySnapshot{
									Size:   5,
									SHA256: afterDigest,
								},
							),
						},
					},
				},
			},
		},
	}

	output := toDiffReportJSON(
		"bug.graybox",
		"http://localhost:8080",
		1024,
		nil,
		report,
	)

	before, ok := output.Results[0].
		Differences[0].
		Before.
		Value.(diffBodySnapshotJSON)

	if !ok {
		t.Fatalf(
			"snapshot type = %T",
			output.Results[0].
				Differences[0].
				Before.
				Value,
		)
	}

	if len(before.SHA256) != 64 {
		t.Fatalf(
			"SHA256 = %q",
			before.SHA256,
		)
	}

	encoded, err := json.Marshal(
		output,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(
		string(encoded),
		`"sha256":"`,
	) {
		t.Fatalf(
			"JSON = %s",
			encoded,
		)
	}
}

func TestParseDiffIgnoreLocation(
	t *testing.T,
) {
	tests := []struct {
		input string
		want  diff.Location
		ok    bool
	}{
		{
			input: "response.status",
			want:  diff.StatusLocation(),
			ok:    true,
		},
		{
			input: "response.headers",
			want: diff.Location{
				Component: diff.ComponentHeaders,
			},
			ok: true,
		},
		{
			input: "response.headers.X-Request-ID",
			want: diff.HeaderLocation(
				"X-Request-ID",
			),
			ok: true,
		},
		{
			input: "response.body",
			want: diff.BodyLocation(
				"",
			),
			ok: true,
		},
		{
			input: "response.body#/metadata/request_id",
			want: diff.BodyLocation(
				"/metadata/request_id",
			),
			ok: true,
		},
		{
			input: "response.body#/a~1b/~0key",
			want: diff.BodyLocation(
				"/a~1b/~0key",
			),
			ok: true,
		},
		{
			input: "response.body#metadata",
			ok:    false,
		},
		{
			input: "response.body#/bad~2escape",
			ok:    false,
		},
		{
			input: "request.body",
			ok:    false,
		},
	}

	for _, test := range tests {
		t.Run(
			test.input,
			func(t *testing.T) {
				got, err := parseDiffIgnoreLocation(
					test.input,
				)

				if !test.ok {
					if err == nil {
						t.Fatalf(
							"error = nil, got %#v",
							got,
						)
					}

					return
				}

				if err != nil {
					t.Fatal(err)
				}

				if got != test.want {
					t.Fatalf(
						"got %#v, want %#v",
						got,
						test.want,
					)
				}
			},
		)
	}
}

func TestDiffFailureDominatesChangedExitCode(
	t *testing.T,
) {
	report := diff.Report{
		Results: []diff.ExchangeResult{
			{
				Comparison: diff.Comparison{
					Differences: []diff.Difference{
						{
							Kind: diff.KindStatusChanged,
						},
					},
				},
			},
			{
				Err: context.DeadlineExceeded,
			},
		},
	}

	if got := diffExitCode(
		report,
	); got != ExitNetwork {
		t.Fatalf(
			"exit = %d, want %d",
			got,
			ExitNetwork,
		)
	}
}

func createDiffTestRecording(
	t *testing.T,
	target string,
	bodyLimit int64,
	exchange recording.Exchange,
) string {
	t.Helper()

	ctx := context.Background()

	path := filepath.Join(
		t.TempDir(),
		"diff.graybox",
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
		strconv.FormatInt(
			bodyLimit,
			10,
		),
	); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Add(
		ctx,
		exchange,
	); err != nil {
		t.Fatal(err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	return path
}

func diffTestExchange(
	body []byte,
	status int,
	headers http.Header,
) recording.Exchange {
	now := time.Now().UTC()

	body = append(
		[]byte(nil),
		body...,
	)

	return recording.Exchange{
		Protocol:  "http",
		StartedAt: now,
		EndedAt: now.Add(
			time.Millisecond,
		),
		Duration: time.Millisecond,
		Request: recording.Request{
			Method:   http.MethodGet,
			URL:      "/resource",
			Complete: true,
		},
		Response: recording.Response{
			StatusCode:   status,
			Headers:      headers,
			Body:         body,
			ObservedSize: int64(len(body)),
			Complete:     true,
		},
	}
}
