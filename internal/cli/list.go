package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/opemori/graybox-core/internal/recording"
	"github.com/opemori/graybox-core/internal/storage"
)

type listItemJSON struct {
	ID         int64   `json:"id"`
	Protocol   string  `json:"protocol"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Status     int     `json:"status"`
	StartedAt  string  `json:"started_at"`
	DurationMS float64 `json:"duration_ms"`
}

func (a App) runList(ctx context.Context, args []string) (int, error) {
	var method, path string
	var status int
	var jsonOutput, help bool
	usage := func() {
		fmt.Fprint(a.Stdout, `Usage: graybox ls RECORDING [options]

List exchanges by request start time, then stable ID. Method matching is case-insensitive. Path
matching is exact and ignores the query unless --path itself contains a query.

Options:
  --status CODE      match an exact HTTP status
  --method METHOD    match an exact HTTP method
  --path PATH        match an exact request path or request URI
  --json             emit structured JSON
  -h, --help         show this help

Example:
  graybox ls bug.graybox --method POST --json
`)
	}
	fs := a.newFlagSet("ls", usage)
	fs.IntVar(&status, "status", 0, "")
	fs.StringVar(&method, "method", "", "")
	fs.StringVar(&path, "path", "", "")
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
		return ExitUsage, usageError{"ls requires exactly one recording file"}
	}
	if status != 0 && (status < 100 || status > 999) {
		return ExitUsage, usageError{"--status must be between 100 and 999"}
	}
	store, err := storage.OpenReadOnly(ctx, positional[0])
	if err != nil {
		return classifyError(err), fmt.Errorf("cannot open recording %q: %w", positional[0], err)
	}
	defer store.Close()
	items, err := store.List(ctx, recording.Filter{Status: status, Method: method, Path: path})
	if err != nil {
		return ExitInternal, err
	}
	if jsonOutput {
		output := make([]listItemJSON, 0, len(items))
		for _, item := range items {
			output = append(output, listItemJSON{ID: item.ID, Protocol: item.Protocol, Method: item.Method,
				Path: requestPath(item.URL), Status: item.StatusCode, StartedAt: item.StartedAt.Format(time.RFC3339Nano),
				DurationMS: float64(item.Duration) / float64(time.Millisecond)})
		}
		if err := writeJSON(a.Stdout, map[string]any{"recording": positional[0], "exchanges": output}); err != nil {
			return ExitInternal, err
		}
		return ExitSuccess, nil
	}
	tw := tabwriter.NewWriter(a.Stdout, 0, 4, 3, ' ', 0)
	fmt.Fprintln(tw, "ID\tMETHOD\tPATH\tSTATUS\tTIME")
	for _, item := range items {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%s\n", item.ID, strings.ToUpper(item.Method), displayPath(item.URL), item.StatusCode, formatDuration(item.Duration))
	}
	if err := tw.Flush(); err != nil {
		return ExitInternal, fmt.Errorf("write output: %w", err)
	}
	return ExitSuccess, nil
}

func displayPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.RequestURI()
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	}
	return fmt.Sprintf("%dms", d.Round(time.Millisecond)/time.Millisecond)
}
