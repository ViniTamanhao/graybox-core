package diff

import (
	"net/http"
	"slices"
	"sort"

	"github.com/ViniTamanhao/graybox-core/internal/recording"
)

// Compare compares the observable behavior of two HTTP responses according to
// the supplied rules.
//
// The comparison is deterministic: status differences are reported first,
// followed by header differences ordered by normalized header name.
//
// Body comparison is intentionally added separately. Keeping status/header
// comparison independent makes the comparison engine testable before replay
// begins retaining complete response bodies.
func Compare(
	baseline recording.Response,
	current recording.Response,
	rules Rules,
) Comparison {
	var differences []Difference

	if difference, changed := compareStatus(
		baseline.StatusCode,
		current.StatusCode,
		rules,
	); changed {
		differences = append(differences, difference)
	}

	differences = append(
		differences,
		compareHeaders(
			baseline.Headers,
			current.Headers,
			rules,
		)...,
	)

	return Comparison{
		Differences: differences,
	}
}

func compareStatus(
	baseline int,
	current int,
	rules Rules,
) (Difference, bool) {
	location := StatusLocation()

	if rules.Ignores(location) {
		return Difference{}, false
	}

	if baseline == current {
		return Difference{}, false
	}

	return Difference{
		Kind:     KindStatusChanged,
		Location: location,
		Before:   ValueOf(baseline),
		After:    ValueOf(current),
	}, true
}

func compareHeaders(
	baseline http.Header,
	current http.Header,
	rules Rules,
) []Difference {
	baselineHeaders := normalizeHeaders(baseline)
	currentHeaders := normalizeHeaders(current)

	names := headerNames(
		baselineHeaders,
		currentHeaders,
	)

	var differences []Difference

	for _, name := range names {
		location := HeaderLocation(name)

		if rules.Ignores(location) {
			continue
		}

		baselineValues, baselinePresent := baselineHeaders[name]
		currentValues, currentPresent := currentHeaders[name]

		switch {
		case !baselinePresent && currentPresent:
			differences = append(
				differences,
				Difference{
					Kind:     KindHeaderAdded,
					Location: location,
					Before:   MissingValue(),
					After:    headerValue(currentValues),
				},
			)

		case baselinePresent && !currentPresent:
			differences = append(
				differences,
				Difference{
					Kind:     KindHeaderRemoved,
					Location: location,
					Before:   headerValue(baselineValues),
					After:    MissingValue(),
				},
			)

		case !slices.Equal(baselineValues, currentValues):
			differences = append(
				differences,
				Difference{
					Kind:     KindHeaderChanged,
					Location: location,
					Before:   headerValue(baselineValues),
					After:    headerValue(currentValues),
				},
			)
		}
	}

	return differences
}

// normalizeHeaders converts HTTP header names into the case-insensitive form
// used by Graybox's diff domain.
//
// Values remain in their observed order. Schema 1 deliberately preserves
// repeated-header order, so the comparison engine preserves that information
// rather than sorting or joining values.
//
// Although net/http normally canonicalizes header names, http.Header is a map
// and callers may construct differently-cased keys manually. When multiple
// keys normalize to the same name, Graybox merges them deterministically.
func normalizeHeaders(headers http.Header) map[string][]string {
	if len(headers) == 0 {
		return nil
	}

	originalNames := make([]string, 0, len(headers))
	for name := range headers {
		originalNames = append(originalNames, name)
	}

	sort.Slice(originalNames, func(i, j int) bool {
		left := HeaderLocation(originalNames[i]).Path
		right := HeaderLocation(originalNames[j]).Path

		if left == right {
			return originalNames[i] < originalNames[j]
		}

		return left < right
	})

	normalized := make(
		map[string][]string,
		len(headers),
	)

	for _, originalName := range originalNames {
		name := HeaderLocation(originalName).Path

		normalized[name] = append(
			normalized[name],
			headers[originalName]...,
		)
	}

	return normalized
}

// headerNames returns the sorted union of header names from both responses.
//
// Stable ordering is important because diff results eventually feed human
// output, JSON output, CI, and coding agents. Map iteration order must never
// affect Graybox's observable diff result.
func headerNames(
	baseline map[string][]string,
	current map[string][]string,
) []string {
	set := make(
		map[string]struct{},
		len(baseline)+len(current),
	)

	for name := range baseline {
		set[name] = struct{}{}
	}

	for name := range current {
		set[name] = struct{}{}
	}

	names := make([]string, 0, len(set))

	for name := range set {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// headerValue creates a stable snapshot for a Difference.
//
// Difference values must not alias a mutable http.Header owned by some other
// component. Otherwise mutating a response after comparison could silently
// mutate an already-produced diff result.
func headerValue(values []string) Value {
	return ValueOf(slices.Clone(values))
}
