package diff

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
	"github.com/ViniTamanhao/graybox-core/internal/replay"
)

type singleExchangeSource struct {
	exchange recording.Exchange
}

func (s singleExchangeSource) Get(
	context.Context,
	int64,
) (recording.Exchange, error) {
	return s.exchange, nil
}

func (s singleExchangeSource) List(
	context.Context,
	recording.Filter,
) ([]recording.Summary, error) {
	return []recording.Summary{
		{
			ID: s.exchange.ID,
		},
	}, nil
}

type fakeReplayVisit struct {
	baseline  recording.Exchange
	execution replay.Execution
}

type fakeReplayer struct {
	visits []fakeReplayVisit
	err    error
}

func (r fakeReplayer) RunEach(
	_ context.Context,
	_ *url.URL,
	_ *int64,
	visit replay.VisitFunc,
) error {
	for _, item := range r.visits {
		if err := visit(
			item.baseline,
			item.execution,
		); err != nil {
			return err
		}
	}

	return r.err
}

func TestRunnerReportsSemanticallyEquivalentReplay(
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

	target, err := url.Parse(
		server.URL,
	)
	if err != nil {
		t.Fatal(err)
	}

	exchange := testDiffExchange(
		[]byte(`{"value":1}`),
	)

	report, err := (Runner{
		Replayer: replay.Runner{
			Source: singleExchangeSource{
				exchange: exchange,
			},
			Client: server.Client(),
		},
		Rules: testReplayRules(),
	}).Run(
		context.Background(),
		target,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Results) != 1 {
		t.Fatalf(
			"len(Results) = %d, want 1",
			len(report.Results),
		)
	}

	result := report.Results[0]

	if result.Err != nil {
		t.Fatalf(
			"result error = %v",
			result.Err,
		)
	}

	if result.Outcome() != OutcomeEquivalent {
		t.Fatalf(
			"Outcome() = %q, want %q; differences = %#v",
			result.Outcome(),
			OutcomeEquivalent,
			result.Comparison.Differences,
		)
	}

	summary := report.Summary()

	if summary.Total != 1 ||
		summary.Equivalent != 1 ||
		summary.Changed != 0 ||
		summary.Failed != 0 {
		t.Fatalf(
			"summary = %#v",
			summary,
		)
	}
}

func TestRunnerReportsReplayBehaviorChanges(
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

	target, err := url.Parse(
		server.URL,
	)
	if err != nil {
		t.Fatal(err)
	}

	exchange := testDiffExchange(
		[]byte(`{"value":1}`),
	)

	report, err := (Runner{
		Replayer: replay.Runner{
			Source: singleExchangeSource{
				exchange: exchange,
			},
			Client: server.Client(),
		},
		Rules: testReplayRules(),
	}).Run(
		context.Background(),
		target,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Results) != 1 {
		t.Fatalf(
			"len(Results) = %d, want 1",
			len(report.Results),
		)
	}

	result := report.Results[0]

	if result.Err != nil {
		t.Fatalf(
			"result error = %v",
			result.Err,
		)
	}

	if result.Outcome() != OutcomeChanged {
		t.Fatalf(
			"Outcome() = %q, want %q",
			result.Outcome(),
			OutcomeChanged,
		)
	}

	if len(result.Comparison.Differences) != 3 {
		t.Fatalf(
			"len(Differences) = %d, want 3: %#v",
			len(result.Comparison.Differences),
			result.Comparison.Differences,
		)
	}

	if result.Comparison.Differences[0].Kind !=
		KindStatusChanged {
		t.Fatalf(
			"first difference kind = %q, want %q",
			result.Comparison.Differences[0].Kind,
			KindStatusChanged,
		)
	}

	if result.Comparison.Differences[1].Kind !=
		KindHeaderChanged {
		t.Fatalf(
			"second difference kind = %q, want %q",
			result.Comparison.Differences[1].Kind,
			KindHeaderChanged,
		)
	}

	if result.Comparison.Differences[2].Kind !=
		KindValueChanged {
		t.Fatalf(
			"third difference kind = %q, want %q",
			result.Comparison.Differences[2].Kind,
			KindValueChanged,
		)
	}
}

func TestRunnerFailsWhenBaselineBodyIsUnavailable(
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
					[]byte(`{"value":1}`),
				)
			},
		),
	)
	defer server.Close()

	target, err := url.Parse(
		server.URL,
	)
	if err != nil {
		t.Fatal(err)
	}

	exchange := testDiffExchange(
		[]byte(`{"val`),
	)

	exchange.Response.ObservedSize = 20
	exchange.Response.Truncated = true

	report, err := (Runner{
		Replayer: replay.Runner{
			Source: singleExchangeSource{
				exchange: exchange,
			},
			Client: server.Client(),
		},
		Rules: testReplayRules(),
	}).Run(
		context.Background(),
		target,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Results) != 1 {
		t.Fatalf(
			"len(Results) = %d, want 1",
			len(report.Results),
		)
	}

	result := report.Results[0]

	if !errors.Is(
		result.Err,
		ErrBodyUnavailable,
	) {
		t.Fatalf(
			"result error = %v, want ErrBodyUnavailable",
			result.Err,
		)
	}

	if result.Outcome() != OutcomeFailed {
		t.Fatalf(
			"Outcome() = %q, want %q",
			result.Outcome(),
			OutcomeFailed,
		)
	}
}

func TestRunnerContinuesAfterPerExchangeReplayFailure(
	t *testing.T,
) {
	first := testDiffExchange(
		[]byte(`{"value":1}`),
	)
	first.ID = 1

	second := testDiffExchange(
		[]byte(`{"value":2}`),
	)
	second.ID = 2

	secondResponse := second.Response

	report, err := (Runner{
		Replayer: fakeReplayer{
			visits: []fakeReplayVisit{
				{
					baseline: first,
					execution: replay.Execution{
						ExchangeID: first.ID,
						Method:     first.Request.Method,
						Path:       first.Request.URL,
						Err: errors.New(
							"connection refused",
						),
					},
				},
				{
					baseline: second,
					execution: replay.Execution{
						ExchangeID: second.ID,
						Method:     second.Request.Method,
						Path:       second.Request.URL,
						Response:   &secondResponse,
					},
				},
			},
		},
	}).Run(
		context.Background(),
		&url.URL{
			Scheme: "http",
			Host:   "example.test",
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Results) != 2 {
		t.Fatalf(
			"len(Results) = %d, want 2",
			len(report.Results),
		)
	}

	if report.Results[0].Outcome() != OutcomeFailed {
		t.Fatalf(
			"first outcome = %q, want %q",
			report.Results[0].Outcome(),
			OutcomeFailed,
		)
	}

	if report.Results[1].Outcome() != OutcomeEquivalent {
		t.Fatalf(
			"second outcome = %q, want %q",
			report.Results[1].Outcome(),
			OutcomeEquivalent,
		)
	}

	summary := report.Summary()

	if summary.Total != 2 {
		t.Fatalf(
			"Total = %d, want 2",
			summary.Total,
		)
	}

	if summary.Failed != 1 {
		t.Fatalf(
			"Failed = %d, want 1",
			summary.Failed,
		)
	}

	if summary.Equivalent != 1 {
		t.Fatalf(
			"Equivalent = %d, want 1",
			summary.Equivalent,
		)
	}

	if summary.Changed != 0 {
		t.Fatalf(
			"Changed = %d, want 0",
			summary.Changed,
		)
	}
}

func testDiffExchange(
	body []byte,
) recording.Exchange {
	copied := append(
		[]byte(nil),
		body...,
	)

	return recording.Exchange{
		ID:       1,
		Duration: 25 * time.Millisecond,
		Request: recording.Request{
			Method:   http.MethodGet,
			URL:      "/resource",
			Complete: true,
		},
		Response: recording.Response{
			StatusCode: http.StatusOK,
			Headers: http.Header{
				"Content-Type": {
					"application/json",
				},
				"X-Version": {
					"1",
				},
			},
			Body:         copied,
			ObservedSize: int64(len(copied)),
			Complete:     true,
		},
	}
}

func testReplayRules() Rules {
	return Rules{
		Ignore: []Location{
			HeaderLocation("Date"),
			HeaderLocation("Content-Length"),
		},
	}
}
