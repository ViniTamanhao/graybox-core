package diff

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
)

func mustCompareCompleteBodies(
	t *testing.T,
	baseline recording.Response,
	current recording.Response,
	rules Rules,
) Comparison {
	t.Helper()

	// These tests exercise status/header behavior rather than capture failure.
	// Give their empty bodies the valid schema-1 state for a normally completed
	// zero-byte response.
	baseline.Complete = true
	baseline.ObservedSize = int64(len(baseline.Body))

	current.Complete = true
	current.ObservedSize = int64(len(current.Body))

	comparison, err := Compare(
		baseline,
		current,
		rules,
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	return comparison
}

func TestCompareEquivalentStatusAndHeaders(t *testing.T) {
	baseline := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type": {"application/json"},
			"X-Trace":      {"a", "b"},
		},
	}

	current := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"content-type": {"application/json"},
			"x-trace":      {"a", "b"},
		},
	}

	got := mustCompareCompleteBodies(
		t,
		baseline,
		current,
		Rules{},
	)

	if !got.Equivalent() {
		t.Fatalf(
			"Compare() differences = %#v, want equivalent",
			got.Differences,
		)
	}
}

func TestCompareStatusChanged(t *testing.T) {
	baseline := recording.Response{
		StatusCode: http.StatusInternalServerError,
	}

	current := recording.Response{
		StatusCode: http.StatusOK,
	}

	got := mustCompareCompleteBodies(
		t,
		baseline,
		current,
		Rules{},
	)

	want := Comparison{
		Differences: []Difference{
			{
				Kind:     KindStatusChanged,
				Location: StatusLocation(),
				Before:   ValueOf(http.StatusInternalServerError),
				After:    ValueOf(http.StatusOK),
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"Compare() = %#v, want %#v",
			got,
			want,
		)
	}
}

func TestCompareIgnoredStatus(t *testing.T) {
	baseline := recording.Response{
		StatusCode: http.StatusInternalServerError,
	}

	current := recording.Response{
		StatusCode: http.StatusOK,
	}

	got := mustCompareCompleteBodies(
		t,
		baseline,
		current,
		Rules{
			Ignore: []Location{
				StatusLocation(),
			},
		},
	)

	if !got.Equivalent() {
		t.Fatalf(
			"Compare() differences = %#v, want equivalent",
			got.Differences,
		)
	}
}

func TestCompareHeaderAddedRemovedAndChanged(t *testing.T) {
	baseline := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type": {"application/json"},
			"X-Changed":    {"a", "b"},
			"X-Removed":    {"before"},
		},
	}

	current := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type": {"application/json"},
			"X-Added":      {"after"},
			"X-Changed":    {"a", "c"},
		},
	}

	got := mustCompareCompleteBodies(
		t,
		baseline,
		current,
		Rules{},
	)

	want := Comparison{
		Differences: []Difference{
			{
				Kind:     KindHeaderAdded,
				Location: HeaderLocation("X-Added"),
				Before:   MissingValue(),
				After:    ValueOf([]string{"after"}),
			},
			{
				Kind:     KindHeaderChanged,
				Location: HeaderLocation("X-Changed"),
				Before:   ValueOf([]string{"a", "b"}),
				After:    ValueOf([]string{"a", "c"}),
			},
			{
				Kind:     KindHeaderRemoved,
				Location: HeaderLocation("X-Removed"),
				Before:   ValueOf([]string{"before"}),
				After:    MissingValue(),
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"Compare() = %#v, want %#v",
			got,
			want,
		)
	}
}

func TestCompareHeaderNamesAreCaseInsensitive(t *testing.T) {
	baseline := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
	}

	current := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"CONTENT-TYPE": {"application/json"},
		},
	}

	got := mustCompareCompleteBodies(
		t,
		baseline,
		current,
		Rules{},
	)

	if !got.Equivalent() {
		t.Fatalf(
			"Compare() differences = %#v, want equivalent",
			got.Differences,
		)
	}
}

func TestCompareRepeatedHeaderValueOrderIsSignificant(t *testing.T) {
	baseline := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"X-Example": {"a", "b"},
		},
	}

	current := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"X-Example": {"b", "a"},
		},
	}

	got := mustCompareCompleteBodies(
		t,
		baseline,
		current,
		Rules{},
	)

	want := Comparison{
		Differences: []Difference{
			{
				Kind:     KindHeaderChanged,
				Location: HeaderLocation("X-Example"),
				Before:   ValueOf([]string{"a", "b"}),
				After:    ValueOf([]string{"b", "a"}),
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"Compare() = %#v, want %#v",
			got,
			want,
		)
	}
}

func TestCompareIgnoredHeader(t *testing.T) {
	baseline := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Date":         {"Sat, 19 Sep 2026 12:00:00 GMT"},
			"Content-Type": {"application/json"},
		},
	}

	current := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Date":         {"Sat, 19 Sep 2026 12:00:05 GMT"},
			"Content-Type": {"application/json"},
		},
	}

	got := mustCompareCompleteBodies(
		t,
		baseline,
		current,
		Rules{
			Ignore: []Location{
				HeaderLocation("Date"),
			},
		},
	)

	if !got.Equivalent() {
		t.Fatalf(
			"Compare() differences = %#v, want equivalent",
			got.Differences,
		)
	}
}

func TestCompareIgnoredHeadersComponent(t *testing.T) {
	baseline := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type": {"application/json"},
			"X-Version":    {"one"},
		},
	}

	current := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type": {"text/plain"},
			"X-Another":    {"value"},
		},
	}

	got := mustCompareCompleteBodies(
		t,
		baseline,
		current,
		Rules{
			Ignore: []Location{
				{
					Component: ComponentHeaders,
				},
			},
		},
	)

	if !got.Equivalent() {
		t.Fatalf(
			"Compare() differences = %#v, want equivalent",
			got.Differences,
		)
	}
}

func TestCompareStatusIsReportedBeforeHeaders(t *testing.T) {
	baseline := recording.Response{
		StatusCode: http.StatusInternalServerError,
		Headers: http.Header{
			"Z-Header": {"old"},
			"A-Header": {"old"},
		},
	}

	current := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Z-Header": {"new"},
			"A-Header": {"new"},
		},
	}

	got := mustCompareCompleteBodies(
		t,
		baseline,
		current,
		Rules{},
	)

	if len(got.Differences) != 3 {
		t.Fatalf(
			"len(Differences) = %d, want 3",
			len(got.Differences),
		)
	}

	if got.Differences[0].Location != StatusLocation() {
		t.Fatalf(
			"first difference = %q, want %q",
			got.Differences[0].Location.String(),
			StatusLocation().String(),
		)
	}

	if got.Differences[1].Location != HeaderLocation("A-Header") {
		t.Fatalf(
			"second difference = %q, want %q",
			got.Differences[1].Location.String(),
			HeaderLocation("A-Header").String(),
		)
	}

	if got.Differences[2].Location != HeaderLocation("Z-Header") {
		t.Fatalf(
			"third difference = %q, want %q",
			got.Differences[2].Location.String(),
			HeaderLocation("Z-Header").String(),
		)
	}
}

func TestCompareNilAndEmptyHeadersAreEquivalent(t *testing.T) {
	baseline := recording.Response{
		StatusCode: http.StatusNoContent,
		Headers:    nil,
	}

	current := recording.Response{
		StatusCode: http.StatusNoContent,
		Headers:    http.Header{},
	}

	got := mustCompareCompleteBodies(
		t,
		baseline,
		current,
		Rules{},
	)

	if !got.Equivalent() {
		t.Fatalf(
			"Compare() differences = %#v, want equivalent",
			got.Differences,
		)
	}
}

func TestCompareHeaderDifferencesOwnTheirValues(t *testing.T) {
	baselineValues := []string{"before"}
	currentValues := []string{"after"}

	baseline := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"X-Value": baselineValues,
		},
	}

	current := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"X-Value": currentValues,
		},
	}

	got := mustCompareCompleteBodies(
		t,
		baseline,
		current,
		Rules{},
	)

	if len(got.Differences) != 1 {
		t.Fatalf(
			"len(Differences) = %d, want 1",
			len(got.Differences),
		)
	}

	baselineValues[0] = "mutated baseline"
	currentValues[0] = "mutated current"

	wantBefore := []string{"before"}
	wantAfter := []string{"after"}

	if !reflect.DeepEqual(
		got.Differences[0].Before.Data,
		wantBefore,
	) {
		t.Fatalf(
			"Before.Data = %#v, want %#v",
			got.Differences[0].Before.Data,
			wantBefore,
		)
	}

	if !reflect.DeepEqual(
		got.Differences[0].After.Data,
		wantAfter,
	) {
		t.Fatalf(
			"After.Data = %#v, want %#v",
			got.Differences[0].After.Data,
			wantAfter,
		)
	}
}

func TestCompareMergesDifferentlyCasedHeaderKeysDeterministically(
	t *testing.T,
) {
	baseline := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"X-Test": {"upper"},
			"x-test": {"lower"},
		},
	}

	current := recording.Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"X-Test": {"upper", "lower"},
		},
	}

	got := mustCompareCompleteBodies(
		t,
		baseline,
		current,
		Rules{},
	)

	if !got.Equivalent() {
		t.Fatalf(
			"Compare() differences = %#v, want equivalent",
			got.Differences,
		)
	}
}
