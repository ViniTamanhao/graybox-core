// Package diff contains Graybox's behavioral comparison model.
package diff

import "time"

// DifferenceKind identifies a behavioral difference between the recorded
// baseline response and the response observed during replay.
type DifferenceKind string

const (
	// KindStatusChanged means the HTTP response status changed.
	KindStatusChanged DifferenceKind = "status_changed"

	// KindHeaderAdded means the replayed response contains a header that was
	// not present in the recorded baseline response.
	KindHeaderAdded DifferenceKind = "header_added"

	// KindHeaderRemoved means a header present in the recorded baseline
	// response is absent from the replayed response.
	KindHeaderRemoved DifferenceKind = "header_removed"

	// KindHeaderChanged means a response header exists in both responses but
	// its values differ.
	KindHeaderChanged DifferenceKind = "header_changed"

	// KindFieldAdded means a JSON object field exists in the replayed response
	// but not in the recorded baseline response.
	KindFieldAdded DifferenceKind = "field_added"

	// KindFieldRemoved means a JSON object field exists in the recorded
	// baseline response but not in the replayed response.
	KindFieldRemoved DifferenceKind = "field_removed"

	// KindTypeChanged means a JSON value exists in both responses but its JSON
	// type changed.
	KindTypeChanged DifferenceKind = "type_changed"

	// KindValueChanged means a value exists in both responses with the same
	// semantic type but a different value.
	KindValueChanged DifferenceKind = "value_changed"

	// KindBodyChanged means a body that is not semantically compared as JSON
	// differs from the recorded baseline body.
	//
	// V1 uses this for textual and binary bodies that are compared exactly.
	KindBodyChanged DifferenceKind = "body_changed"

	// KindElementAdded means an element exists at a JSON array position in the
	// replayed response but not in the recorded baseline response.
	KindElementAdded DifferenceKind = "element_added"

	// KindElementRemoved means an element exists at a JSON array position in the
	// recorded baseline response but not in the replayed response.
	KindElementRemoved DifferenceKind = "element_removed"
)

// Value is one side of a behavioral difference.
//
// Present distinguishes an absent value from a value that is explicitly null.
// This matters for JSON:
//
//	{"name": null}
//
// is semantically different from:
//
//	{}
//
// ValueOf(nil) therefore represents a present JSON null, while MissingValue()
// represents absence.
type Value struct {
	Present bool
	Data    any
}

// ValueOf returns a present difference value.
//
// data may itself be nil, in which case the value represents an explicit JSON
// null rather than a missing value.
func ValueOf(data any) Value {
	return Value{
		Present: true,
		Data:    data,
	}
}

// MissingValue returns a value representing absence.
func MissingValue() Value {
	return Value{}
}

// Difference describes one concrete behavioral change.
//
// Location identifies what changed. Before and After preserve the baseline and
// replayed values without imposing human or JSON rendering concerns on the
// comparison package.
type Difference struct {
	Kind     DifferenceKind
	Location Location
	Before   Value
	After    Value
}

// Comparison contains behavioral differences discovered while comparing one
// baseline response with one current response.
//
// Compare may return a partial Comparison together with a non-nil error when
// some evidence cannot be compared reliably. Callers must therefore check the
// error returned by Compare before treating Equivalent as a conclusion about
// the complete responses.
//
// Equivalent is deviced from Differences rather than stored separately so the
// model cannot represent contradictory states such as Equivalent=true while
// also containing differences.
type Comparison struct {
	Differences []Difference
}

// Equivalent reports whether this Comparison contains no behavioral
// differences. Callers of Compare must also check its returned error before
// treating the complete responses as behaviorally equivalent.
func (c Comparison) Equivalent() bool {
	return len(c.Differences) == 0
}

// Outcome is the high-level state of one attempted exchange comparison.
type Outcome string

const (
	// OutcomeEquivalent means replay completed and no behavioral differences
	// were found.
	OutcomeEquivalent Outcome = "equivalent"

	// OutcomeChanged means replay completed successfully but one or more
	// behavioral differences were found.
	OutcomeChanged Outcome = "changed"

	// OutcomeFailed means Graybox could not produce a response suitable for
	// behavioral comparison, for example because replay failed.
	OutcomeFailed Outcome = "failed"
)

// ExchangeResult describes the comparison of one recorded exchange against a
// replayed request.
//
// BaselineDuration and CurrentDuration are retained as observations but are not
// automatically behavioral differences. Network timing is inherently noisy;
// latency should only affect equivalence once Graybox has explicit threshold
// semantics.
type ExchangeResult struct {
	ExchangeID int64
	Method     string
	Path       string

	BaselineDuration time.Duration
	CurrentDuration  time.Duration

	Comparison Comparison
	Err        error
}

// Outcome returns the derived state of the exchange comparison.
func (r ExchangeResult) Outcome() Outcome {
	if r.Err != nil {
		return OutcomeFailed
	}
	if r.Comparison.Equivalent() {
		return OutcomeEquivalent
	}
	return OutcomeChanged
}

// Summary contains aggregate counts for a diff report.
type Summary struct {
	Total      int
	Equivalent int
	Changed    int
	Failed     int
}

// Report is the complete result of a Graybox diff operation.
//
// It deliberately contains domain results only. Information such as the
// recording filename, target URL, JSON field names, terminal formatting, and
// process exit codes belongs to orchestration or presentation layers.
type Report struct {
	Results []ExchangeResult
}

// Summary derives aggregate counts from the report.
//
// Counts are computed instead of stored in Report so they cannot become stale
// or inconsistent with Results.
func (r Report) Summary() Summary {
	summary := Summary{
		Total: len(r.Results),
	}

	for _, result := range r.Results {
		switch result.Outcome() {
		case OutcomeEquivalent:
			summary.Equivalent++
		case OutcomeChanged:
			summary.Changed++
		case OutcomeFailed:
			summary.Failed++
		}
	}

	return summary
}
