package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ViniTamanhao/graybox-core/internal/replay"
	"github.com/ViniTamanhao/graybox-core/internal/storage"
)

type replayItemJSON struct {
	ExchangeID int64   `json:"exchange_id"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	TargetURL  string  `json:"target_url"`
	Status     int     `json:"status,omitempty"`
	StatusText string  `json:"status_text,omitempty"`
	DurationMS float64 `json:"duration_ms"`
	Error      string  `json:"error,omitempty"`
}

func (a App) runReplay(
	ctx context.Context,
	args []string,
) (int, error) {
	var targetValue string
	var idValue int64
	var secretHeaderValues secretHeaderFlag
	var jsonOutput bool
	var unsafeOriginalTarget bool
	var help bool

	usage := func() {
		fmt.Fprint(
			a.Stdout,
			`Usage: graybox replay RECORDING [options]

Replay recorded requests sequentially. If --target is omitted, Graybox uses
the saved upstream target only when it is a loopback address. Redirects are
not followed, truncated request bodies are refused, and redacted credentials
and hop-by-hop headers are omitted.

Runtime credentials can be supplied explicitly from environment variables with
--secret-header. Runtime secret values are applied only to outgoing requests
and are never written back to the recording.

Options:
  --id ID            replay only one exchange
  --target URL       replace the original target
  --secret-header HEADER=ENV_VAR
                     set a request header from an environment variable;
                     may be repeated
  --unsafe-original-target
                     allow an omitted --target to use a saved remote target
  --json             emit structured JSON
  -h, --help         show this help

Examples:
  graybox replay bug.graybox
  graybox replay bug.graybox --id 42 --target http://localhost:8081
  graybox replay bug.graybox --secret-header Authorization=API_AUTH
`,
		)
	}

	fs := a.newFlagSet(
		"replay",
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
		&secretHeaderValues,
		"secret-header",
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
			"replay requires exactly one recording file",
		}
	}

	if flagWasSet(
		fs,
		"id",
	) &&
		idValue <= 0 {
		return ExitUsage, usageError{
			"--id must be a positive integer",
		}
	}

	requestHeaderOverrides, redactor, err := resolveSecretHeaders(
		secretHeaderValues,
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
		return classifyError(
				err,
			),
			redactor.redactError(
				fmt.Errorf(
					"cannot open recording %q: %w",
					recordingPath,
					err,
				),
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
		return classifyError(
				err,
			),
			redactor.redactError(
				err,
			)
	}

	var selectedID *int64

	if idValue > 0 {
		selectedID = &idValue
	}

	results, err := (replay.Runner{
		Source:                 store,
		RequestHeaderOverrides: requestHeaderOverrides,
	}).Run(
		ctx,
		target,
		selectedID,
	)

	if errors.Is(
		err,
		storage.ErrNotFound,
	) {
		return ExitInvalidRecording,
			redactor.redactError(
				fmt.Errorf(
					"recording %q has no exchange %d",
					recordingPath,
					idValue,
				),
			)
	}

	if err != nil {
		return classifyError(
				err,
			),
			redactor.redactError(
				err,
			)
	}

	failed := 0

	for _, result := range results {
		if result.Err != nil {
			failed++
		}
	}

	if jsonOutput {
		items := make(
			[]replayItemJSON,
			0,
			len(results),
		)

		for _, result := range results {
			item := replayItemJSON{
				ExchangeID: result.ExchangeID,
				Method:     result.Method,
				Path:       result.Path,
				TargetURL:  result.TargetURL,
				Status:     result.StatusCode,
				StatusText: result.Status,
				DurationMS: float64(
					result.Duration,
				) / float64(
					time.Millisecond,
				),
			}

			if result.Err != nil {
				item.Error = result.Err.Error()
			}

			items = append(
				items,
				item,
			)
		}

		output := map[string]any{
			"recording": recordingPath,
			"target":    target.String(),
			"succeeded": len(results) - failed,
			"failed":    failed,
			"results":   items,
		}

		if err := redactor.writeJSONOutput(
			a.Stdout,
			func(
				writer io.Writer,
			) error {
				return writeJSON(
					writer,
					output,
				)
			},
		); err != nil {
			return ExitInternal,
				redactor.redactError(
					fmt.Errorf(
						"write output: %w",
						err,
					),
				)
		}
	} else {
		if err := redactor.writeOutput(
			a.Stdout,
			func(
				writer io.Writer,
			) error {
				if len(results) == 1 {
					result := results[0]

					fmt.Fprintf(
						writer,
						"Replaying exchange %d\n\n%s %s\nTarget: %s\n",
						result.ExchangeID,
						result.Method,
						result.Path,
						target,
					)

					if result.Err != nil {
						fmt.Fprintf(
							writer,
							"Error: %v\nTime: %s\n",
							result.Err,
							formatDuration(
								result.Duration,
							),
						)
					} else {
						fmt.Fprintf(
							writer,
							"Response: %s\nTime: %s\n",
							result.Status,
							formatDuration(
								result.Duration,
							),
						)
					}

					return nil
				}

				fmt.Fprintf(
					writer,
					"Replaying %d exchanges against %s\n\n%d succeeded\n%d failed\n",
					len(results),
					target,
					len(results)-failed,
					failed,
				)

				return nil
			},
		); err != nil {
			return ExitInternal,
				redactor.redactError(
					fmt.Errorf(
						"write output: %w",
						err,
					),
				)
		}
	}

	if failed > 0 {
		if len(results) == 1 {
			return ExitNetwork, nil
		}

		return ExitReplayFailed, nil
	}

	return ExitSuccess, nil
}
