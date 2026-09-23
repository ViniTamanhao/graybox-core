package cli

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ViniTamanhao/graybox-core/internal/diff"
	"github.com/ViniTamanhao/graybox-core/internal/replay"
	"github.com/ViniTamanhao/graybox-core/internal/storage"
)

const maxHumanDiffValueRunes = 512

type stringListFlag []string

func (values *stringListFlag) String() string {
	if values == nil {
		return ""
	}

	return strings.Join(
		*values,
		",",
	)
}

func (values *stringListFlag) Set(
	value string,
) error {
	if strings.TrimSpace(
		value,
	) == "" {
		return errors.New(
			"ignore location must not be empty",
		)
	}

	*values = append(
		*values,
		value,
	)

	return nil
}

type diffSummaryJSON struct {
	Total      int `json:"total"`
	Equivalent int `json:"equivalent"`
	Changed    int `json:"changed"`
	Failed     int `json:"failed"`
}

type diffLocationJSON struct {
	Component string `json:"component"`
	Path      string `json:"path"`
}

type diffValueJSON struct {
	Present bool `json:"present"`
	Value   any  `json:"value"`
}

type diffBodySnapshotJSON struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type diffDifferenceJSON struct {
	Kind     string           `json:"kind"`
	Location diffLocationJSON `json:"location"`
	Before   diffValueJSON    `json:"before"`
	After    diffValueJSON    `json:"after"`
}

type diffResultJSON struct {
	ExchangeID int64  `json:"exchange_id"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Outcome    string `json:"outcome"`

	BaselineDurationMS float64 `json:"baseline_duration_ms"`
	CurrentDurationMS  float64 `json:"current_duration_ms"`

	Differences []diffDifferenceJSON `json:"differences"`
	Error       string               `json:"error,omitempty"`
}

type diffReportJSON struct {
	Recording        string           `json:"recording"`
	Target           string           `json:"target"`
	BodyCaptureLimit int64            `json:"body_capture_limit"`
	Ignored          []string         `json:"ignored"`
	Summary          diffSummaryJSON  `json:"summary"`
	Results          []diffResultJSON `json:"results"`
}

func (a App) runDiff(
	ctx context.Context,
	args []string,
) (int, error) {
	var targetValue string
	var idValue int64
	var ignoreValues stringListFlag
	var jsonOutput bool
	var unsafeOriginalTarget bool
	var help bool

	usage := func() {
		fmt.Fprint(
			a.Stdout,
			`Usage: graybox diff RECORDING [options]

Replay recorded requests and compare the live responses with the recorded
baseline. Status, response headers, and bodies are compared. Valid JSON bodies
are compared semantically; other bodies are compared exactly.

By default Graybox ignores Date and Content-Length response headers.

Options:
  --id ID            compare only one exchange
  --target URL       replace the original target
  --ignore LOCATION  ignore a response location; may be repeated
  --unsafe-original-target
                     allow an omitted --target to use a saved remote target
  --json             emit structured JSON
  -h, --help         show this help

Ignore locations:
  response.status
  response.headers
  response.headers.NAME
  response.body
  response.body#/JSON/POINTER

Examples:
  graybox diff bug.graybox
  graybox diff bug.graybox --id 42 --target http://localhost:8081
  graybox diff bug.graybox --ignore 'response.body#/metadata/request_id'
  graybox diff bug.graybox --json
`,
		)
	}

	fs := a.newFlagSet(
		"diff",
		usage,
	)

	fs.Int64Var(
		&idValue,
		"id",
		0,
		"",
	)

	fs.StringVar(
		&targetValue,
		"target",
		"",
		"",
	)

	fs.Var(
		&ignoreValues,
		"ignore",
		"",
	)

	fs.BoolVar(
		&unsafeOriginalTarget,
		"unsafe-original-target",
		false,
		"",
	)

	fs.BoolVar(
		&jsonOutput,
		"json",
		false,
		"",
	)

	fs.BoolVar(
		&help,
		"help",
		false,
		"",
	)

	fs.BoolVar(
		&help,
		"h",
		false,
		"",
	)

	positional, err := parseInterspersed(
		fs,
		args,
		map[string]bool{
			"json":                   true,
			"unsafe-original-target": true,
			"help":                   true,
			"h":                      true,
		},
	)
	if err != nil {
		return ExitUsage, usageError{
			err.Error(),
		}
	}

	if help {
		usage()

		return ExitSuccess, nil
	}

	if len(positional) != 1 {
		return ExitUsage, usageError{
			"diff requires exactly one recording file",
		}
	}

	if flagWasSet(fs, "id") && idValue <= 0 {
		return ExitUsage, usageError{
			"--id must be a positive integer",
		}
	}

	rules, ignored, err := buildDiffRules(
		ignoreValues,
	)
	if err != nil {
		return ExitUsage, err
	}

	recordingPath := positional[0]

	store, err := storage.OpenReadOnly(
		ctx,
		recordingPath,
	)
	if err != nil {
		return classifyError(err), fmt.Errorf(
			"cannot open recording %q: %w",
			recordingPath,
			err,
		)
	}

	defer store.Close()

	target, err := resolveExecutionTarget(
		ctx,
		store,
		targetValue,
		unsafeOriginalTarget,
	)
	if err != nil {
		return classifyError(err), err
	}

	bodyLimit, err := diffResponseBodyLimit(
		ctx,
		store,
	)
	if err != nil {
		return classifyError(err), err
	}

	var selectedID *int64

	if idValue > 0 {
		selectedID = &idValue
	}

	report, err := (diff.Runner{
		Replayer: replay.Runner{
			Source:            store,
			ResponseBodyLimit: bodyLimit,
		},
		Rules: rules,
	}).Run(
		ctx,
		target,
		selectedID,
	)

	if errors.Is(
		err,
		storage.ErrNotFound,
	) &&
		selectedID != nil {
		return ExitInvalidRecording, fmt.Errorf(
			"recording %q has no exchange %d",
			recordingPath,
			idValue,
		)
	}

	if err != nil {
		return classifyError(err), err
	}

	if jsonOutput {
		if err := writeJSON(
			a.Stdout,
			toDiffReportJSON(
				recordingPath,
				target.String(),
				bodyLimit,
				ignored,
				report,
			),
		); err != nil {
			return ExitInternal, fmt.Errorf(
				"write output: %w",
				err,
			)
		}
	} else {
		if err := writeDiffHuman(
			a.Stdout,
			target.String(),
			ignored,
			report,
		); err != nil {
			return ExitInternal, fmt.Errorf(
				"write output: %w",
				err,
			)
		}
	}

	return diffExitCode(
		report,
	), nil
}

func defaultDiffIgnoredLocations() []diff.Location {
	return []diff.Location{
		diff.HeaderLocation(
			"Date",
		),
		diff.HeaderLocation(
			"Content-Length",
		),
	}
}

func buildDiffRules(
	values []string,
) (diff.Rules, []string, error) {
	var locations []diff.Location
	var display []string

	seen := make(
		map[diff.Location]struct{},
	)

	appendLocation := func(
		location diff.Location,
	) {
		if _, exists := seen[location]; exists {
			return
		}

		seen[location] = struct{}{}

		locations = append(
			locations,
			location,
		)

		display = append(
			display,
			location.String(),
		)
	}

	for _, location := range defaultDiffIgnoredLocations() {
		appendLocation(
			location,
		)
	}

	for _, value := range values {
		location, err := parseDiffIgnoreLocation(
			value,
		)
		if err != nil {
			return diff.Rules{}, nil, err
		}

		appendLocation(
			location,
		)
	}

	return diff.Rules{
		Ignore: locations,
	}, display, nil
}

func parseDiffIgnoreLocation(
	value string,
) (diff.Location, error) {
	value = strings.TrimSpace(
		value,
	)

	switch {
	case value == "response.status":
		return diff.StatusLocation(), nil

	case value == "response.headers":
		return diff.Location{
			Component: diff.ComponentHeaders,
		}, nil

	case strings.HasPrefix(
		value,
		"response.headers.",
	):
		name := strings.TrimSpace(
			strings.TrimPrefix(
				value,
				"response.headers.",
			),
		)

		if name == "" {
			break
		}

		return diff.HeaderLocation(
			name,
		), nil

	case value == "response.body":
		return diff.BodyLocation(
			"",
		), nil

	case strings.HasPrefix(
		value,
		"response.body#",
	):
		pointer := strings.TrimPrefix(
			value,
			"response.body#",
		)

		if validJSONPointer(
			pointer,
		) {
			return diff.BodyLocation(
				pointer,
			), nil
		}
	}

	return diff.Location{}, usageError{
		fmt.Sprintf(
			"invalid --ignore location %q; expected "+
				"response.status, response.headers, "+
				"response.headers.NAME, response.body, "+
				"or response.body#/JSON/POINTER",
			value,
		),
	}
}

func validJSONPointer(
	pointer string,
) bool {
	if pointer == "" ||
		pointer[0] != '/' {
		return false
	}

	for index := 0; index < len(pointer); index++ {
		if pointer[index] != '~' {
			continue
		}

		if index+1 >= len(pointer) ||
			(pointer[index+1] != '0' &&
				pointer[index+1] != '1') {
			return false
		}

		index++
	}

	return true
}

func diffResponseBodyLimit(
	ctx context.Context,
	metadata metadataReader,
) (int64, error) {
	value, err := metadata.Metadata(
		ctx,
		"body_capture_limit",
	)

	if errors.Is(
		err,
		storage.ErrNotFound,
	) {
		return replay.DefaultResponseBodyCaptureLimit, nil
	}

	if err != nil {
		return 0, fmt.Errorf(
			"read recording body capture limit: %w",
			err,
		)
	}

	value = strings.TrimSpace(
		value,
	)

	limit, err := strconv.ParseInt(
		value,
		10,
		64,
	)

	if err != nil ||
		limit <= 0 {
		return 0, fmt.Errorf(
			"%w: invalid body_capture_limit metadata %q",
			storage.ErrInvalidRecording,
			value,
		)
	}

	return limit, nil
}

func diffExitCode(
	report diff.Report,
) int {
	summary := report.Summary()

	if summary.Failed > 0 {
		return ExitComparisonFailed
	}

	if summary.Changed > 0 {
		return ExitBehaviorChanged
	}

	return ExitSuccess
}

func toDiffReportJSON(
	recordingPath string,
	target string,
	bodyLimit int64,
	ignored []string,
	report diff.Report,
) diffReportJSON {
	summary := report.Summary()

	results := make(
		[]diffResultJSON,
		0,
		len(report.Results),
	)

	for _, result := range report.Results {
		results = append(
			results,
			toDiffResultJSON(
				result,
			),
		)
	}

	return diffReportJSON{
		Recording:        recordingPath,
		Target:           target,
		BodyCaptureLimit: bodyLimit,
		Ignored: append(
			[]string{},
			ignored...,
		),
		Summary: diffSummaryJSON{
			Total:      summary.Total,
			Equivalent: summary.Equivalent,
			Changed:    summary.Changed,
			Failed:     summary.Failed,
		},
		Results: results,
	}
}

func toDiffResultJSON(
	result diff.ExchangeResult,
) diffResultJSON {
	differences := make(
		[]diffDifferenceJSON,
		0,
		len(result.Comparison.Differences),
	)

	for _, difference := range result.Comparison.Differences {
		differences = append(
			differences,
			diffDifferenceJSON{
				Kind: string(
					difference.Kind,
				),
				Location: diffLocationJSON{
					Component: string(
						difference.Location.Component,
					),
					Path: difference.Location.Path,
				},
				Before: toDiffValueJSON(
					difference.Before,
				),
				After: toDiffValueJSON(
					difference.After,
				),
			},
		)
	}

	item := diffResultJSON{
		ExchangeID: result.ExchangeID,
		Method:     result.Method,
		Path:       result.Path,
		Outcome: string(
			result.Outcome(),
		),
		BaselineDurationMS: durationMS(
			result.BaselineDuration,
		),
		CurrentDurationMS: durationMS(
			result.CurrentDuration,
		),
		Differences: differences,
	}

	if result.Err != nil {
		item.Error = result.Err.Error()
	}

	return item
}

func toDiffValueJSON(
	value diff.Value,
) diffValueJSON {
	if !value.Present {
		return diffValueJSON{
			Present: false,
			Value:   nil,
		}
	}

	return diffValueJSON{
		Present: true,
		Value: toDiffJSONData(
			value.Data,
		),
	}
}

func toDiffJSONData(
	value any,
) any {
	switch typed := value.(type) {
	case diff.BodySnapshot:
		return diffBodySnapshotJSON{
			Size: typed.Size,
			SHA256: hex.EncodeToString(
				typed.SHA256[:],
			),
		}

	default:
		return value
	}
}

func writeDiffHuman(
	writer io.Writer,
	target string,
	ignored []string,
	report diff.Report,
) error {
	var output strings.Builder

	summary := report.Summary()

	fmt.Fprintf(
		&output,
		"Diffing %d exchanges against %s\n",
		summary.Total,
		target,
	)

	if len(ignored) > 0 {
		fmt.Fprintf(
			&output,
			"Ignoring: %s\n",
			strings.Join(
				ignored,
				", ",
			),
		)
	}

	output.WriteString(
		"\n",
	)

	for _, result := range report.Results {
		fmt.Fprintf(
			&output,
			"%d %s %s\n",
			result.ExchangeID,
			strings.ToUpper(
				result.Method,
			),
			displayPath(
				result.Path,
			),
		)

		fmt.Fprintf(
			&output,
			"  %s\n",
			result.Outcome(),
		)

		for _, difference := range result.Comparison.Differences {
			fmt.Fprintf(
				&output,
				"  %s\n",
				difference.Location.String(),
			)

			fmt.Fprintf(
				&output,
				"    %s -> %s\n",
				formatDiffValue(
					difference.Before,
				),
				formatDiffValue(
					difference.After,
				),
			)
		}

		if result.Err != nil {
			fmt.Fprintf(
				&output,
				"  Error: %v\n",
				result.Err,
			)
		}

		output.WriteString(
			"\n",
		)
	}

	fmt.Fprintf(
		&output,
		"%d equivalent\n%d changed\n%d failed\n",
		summary.Equivalent,
		summary.Changed,
		summary.Failed,
	)

	_, err := io.WriteString(
		writer,
		output.String(),
	)

	return err
}

func formatDiffValue(
	value diff.Value,
) string {
	if !value.Present {
		return "<missing>"
	}

	if snapshot, ok := value.Data.(diff.BodySnapshot); ok {
		return fmt.Sprintf(
			"body(size=%d sha256=%s)",
			snapshot.Size,
			hex.EncodeToString(
				snapshot.SHA256[:],
			),
		)
	}

	encoded, err := json.Marshal(
		value.Data,
	)
	if err != nil {
		return truncateDiffDisplay(
			fmt.Sprint(
				value.Data,
			),
		)
	}

	return truncateDiffDisplay(
		string(encoded),
	)
}

func truncateDiffDisplay(
	value string,
) string {
	runes := []rune(
		value,
	)

	if len(runes) <= maxHumanDiffValueRunes {
		return value
	}

	return string(
		runes[:maxHumanDiffValueRunes],
	) + "..."
}
