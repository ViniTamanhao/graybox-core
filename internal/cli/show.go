package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/ViniTamanhao/graybox-core/internal/storage"
)

func (a App) runShow(ctx context.Context, args []string) (int, error) {
	var jsonOutput, help bool
	usage := func() {
		fmt.Fprint(a.Stdout, `Usage: graybox show RECORDING ID [options]

Show one recorded exchange, including proxy errors and body capture state.
Truncated and incomplete bodies are labeled. JSON uses UTF-8 or base64.

Options:
  --json             emit structured JSON
  -h, --help         show this help

Example:
  graybox show bug.graybox 42 --json
`)
	}
	fs := a.newFlagSet("show", usage)
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
	if len(positional) != 2 {
		return ExitUsage, usageError{"show requires a recording file and exchange ID"}
	}
	id, err := parseID(positional[1])
	if err != nil {
		return ExitUsage, err
	}
	store, err := storage.OpenReadOnly(ctx, positional[0])
	if err != nil {
		return classifyError(err), fmt.Errorf("cannot open recording %q: %w", positional[0], err)
	}
	defer store.Close()
	ex, err := store.Get(ctx, id)
	if errors.Is(err, storage.ErrNotFound) {
		return ExitInvalidRecording, fmt.Errorf("recording %q has no exchange %d", positional[0], id)
	}
	if err != nil {
		return classifyError(err), err
	}
	if jsonOutput {
		if err := writeJSON(a.Stdout, toExchangeJSON(ex)); err != nil {
			return ExitInternal, err
		}
		return ExitSuccess, nil
	}
	status := fmt.Sprintf("%d", ex.Response.StatusCode)
	if text := http.StatusText(ex.Response.StatusCode); text != "" {
		status += " " + text
	}
	fmt.Fprintf(a.Stdout, "%s %s\n%s\n%s\n\n", ex.Request.Method, requestPath(ex.Request.URL), status, formatDuration(ex.Duration))
	if ex.ProxyError != "" {
		fmt.Fprintf(a.Stdout, "Proxy error: %s\n\n", ex.ProxyError)
	}
	fmt.Fprintln(a.Stdout, "Request\n\nHeaders:")
	writeHeaders(a.Stdout, ex.Request.Headers)
	fmt.Fprintln(a.Stdout, "\nBody:")
	writeBody(
		a.Stdout,
		ex.Request.Body,
		ex.Request.ObservedSize,
		ex.Request.Truncated,
		ex.Request.Complete,
		ex.Request.Headers.Get("Content-Type"),
	)
	fmt.Fprintln(a.Stdout, "\nResponse\n\nHeaders:")
	writeHeaders(a.Stdout, ex.Response.Headers)
	fmt.Fprintln(a.Stdout, "\nBody:")
	writeBody(
		a.Stdout,
		ex.Response.Body,
		ex.Response.ObservedSize,
		ex.Response.Truncated,
		ex.Response.Complete,
		ex.Response.Headers.Get("Content-Type"),
	)
	return ExitSuccess, nil
}
