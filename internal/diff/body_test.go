package diff

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
)

func TestCompareSemanticJSONIgnoresFormattingObjectOrderAndNumberSpelling(
	t *testing.T,
) {
	baseline := completeBodyResponse(
		[]byte(`{"name":"Vini","enabled":true,"count":1}`),
	)

	current := completeBodyResponse(
		[]byte(`{
			"count": 1.0e0,
			"enabled": true,
			"name": "Vini"
		}`),
	)

	got, err := Compare(
		baseline,
		current,
		Rules{},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	if !got.Equivalent() {
		t.Fatalf(
			"Compare() differences = %#v, want equivalent",
			got.Differences,
		)
	}
}

func TestCompareJSONDifferencesAreDeterministic(
	t *testing.T,
) {
	baseline := completeBodyResponse(
		[]byte(`{
			"removed": "old",
			"changed": 1,
			"nested": {
				"z": true
			}
		}`),
	)

	current := completeBodyResponse(
		[]byte(`{
			"added": "new",
			"changed": 2,
			"nested": {
				"z": false
			}
		}`),
	)

	got, err := Compare(
		baseline,
		current,
		Rules{},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	want := Comparison{
		Differences: []Difference{
			{
				Kind:     KindFieldAdded,
				Location: BodyLocation("/added"),
				Before:   MissingValue(),
				After:    ValueOf("new"),
			},
			{
				Kind:     KindValueChanged,
				Location: BodyLocation("/changed"),
				Before:   ValueOf(json.Number("1")),
				After:    ValueOf(json.Number("2")),
			},
			{
				Kind:     KindValueChanged,
				Location: BodyLocation("/nested/z"),
				Before:   ValueOf(true),
				After:    ValueOf(false),
			},
			{
				Kind:     KindFieldRemoved,
				Location: BodyLocation("/removed"),
				Before:   ValueOf("old"),
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

func TestCompareJSONDistinguishesNullFromMissing(
	t *testing.T,
) {
	baseline := completeBodyResponse(
		[]byte(`{"nickname":null}`),
	)

	current := completeBodyResponse(
		[]byte(`{}`),
	)

	got, err := Compare(
		baseline,
		current,
		Rules{},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	want := Comparison{
		Differences: []Difference{
			{
				Kind:     KindFieldRemoved,
				Location: BodyLocation("/nickname"),
				Before:   ValueOf(nil),
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

func TestCompareJSONTypeChanged(
	t *testing.T,
) {
	baseline := completeBodyResponse(
		[]byte(`{"value":1}`),
	)

	current := completeBodyResponse(
		[]byte(`{"value":"1"}`),
	)

	got, err := Compare(
		baseline,
		current,
		Rules{},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	want := Comparison{
		Differences: []Difference{
			{
				Kind:     KindTypeChanged,
				Location: BodyLocation("/value"),
				Before:   ValueOf(json.Number("1")),
				After:    ValueOf("1"),
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

func TestCompareJSONArraysPositionally(
	t *testing.T,
) {
	baseline := completeBodyResponse(
		[]byte(`[1,2,3]`),
	)

	current := completeBodyResponse(
		[]byte(`[1,4,3,5]`),
	)

	got, err := Compare(
		baseline,
		current,
		Rules{},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	want := Comparison{
		Differences: []Difference{
			{
				Kind:     KindValueChanged,
				Location: BodyLocation("/1"),
				Before:   ValueOf(json.Number("2")),
				After:    ValueOf(json.Number("4")),
			},
			{
				Kind:     KindElementAdded,
				Location: BodyLocation("/3"),
				Before:   MissingValue(),
				After:    ValueOf(json.Number("5")),
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

func TestCompareJSONArrayElementRemoved(
	t *testing.T,
) {
	baseline := completeBodyResponse(
		[]byte(`[1,2]`),
	)

	current := completeBodyResponse(
		[]byte(`[1]`),
	)

	got, err := Compare(
		baseline,
		current,
		Rules{},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	want := Comparison{
		Differences: []Difference{
			{
				Kind:     KindElementRemoved,
				Location: BodyLocation("/1"),
				Before:   ValueOf(json.Number("2")),
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

func TestCompareJSONNestedIgnore(
	t *testing.T,
) {
	baseline := completeBodyResponse(
		[]byte(`{
			"name": "before",
			"metadata": {
				"request_id": "old",
				"stable": true
			}
		}`),
	)

	current := completeBodyResponse(
		[]byte(`{
			"name": "after",
			"metadata": {
				"request_id": "new",
				"stable": true
			}
		}`),
	)

	got, err := Compare(
		baseline,
		current,
		Rules{
			Ignore: []Location{
				BodyLocation(
					"/metadata/request_id",
				),
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	want := Comparison{
		Differences: []Difference{
			{
				Kind:     KindValueChanged,
				Location: BodyLocation("/name"),
				Before:   ValueOf("before"),
				After:    ValueOf("after"),
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

func TestCompareJSONPointerEscapesObjectKeys(
	t *testing.T,
) {
	baseline := completeBodyResponse(
		[]byte(`{
			"a/b": {
				"~key": 1
			}
		}`),
	)

	current := completeBodyResponse(
		[]byte(`{
			"a/b": {
				"~key": 2
			}
		}`),
	)

	got, err := Compare(
		baseline,
		current,
		Rules{},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	if len(got.Differences) != 1 {
		t.Fatalf(
			"len(Differences) = %d, want 1",
			len(got.Differences),
		)
	}

	want := BodyLocation(
		"/a~1b/~0key",
	)

	if got.Differences[0].Location != want {
		t.Fatalf(
			"Location = %q, want %q",
			got.Differences[0].Location.String(),
			want.String(),
		)
	}
}

func TestCompareJSONNumbersUseExactSemanticComparison(
	t *testing.T,
) {
	t.Run("equivalent spellings", func(t *testing.T) {
		baseline := completeBodyResponse(
			[]byte(`{"number":1}`),
		)

		current := completeBodyResponse(
			[]byte(`{"number":1.000e0}`),
		)

		got, err := Compare(
			baseline,
			current,
			Rules{},
		)
		if err != nil {
			t.Fatalf(
				"Compare() error = %v, want nil",
				err,
			)
		}

		if !got.Equivalent() {
			t.Fatalf(
				"Compare() differences = %#v, want equivalent",
				got.Differences,
			)
		}
	})

	t.Run("large integers remain distinct", func(t *testing.T) {
		baseline := completeBodyResponse(
			[]byte(
				`{"number":9007199254740992}`,
			),
		)

		current := completeBodyResponse(
			[]byte(
				`{"number":9007199254740993}`,
			),
		)

		got, err := Compare(
			baseline,
			current,
			Rules{},
		)
		if err != nil {
			t.Fatalf(
				"Compare() error = %v, want nil",
				err,
			)
		}

		if len(got.Differences) != 1 {
			t.Fatalf(
				"len(Differences) = %d, want 1",
				len(got.Differences),
			)
		}

		difference := got.Differences[0]

		if difference.Kind != KindValueChanged {
			t.Fatalf(
				"Kind = %q, want %q",
				difference.Kind,
				KindValueChanged,
			)
		}

		if difference.Before.Data != json.Number(
			"9007199254740992",
		) {
			t.Fatalf(
				"Before.Data = %#v, want exact baseline number",
				difference.Before.Data,
			)
		}

		if difference.After.Data != json.Number(
			"9007199254740993",
		) {
			t.Fatalf(
				"After.Data = %#v, want exact current number",
				difference.After.Data,
			)
		}
	})
}

func TestCompareValidJSONDoesNotRequireJSONContentType(
	t *testing.T,
) {
	baseline := completeBodyResponse(
		[]byte(`{"value":1}`),
	)
	baseline.Headers = http.Header{
		"Content-Type": {"text/plain"},
	}

	current := completeBodyResponse(
		[]byte(`{"value":1.0}`),
	)
	current.Headers = http.Header{
		"Content-Type": {"text/plain"},
	}

	got, err := Compare(
		baseline,
		current,
		Rules{},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	if !got.Equivalent() {
		t.Fatalf(
			"Compare() differences = %#v, want equivalent",
			got.Differences,
		)
	}
}

func TestCompareInvalidJSONFallsBackToExactBodyComparison(
	t *testing.T,
) {
	baselineBody := []byte(
		`{"value":1`,
	)
	currentBody := []byte(
		`{"value":2`,
	)

	baseline := completeBodyResponse(
		baselineBody,
	)
	current := completeBodyResponse(
		currentBody,
	)

	got, err := Compare(
		baseline,
		current,
		Rules{},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	if len(got.Differences) != 1 {
		t.Fatalf(
			"len(Differences) = %d, want 1",
			len(got.Differences),
		)
	}

	difference := got.Differences[0]

	if difference.Kind != KindBodyChanged {
		t.Fatalf(
			"Kind = %q, want %q",
			difference.Kind,
			KindBodyChanged,
		)
	}

	if difference.Location != BodyLocation("") {
		t.Fatalf(
			"Location = %q, want %q",
			difference.Location.String(),
			BodyLocation("").String(),
		)
	}

	wantBefore := BodySnapshot{
		Size: int64(len(baselineBody)),
		SHA256: sha256.Sum256(
			baselineBody,
		),
	}

	wantAfter := BodySnapshot{
		Size: int64(len(currentBody)),
		SHA256: sha256.Sum256(
			currentBody,
		),
	}

	if difference.Before.Data != wantBefore {
		t.Fatalf(
			"Before.Data = %#v, want %#v",
			difference.Before.Data,
			wantBefore,
		)
	}

	if difference.After.Data != wantAfter {
		t.Fatalf(
			"After.Data = %#v, want %#v",
			difference.After.Data,
			wantAfter,
		)
	}
}

func TestCompareBinaryBodiesExactly(
	t *testing.T,
) {
	baselineBody := []byte{
		0x00,
		0x01,
		0x02,
	}

	currentBody := []byte{
		0x00,
		0x01,
		0x03,
	}

	baseline := completeBodyResponse(
		baselineBody,
	)
	current := completeBodyResponse(
		currentBody,
	)

	got, err := Compare(
		baseline,
		current,
		Rules{},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	if len(got.Differences) != 1 {
		t.Fatalf(
			"len(Differences) = %d, want 1",
			len(got.Differences),
		)
	}

	if got.Differences[0].Kind != KindBodyChanged {
		t.Fatalf(
			"Kind = %q, want %q",
			got.Differences[0].Kind,
			KindBodyChanged,
		)
	}
}

func TestCompareBodyUnavailable(
	t *testing.T,
) {
	tests := []struct {
		name string

		baseline recording.Response
		current  recording.Response

		wantSide      BodySide
		wantTruncated bool
		wantComplete  bool
	}{
		{
			name: "baseline truncated",
			baseline: recording.Response{
				StatusCode:   http.StatusOK,
				Body:         []byte("abcd"),
				ObservedSize: 10,
				Truncated:    true,
				Complete:     true,
			},
			current: completeBodyResponse(
				[]byte("abcdefghij"),
			),
			wantSide:      BodySideBaseline,
			wantTruncated: true,
			wantComplete:  true,
		},
		{
			name: "baseline incomplete",
			baseline: recording.Response{
				StatusCode:   http.StatusOK,
				Body:         []byte("abcd"),
				ObservedSize: 4,
				Truncated:    false,
				Complete:     false,
			},
			current: completeBodyResponse(
				[]byte("abcd"),
			),
			wantSide:      BodySideBaseline,
			wantTruncated: false,
			wantComplete:  false,
		},
		{
			name: "current truncated",
			baseline: completeBodyResponse(
				[]byte("abcdefghij"),
			),
			current: recording.Response{
				StatusCode:   http.StatusOK,
				Body:         []byte("abcd"),
				ObservedSize: 10,
				Truncated:    true,
				Complete:     true,
			},
			wantSide:      BodySideCurrent,
			wantTruncated: true,
			wantComplete:  true,
		},
		{
			name: "current incomplete",
			baseline: completeBodyResponse(
				[]byte("abcd"),
			),
			current: recording.Response{
				StatusCode:   http.StatusOK,
				Body:         []byte("abcd"),
				ObservedSize: 4,
				Truncated:    false,
				Complete:     false,
			},
			wantSide:      BodySideCurrent,
			wantTruncated: false,
			wantComplete:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compare(
				test.baseline,
				test.current,
				Rules{},
			)

			if err == nil {
				t.Fatal(
					"Compare() error = nil, want body unavailable error",
				)
			}

			if !errors.Is(
				err,
				ErrBodyUnavailable,
			) {
				t.Fatalf(
					"errors.Is(error, ErrBodyUnavailable) = false; error = %v",
					err,
				)
			}

			var unavailable *BodyUnavailableError
			if !errors.As(
				err,
				&unavailable,
			) {
				t.Fatalf(
					"errors.As(error, *BodyUnavailableError) = false; error = %v",
					err,
				)
			}

			if unavailable.Side != test.wantSide {
				t.Fatalf(
					"Side = %q, want %q",
					unavailable.Side,
					test.wantSide,
				)
			}

			if unavailable.Truncated != test.wantTruncated {
				t.Fatalf(
					"Truncated = %v, want %v",
					unavailable.Truncated,
					test.wantTruncated,
				)
			}

			if unavailable.Complete != test.wantComplete {
				t.Fatalf(
					"Complete = %v, want %v",
					unavailable.Complete,
					test.wantComplete,
				)
			}
		})
	}
}

func TestCompareIgnoredBodyDoesNotRequireCompleteCapture(
	t *testing.T,
) {
	baseline := recording.Response{
		StatusCode:   http.StatusOK,
		Body:         []byte("abcd"),
		ObservedSize: 100,
		Truncated:    true,
		Complete:     false,
	}

	current := completeBodyResponse(
		[]byte("completely different"),
	)

	got, err := Compare(
		baseline,
		current,
		Rules{
			Ignore: []Location{
				BodyLocation(""),
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	if !got.Equivalent() {
		t.Fatalf(
			"Compare() differences = %#v, want equivalent",
			got.Differences,
		)
	}
}

func TestCompareReturnsKnownDifferencesAlongsideBodyError(
	t *testing.T,
) {
	baseline := recording.Response{
		StatusCode:   http.StatusInternalServerError,
		Body:         []byte("partial"),
		ObservedSize: 100,
		Truncated:    true,
		Complete:     true,
	}

	current := completeBodyResponse(
		[]byte("complete"),
	)
	current.StatusCode = http.StatusOK

	got, err := Compare(
		baseline,
		current,
		Rules{},
	)

	if !errors.Is(
		err,
		ErrBodyUnavailable,
	) {
		t.Fatalf(
			"Compare() error = %v, want ErrBodyUnavailable",
			err,
		)
	}

	want := Comparison{
		Differences: []Difference{
			{
				Kind:     KindStatusChanged,
				Location: StatusLocation(),
				Before: ValueOf(
					http.StatusInternalServerError,
				),
				After: ValueOf(
					http.StatusOK,
				),
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

func TestCompareReportsBodyAfterStatusAndHeaders(
	t *testing.T,
) {
	baseline := completeBodyResponse(
		[]byte(`{"value":1}`),
	)
	baseline.StatusCode = http.StatusInternalServerError
	baseline.Headers = http.Header{
		"X-Version": {"old"},
	}

	current := completeBodyResponse(
		[]byte(`{"value":2}`),
	)
	current.StatusCode = http.StatusOK
	current.Headers = http.Header{
		"X-Version": {"new"},
	}

	got, err := Compare(
		baseline,
		current,
		Rules{},
	)
	if err != nil {
		t.Fatalf(
			"Compare() error = %v, want nil",
			err,
		)
	}

	if len(got.Differences) != 3 {
		t.Fatalf(
			"len(Differences) = %d, want 3",
			len(got.Differences),
		)
	}

	if got.Differences[0].Location != StatusLocation() {
		t.Fatalf(
			"first difference = %q, want status",
			got.Differences[0].Location.String(),
		)
	}

	if got.Differences[1].Location != HeaderLocation(
		"X-Version",
	) {
		t.Fatalf(
			"second difference = %q, want X-Version header",
			got.Differences[1].Location.String(),
		)
	}

	if got.Differences[2].Location != BodyLocation(
		"/value",
	) {
		t.Fatalf(
			"third difference = %q, want body /value",
			got.Differences[2].Location.String(),
		)
	}
}

func completeBodyResponse(
	body []byte,
) recording.Response {
	copied := append(
		[]byte(nil),
		body...,
	)

	return recording.Response{
		StatusCode:   http.StatusOK,
		Body:         copied,
		ObservedSize: int64(len(copied)),
		Truncated:    false,
		Complete:     true,
	}
}
