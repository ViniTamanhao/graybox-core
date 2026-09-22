package diff

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
)

// ErrBodyUnavailable means Graybox does not possess a complete response body
// and therefore cannot establish behavioral equivalence for that body.
var ErrBodyUnavailable = errors.New(
	"response body is not fully available",
)

// BodySide identifies which side of a comparison lacks a complete body.
type BodySide string

const (
	// BodySideBaseline identifies the recorded baseline response.
	BodySideBaseline BodySide = "baseline"

	// BodySideCurrent identifies the newly observed response.
	BodySideCurrent BodySide = "current"
)

// BodyUnavailableError describes why a response body cannot be compared.
//
// CapturedSize and ObservedSize retain the same meanings as schema 1:
// CapturedSize is the number of bytes Graybox retained, while ObservedSize is
// the number of bytes Graybox actually observed before persistence.
type BodyUnavailableError struct {
	Side         BodySide
	CapturedSize int64
	ObservedSize int64
	Truncated    bool
	Complete     bool
}

// Error returns a human-readable description of the unavailable body.
func (e *BodyUnavailableError) Error() string {
	switch {
	case e.Truncated && !e.Complete:
		return fmt.Sprintf(
			"%s response body cannot be compared: "+
				"capture is truncated (%d of %d observed bytes) "+
				"and the body stream was incomplete",
			e.Side,
			e.CapturedSize,
			e.ObservedSize,
		)

	case e.Truncated:
		return fmt.Sprintf(
			"%s response body cannot be compared: "+
				"capture is truncated (%d of %d observed bytes)",
			e.Side,
			e.CapturedSize,
			e.ObservedSize,
		)

	case !e.Complete:
		return fmt.Sprintf(
			"%s response body cannot be compared: "+
				"body stream was incomplete after %d observed bytes",
			e.Side,
			e.ObservedSize,
		)

	default:
		return fmt.Sprintf(
			"%s response body cannot be compared",
			e.Side,
		)
	}
}

// Unwrap allows callers to classify the error with errors.Is.
func (e *BodyUnavailableError) Unwrap() error {
	return ErrBodyUnavailable
}

// BodySnapshot is the compact representation stored for a changed body that is
// compared exactly rather than semantically as JSON.
//
// The comparison itself still uses every captured byte. The snapshot prevents
// a diff report from retaining another complete copy of potentially large
// request or response bodies merely to describe that they changed.
type BodySnapshot struct {
	Size   int64
	SHA256 [sha256.Size]byte
}

func compareBody(
	baseline recording.Response,
	current recording.Response,
	rules Rules,
) ([]Difference, error) {
	root := BodyLocation("")

	// Ignoring the entire body means body availability is irrelevant. This is
	// checked before completeness validation so a user can intentionally diff
	// only status and headers even when the original body was truncated.
	if rules.Ignores(root) {
		return nil, nil
	}

	if err := ensureComparableBody(
		BodySideBaseline,
		baseline,
	); err != nil {
		return nil, err
	}

	if err := ensureComparableBody(
		BodySideCurrent,
		current,
	); err != nil {
		return nil, err
	}

	// Exact equality is the common and cheapest case. It also avoids JSON
	// decoding when the representation is already byte-for-byte identical.
	if bytes.Equal(
		baseline.Body,
		current.Body,
	) {
		return nil, nil
	}

	baselineJSON, baselineIsJSON := decodeJSON(
		baseline.Body,
	)
	currentJSON, currentIsJSON := decodeJSON(
		current.Body,
	)

	if baselineIsJSON && currentIsJSON {
		return compareJSONValues(
			"",
			baselineJSON,
			currentJSON,
			rules,
		), nil
	}

	return []Difference{
		{
			Kind:     KindBodyChanged,
			Location: root,
			Before: ValueOf(
				bodySnapshot(baseline.Body),
			),
			After: ValueOf(
				bodySnapshot(current.Body),
			),
		},
	}, nil
}

func ensureComparableBody(
	side BodySide,
	response recording.Response,
) error {
	if !response.Truncated && response.Complete {
		return nil
	}

	return &BodyUnavailableError{
		Side:         side,
		CapturedSize: int64(len(response.Body)),
		ObservedSize: response.ObservedSize,
		Truncated:    response.Truncated,
		Complete:     response.Complete,
	}
}

func bodySnapshot(body []byte) BodySnapshot {
	return BodySnapshot{
		Size:   int64(len(body)),
		SHA256: sha256.Sum256(body),
	}
}

// decodeJSON decodes one complete JSON value while preserving JSON numbers as
// json.Number instead of float64.
//
// Preserving the original number token avoids float64 precision loss for large
// integers and high-precision decimal values.
func decodeJSON(body []byte) (any, bool) {
	if !json.Valid(body) {
		return nil, false
	}

	decoder := json.NewDecoder(
		bytes.NewReader(body),
	)
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}

	return value, true
}

func compareJSONValues(
	pointer string,
	baseline any,
	current any,
	rules Rules,
) []Difference {
	location := BodyLocation(pointer)

	if rules.Ignores(location) {
		return nil
	}

	// JSON null is represented by nil. Missing object fields and array
	// positions never arrive here: their absence is handled by the object and
	// array comparators before recursion.
	if baseline == nil || current == nil {
		if baseline == nil && current == nil {
			return nil
		}

		return jsonDifference(
			KindTypeChanged,
			pointer,
			ValueOf(baseline),
			ValueOf(current),
		)
	}

	// encoding/json with UseNumber produces a deliberately small set of Go
	// types. A different Go type therefore represents a different JSON type.
	if reflect.TypeOf(baseline) != reflect.TypeOf(current) {
		return jsonDifference(
			KindTypeChanged,
			pointer,
			ValueOf(baseline),
			ValueOf(current),
		)
	}

	switch baseline := baseline.(type) {
	case map[string]any:
		return compareJSONObjects(
			pointer,
			baseline,
			current.(map[string]any),
			rules,
		)

	case []any:
		return compareJSONArrays(
			pointer,
			baseline,
			current.([]any),
			rules,
		)

	case json.Number:
		if jsonNumbersEqual(
			baseline,
			current.(json.Number),
		) {
			return nil
		}

	case string:
		if baseline == current.(string) {
			return nil
		}

	case bool:
		if baseline == current.(bool) {
			return nil
		}

	default:
		// decodeJSON currently cannot produce another concrete value type.
		// Keeping this fallback makes the helper robust if encoding/json's
		// representation or this package's decoder changes in the future.
		if reflect.DeepEqual(
			baseline,
			current,
		) {
			return nil
		}
	}

	return jsonDifference(
		KindValueChanged,
		pointer,
		ValueOf(baseline),
		ValueOf(current),
	)
}

func compareJSONObjects(
	pointer string,
	baseline map[string]any,
	current map[string]any,
	rules Rules,
) []Difference {
	keys := jsonObjectKeys(
		baseline,
		current,
	)

	var differences []Difference

	for _, key := range keys {
		childPointer := AppendJSONPointer(
			pointer,
			key,
		)
		location := BodyLocation(childPointer)

		if rules.Ignores(location) {
			continue
		}

		baselineValue, baselinePresent := baseline[key]
		currentValue, currentPresent := current[key]

		switch {
		case !baselinePresent && currentPresent:
			differences = append(
				differences,
				Difference{
					Kind:     KindFieldAdded,
					Location: location,
					Before:   MissingValue(),
					After:    ValueOf(currentValue),
				},
			)

		case baselinePresent && !currentPresent:
			differences = append(
				differences,
				Difference{
					Kind:     KindFieldRemoved,
					Location: location,
					Before:   ValueOf(baselineValue),
					After:    MissingValue(),
				},
			)

		default:
			differences = append(
				differences,
				compareJSONValues(
					childPointer,
					baselineValue,
					currentValue,
					rules,
				)...,
			)
		}
	}

	return differences
}

func jsonObjectKeys(
	baseline map[string]any,
	current map[string]any,
) []string {
	set := make(
		map[string]struct{},
		len(baseline)+len(current),
	)

	for key := range baseline {
		set[key] = struct{}{}
	}

	for key := range current {
		set[key] = struct{}{}
	}

	keys := make([]string, 0, len(set))

	for key := range set {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func compareJSONArrays(
	pointer string,
	baseline []any,
	current []any,
	rules Rules,
) []Difference {
	shared := min(
		len(baseline),
		len(current),
	)

	var differences []Difference

	for index := 0; index < shared; index++ {
		childPointer := AppendJSONPointer(
			pointer,
			strconv.Itoa(index),
		)

		differences = append(
			differences,
			compareJSONValues(
				childPointer,
				baseline[index],
				current[index],
				rules,
			)...,
		)
	}

	for index := shared; index < len(baseline); index++ {
		childPointer := AppendJSONPointer(
			pointer,
			strconv.Itoa(index),
		)
		location := BodyLocation(childPointer)

		if rules.Ignores(location) {
			continue
		}

		differences = append(
			differences,
			Difference{
				Kind:     KindElementRemoved,
				Location: location,
				Before:   ValueOf(baseline[index]),
				After:    MissingValue(),
			},
		)
	}

	for index := shared; index < len(current); index++ {
		childPointer := AppendJSONPointer(
			pointer,
			strconv.Itoa(index),
		)
		location := BodyLocation(childPointer)

		if rules.Ignores(location) {
			continue
		}

		differences = append(
			differences,
			Difference{
				Kind:     KindElementAdded,
				Location: location,
				Before:   MissingValue(),
				After:    ValueOf(current[index]),
			},
		)
	}

	return differences
}

func jsonDifference(
	kind DifferenceKind,
	pointer string,
	before Value,
	after Value,
) []Difference {
	return []Difference{
		{
			Kind:     kind,
			Location: BodyLocation(pointer),
			Before:   before,
			After:    after,
		},
	}
}

type canonicalJSONNumber struct {
	Negative bool
	Digits   string
	Exponent string
}

func jsonNumbersEqual(
	baseline json.Number,
	current json.Number,
) bool {
	if baseline == current {
		return true
	}

	baselineCanonical, baselineOK := canonicalizeJSONNumber(
		baseline,
	)
	currentCanonical, currentOK := canonicalizeJSONNumber(
		current,
	)

	if !baselineOK || !currentOK {
		// Numbers reaching this function normally originate from
		// encoding/json, so canonicalization should succeed. Falling back to
		// token equality is safer than guessing if that invariant ever changes.
		return baseline.String() == current.String()
	}

	return baselineCanonical == currentCanonical
}

func canonicalizeJSONNumber(
	number json.Number,
) (canonicalJSONNumber, bool) {
	raw := number.String()
	if raw == "" {
		return canonicalJSONNumber{}, false
	}

	negative := false

	if raw[0] == '-' {
		negative = true
		raw = raw[1:]

		if raw == "" {
			return canonicalJSONNumber{}, false
		}
	}

	mantissa := raw
	exponentText := "0"

	if index := strings.IndexAny(raw, "eE"); index >= 0 {
		mantissa = raw[:index]
		exponentText = raw[index+1:]

		if mantissa == "" || exponentText == "" {
			return canonicalJSONNumber{}, false
		}
	}

	exponent := new(big.Int)
	if _, ok := exponent.SetString(
		exponentText,
		10,
	); !ok {
		return canonicalJSONNumber{}, false
	}

	integerPart := mantissa
	fractionPart := ""

	if index := strings.IndexByte(
		mantissa,
		'.',
	); index >= 0 {
		integerPart = mantissa[:index]
		fractionPart = mantissa[index+1:]

		if integerPart == "" || fractionPart == "" {
			return canonicalJSONNumber{}, false
		}
	}

	if !decimalDigits(integerPart) ||
		(fractionPart != "" && !decimalDigits(fractionPart)) {
		return canonicalJSONNumber{}, false
	}

	digits := integerPart + fractionPart

	// Leading zeroes do not contribute to the numeric value.
	digits = strings.TrimLeft(
		digits,
		"0",
	)

	// Every representation of zero canonicalizes identically, including -0,
	// -0.0, and 0e999999999.
	if digits == "" {
		return canonicalJSONNumber{
			Negative: false,
			Digits:   "0",
			Exponent: "0",
		}, true
	}

	// Combining the integer and fractional portions implicitly moves the
	// decimal point to the end. Compensate for that by subtracting the number
	// of fractional digits from the explicit exponent.
	exponent.Sub(
		exponent,
		big.NewInt(
			int64(len(fractionPart)),
		),
	)

	// Trailing zeroes in the coefficient can be moved into the decimal
	// exponent without changing the numeric value.
	trimmed := strings.TrimRight(
		digits,
		"0",
	)
	trailingZeroes := len(digits) - len(trimmed)

	digits = trimmed

	exponent.Add(
		exponent,
		big.NewInt(
			int64(trailingZeroes),
		),
	)

	return canonicalJSONNumber{
		Negative: negative,
		Digits:   digits,
		Exponent: exponent.String(),
	}, true
}

func decimalDigits(value string) bool {
	if value == "" {
		return false
	}

	for _, digit := range []byte(value) {
		if digit < '0' || digit > '9' {
			return false
		}
	}

	return true
}
