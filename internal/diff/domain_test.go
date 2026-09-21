package diff

import (
	"errors"
	"testing"
	"time"
)

func TestValueDistinguishesMissingFromNull(t *testing.T) {
	missing := MissingValue()
	null := ValueOf(nil)

	if missing.Present {
		t.Fatal("missing value was marked present")
	}

	if !null.Present {
		t.Fatal("explicit null was marked missing")
	}

	if null.Data != nil {
		t.Fatalf("null data = %#v, want nil", null.Data)
	}
}

func TestComparisonEquivalent(t *testing.T) {
	tests := []struct {
		name       string
		comparison Comparison
		want       bool
	}{
		{
			name: "nil differences",
			comparison: Comparison{
				Differences: nil,
			},
			want: true,
		},
		{
			name: "empty differences",
			comparison: Comparison{
				Differences: []Difference{},
			},
			want: true,
		},
		{
			name: "one difference",
			comparison: Comparison{
				Differences: []Difference{
					{
						Kind:     KindStatusChanged,
						Location: StatusLocation(),
						Before:   ValueOf(500),
						After:    ValueOf(200),
					},
				},
			},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.comparison.Equivalent(); got != test.want {
				t.Fatalf(
					"Equivalent() = %v, want %v",
					got,
					test.want,
				)
			}
		})
	}
}

func TestExchangeResultOutcome(t *testing.T) {
	tests := []struct {
		name   string
		result ExchangeResult
		want   Outcome
	}{
		{
			name:   "equivalent",
			result: ExchangeResult{},
			want:   OutcomeEquivalent,
		},
		{
			name: "changed",
			result: ExchangeResult{
				Comparison: Comparison{
					Differences: []Difference{
						{
							Kind:     KindValueChanged,
							Location: BodyLocation("/total"),
							Before:   ValueOf(49.90),
							After:    ValueOf(54.90),
						},
					},
				},
			},
			want: OutcomeChanged,
		},
		{
			name: "failed",
			result: ExchangeResult{
				Err: errors.New("replay failed"),
			},
			want: OutcomeFailed,
		},
		{
			name: "failure wins over differences",
			result: ExchangeResult{
				Comparison: Comparison{
					Differences: []Difference{
						{
							Kind:     KindStatusChanged,
							Location: StatusLocation(),
							Before:   ValueOf(500),
							After:    ValueOf(200),
						},
					},
				},
				Err: errors.New("response body could not be read"),
			},
			want: OutcomeFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.result.Outcome(); got != test.want {
				t.Fatalf(
					"Outcome() = %q, want %q",
					got,
					test.want,
				)
			}
		})
	}
}

func TestReportSummary(t *testing.T) {
	report := Report{
		Results: []ExchangeResult{
			{
				ExchangeID: 1,
				Method:     "GET",
				Path:       "/health",
			},
			{
				ExchangeID: 2,
				Method:     "POST",
				Path:       "/checkout",
				Comparison: Comparison{
					Differences: []Difference{
						{
							Kind:     KindStatusChanged,
							Location: StatusLocation(),
							Before:   ValueOf(500),
							After:    ValueOf(200),
						},
					},
				},
			},
			{
				ExchangeID: 3,
				Method:     "GET",
				Path:       "/account",
				Err:        errors.New("connection refused"),
			},
		},
	}

	got := report.Summary()

	if got.Total != 3 {
		t.Fatalf("Total = %d, want 3", got.Total)
	}

	if got.Equivalent != 1 {
		t.Fatalf("Equivalent = %d, want 1", got.Equivalent)
	}

	if got.Changed != 1 {
		t.Fatalf("Changed = %d, want 1", got.Changed)
	}

	if got.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", got.Failed)
	}
}

func TestExchangeResultRetainsDurationsWithoutChangingOutcome(t *testing.T) {
	result := ExchangeResult{
		BaselineDuration: 20 * time.Millisecond,
		CurrentDuration:  200 * time.Millisecond,
	}

	if got := result.Outcome(); got != OutcomeEquivalent {
		t.Fatalf(
			"Outcome() = %q, want %q",
			got,
			OutcomeEquivalent,
		)
	}
}

func TestHeaderLocationNormalizesCase(t *testing.T) {
	got := HeaderLocation("Content-Type")

	if got.Component != ComponentHeaders {
		t.Fatalf(
			"Component = %q, want %q",
			got.Component,
			ComponentHeaders,
		)
	}

	if got.Path != "content-type" {
		t.Fatalf(
			"Path = %q, want %q",
			got.Path,
			"content-type",
		)
	}
}

func TestLocationString(t *testing.T) {
	tests := []struct {
		name     string
		location Location
		want     string
	}{
		{
			name:     "status",
			location: StatusLocation(),
			want:     "response.status",
		},
		{
			name:     "header",
			location: HeaderLocation("Content-Type"),
			want:     "response.headers.content-type",
		},
		{
			name:     "whole body",
			location: BodyLocation(""),
			want:     "response.body",
		},
		{
			name:     "json body field",
			location: BodyLocation("/user/name"),
			want:     "response.body#/user/name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.location.String(); got != test.want {
				t.Fatalf(
					"String() = %q, want %q",
					got,
					test.want,
				)
			}
		})
	}
}

func TestAppendJSONPointer(t *testing.T) {
	tests := []struct {
		name    string
		pointer string
		token   string
		want    string
	}{
		{
			name:    "root key",
			pointer: "",
			token:   "user",
			want:    "/user",
		},
		{
			name:    "nested key",
			pointer: "/user",
			token:   "name",
			want:    "/user/name",
		},
		{
			name:    "slash in key",
			pointer: "",
			token:   "a/b",
			want:    "/a~1b",
		},
		{
			name:    "tilde in key",
			pointer: "",
			token:   "a~b",
			want:    "/a~0b",
		},
		{
			name:    "slash and tilde",
			pointer: "/items",
			token:   "a~/b",
			want:    "/items/a~0~1b",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := AppendJSONPointer(
				test.pointer,
				test.token,
			); got != test.want {
				t.Fatalf(
					"AppendJSONPointer(%q, %q) = %q, want %q",
					test.pointer,
					test.token,
					got,
					test.want,
				)
			}
		})
	}
}

func TestRulesIgnoreExactHeader(t *testing.T) {
	rules := Rules{
		Ignore: []Location{
			HeaderLocation("Date"),
		},
	}

	if !rules.Ignores(HeaderLocation("date")) {
		t.Fatal("Date header was not ignored case-insensitively")
	}

	if rules.Ignores(HeaderLocation("Content-Type")) {
		t.Fatal("unrelated Content-Type header was ignored")
	}
}

func TestRulesIgnoreWholeComponent(t *testing.T) {
	rules := Rules{
		Ignore: []Location{
			{
				Component: ComponentHeaders,
			},
		},
	}

	if !rules.Ignores(HeaderLocation("Date")) {
		t.Fatal("whole headers component did not ignore Date")
	}

	if !rules.Ignores(HeaderLocation("Content-Type")) {
		t.Fatal("whole headers component did not ignore Content-Type")
	}

	if rules.Ignores(BodyLocation("/date")) {
		t.Fatal("header rule incorrectly ignored body location")
	}
}

func TestRulesIgnoreBodySubtree(t *testing.T) {
	rules := Rules{
		Ignore: []Location{
			BodyLocation("/metadata"),
		},
	}

	tests := []struct {
		location Location
		want     bool
	}{
		{
			location: BodyLocation("/metadata"),
			want:     true,
		},
		{
			location: BodyLocation("/metadata/request_id"),
			want:     true,
		},
		{
			location: BodyLocation("/metadata/nested/value"),
			want:     true,
		},
		{
			location: BodyLocation("/metadata2"),
			want:     false,
		},
		{
			location: BodyLocation("/user/metadata"),
			want:     false,
		},
	}

	for _, test := range tests {
		if got := rules.Ignores(test.location); got != test.want {
			t.Fatalf(
				"Ignores(%q) = %v, want %v",
				test.location.String(),
				got,
				test.want,
			)
		}
	}
}
