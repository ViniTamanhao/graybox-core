package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/opemori/graybox-core/internal/replay"
	"github.com/opemori/graybox-core/internal/storage"
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

func (a App) runReplay(ctx context.Context, args []string) (int, error) {
	var targetValue string
	var idValue int64
	var jsonOutput, help bool
	usage := func() {
		fmt.Fprint(a.Stdout, `Usage: graybox replay RECORDING [options]

Replay recorded requests sequentially. If --target is omitted, Graybox uses
the upstream target saved when the recording was created.

Options:
  --id ID            replay only one exchange
  --target URL       replace the original target
  --json             emit structured JSON
  -h, --help         show this help

Examples:
  graybox replay bug.graybox
  graybox replay bug.graybox --id 42 --target http://localhost:8081
`)
	}
	fs := a.newFlagSet("replay", usage)
	fs.Int64Var(&idValue, "id", 0, "")
	fs.StringVar(&targetValue, "target", "", "")
	fs.BoolVar(&jsonOutput, "json", false, "")
	fs.BoolVar(&help, "help", false, "")
	fs.BoolVar(&help, "h", false, "")
	positional, err := parseInterspersed(fs, args, map[string]bool{"json": true, "help": true, "h": true})
	if err != nil {
		return ExitUsage, usageError{err.Error()}
	}
	if help {
		usage()
		return ExitSuccess, nil
	}
	if len(positional) != 1 {
		return ExitUsage, usageError{"replay requires exactly one recording file"}
	}
	if idValue < 0 {
		return ExitUsage, usageError{"--id must be a positive integer"}
	}
	store, err := storage.Open(ctx, positional[0])
	if err != nil {
		return classifyError(err), fmt.Errorf("cannot open recording %q: %w", positional[0], err)
	}
	defer store.Close()
	if targetValue == "" {
		targetValue, err = store.Metadata(ctx, "target_url")
		if err != nil {
			return ExitUsage, usageError{"recording has no original target; provide --target"}
		}
	}
	target, err := parseTarget(targetValue)
	if err != nil {
		return ExitUsage, err
	}
	var selectedID *int64
	if idValue > 0 {
		selectedID = &idValue
	}
	results, err := (replay.Runner{Source: store}).Run(ctx, target, selectedID)
	if errors.Is(err, storage.ErrNotFound) {
		return ExitInvalidRecording, fmt.Errorf("recording %q has no exchange %d", positional[0], idValue)
	}
	if err != nil {
		return ExitInternal, err
	}
	failed := 0
	if jsonOutput {
		items := make([]replayItemJSON, 0, len(results))
		for _, result := range results {
			item := replayItemJSON{ExchangeID: result.ExchangeID, Method: result.Method, Path: result.Path,
				TargetURL: result.TargetURL, Status: result.StatusCode, StatusText: result.Status,
				DurationMS: float64(result.Duration) / float64(time.Millisecond)}
			if result.Err != nil {
				item.Error = result.Err.Error()
				failed++
			}
			items = append(items, item)
		}
		output := map[string]any{"recording": positional[0], "target": target.String(), "succeeded": len(results) - failed, "failed": failed, "results": items}
		if err := writeJSON(a.Stdout, output); err != nil {
			return ExitInternal, err
		}
	} else if len(results) == 1 {
		result := results[0]
		fmt.Fprintf(a.Stdout, "Replaying exchange %d\n\n%s %s\nTarget: %s\n", result.ExchangeID, result.Method, result.Path, target)
		if result.Err != nil {
			failed++
			fmt.Fprintf(a.Stdout, "Error: %v\nTime: %s\n", result.Err, formatDuration(result.Duration))
		} else {
			fmt.Fprintf(a.Stdout, "Response: %s\nTime: %s\n", result.Status, formatDuration(result.Duration))
		}
	} else {
		for _, result := range results {
			if result.Err != nil {
				failed++
			}
		}
		fmt.Fprintf(a.Stdout, "Replaying %d exchanges against %s\n\n%d succeeded\n%d failed\n", len(results), target, len(results)-failed, failed)
	}
	if failed > 0 {
		if len(results) == 1 {
			return ExitNetwork, nil
		}
		return ExitReplayFailed, nil
	}
	return ExitSuccess, nil
}
