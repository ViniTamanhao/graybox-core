package diff

import (
	"context"
	"errors"
	"net/url"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
	"github.com/ViniTamanhao/graybox-core/internal/replay"
)

// Replayer supplies replay executions one exchange at a time.
//
// The callback receives both the persisted baseline exchange and the observed
// execution so the diff runner can compare them without reloading the exchange
// or retaining every response body in memory.
type Replayer interface {
	RunEach(
		context.Context,
		*url.URL,
		*int64,
		replay.VisitFunc,
	) error
}

// Runner replays recorded exchanges and compares their observed responses with
// the persisted baseline behavior.
type Runner struct {
	Replayer Replayer
	Rules    Rules
}

// Run replays all selected exchanges and returns their behavioral diff report.
//
// Errors returned directly from Run represent failures to enumerate or load the
// recording itself. Per-exchange replay and comparison failures are retained in
// ExchangeResult.Err so one failed replay does not prevent the remaining
// exchanges from being evaluated.
func (r Runner) Run(
	ctx context.Context,
	target *url.URL,
	id *int64,
) (Report, error) {
	var report Report

	err := r.Replayer.RunEach(
		ctx,
		target,
		id,
		func(
			baseline recording.Exchange,
			execution replay.Execution,
		) error {
			report.Results = append(
				report.Results,
				compareExecution(
					baseline,
					execution,
					r.Rules,
				),
			)

			return nil
		},
	)
	if err != nil {
		return Report{}, err
	}

	return report, nil
}

func compareExecution(
	baseline recording.Exchange,
	execution replay.Execution,
	rules Rules,
) ExchangeResult {
	result := ExchangeResult{
		ExchangeID:       baseline.ID,
		Method:           baseline.Request.Method,
		Path:             baseline.Request.URL,
		BaselineDuration: baseline.Duration,
		CurrentDuration:  execution.Duration,
	}

	if execution.Response == nil {
		result.Err = execution.Err

		// A nil response with no replay error should be impossible, but keeping
		// the invariant explicit prevents a future replay regression from being
		// misclassified as behavioral equivalence.
		if result.Err == nil {
			result.Err = errors.New(
				"replay completed without an HTTP response",
			)
		}

		return result
	}

	comparison, comparisonErr := Compare(
		baseline.Response,
		*execution.Response,
		rules,
	)

	result.Comparison = comparison

	// When replay itself failed after receiving a response, such as during a
	// response-body read, preserve the replay error as the primary failure.
	// Compare still gives us any status/header differences that were knowable
	// before the body failed.
	if execution.Err != nil {
		result.Err = execution.Err
		return result
	}

	result.Err = comparisonErr

	return result
}
