package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ViniTamanhao/graybox-core/internal/capture"
	"github.com/ViniTamanhao/graybox-core/internal/storage"
)

const defaultListenAddress = "127.0.0.1:9000"

type recordingStore interface {
	capture.Recorder
	SetMetadata(context.Context, string, string) error
	Close() error
}

type createRecordingFunc func(context.Context, string, string) (recordingStore, error)
type listenFunc func(string, string) (net.Listener, error)

func (a App) runRecord(ctx context.Context, args []string) (int, error) {
	var listen, targetValue, output string
	var bodyLimit int64
	var jsonOutput, help bool
	usage := func() {
		fmt.Fprint(a.Stdout, `Usage: graybox record --target URL [options]

Proxy HTTP traffic to an upstream target and record it in a .graybox file.
Traffic continues to stream when bounded body captures are truncated. If any
observed exchange cannot be persisted, shutdown reports the loss and fails.

Options:
  --listen ADDRESS   listen address (default 127.0.0.1:9000)
  --target URL       upstream HTTP or HTTPS URL (required)
  --output FILE      output recording (default session.graybox)
  --body-limit BYTES maximum bytes retained per request or response body (default 10485760)
  --json             emit machine-readable startup information
  -h, --help         show this help

Example:
  graybox record --listen 127.0.0.1:9000 --target http://localhost:8080 --output bug.graybox
`)
	}
	fs := a.newFlagSet("record", usage)
	fs.StringVar(&listen, "listen", defaultListenAddress, "")
	fs.StringVar(&targetValue, "target", "", "")
	fs.StringVar(&output, "output", "session.graybox", "")
	fs.Int64Var(&bodyLimit, "body-limit", capture.DefaultBodyCaptureLimit, "")
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
	if len(positional) != 0 {
		return ExitUsage, usageError{"record does not accept positional arguments"}
	}
	if bodyLimit <= 0 {
		return ExitUsage, usageError{"--body-limit must be a positive number of bytes"}
	}
	target, err := parseTarget(targetValue)
	if err != nil {
		return ExitUsage, err
	}
	listenNetwork := a.listen
	if listenNetwork == nil {
		listenNetwork = net.Listen
	}
	listener, err := listenNetwork("tcp", listen)
	if err != nil {
		return ExitInternal, fmt.Errorf("listen on %q: %w", listen, err)
	}
	defer listener.Close()
	createRecording := a.createRecording
	if createRecording == nil {
		createRecording = func(ctx context.Context, path, version string) (recordingStore, error) {
			return storage.Create(ctx, path, version)
		}
	}
	store, err := createRecording(ctx, output, a.buildVersion())
	if err != nil {
		return ExitInternal, fmt.Errorf("cannot create recording %q: %w", output, err)
	}
	defer store.Close()
	if err := store.SetMetadata(ctx, "target_url", target.String()); err != nil {
		return ExitInternal, err
	}
	if err := store.SetMetadata(ctx, "body_capture_limit", fmt.Sprint(bodyLimit)); err != nil {
		return ExitInternal, err
	}

	if jsonOutput {
		err = writeJSON(a.Stdout, map[string]any{"listen": displayListen(listen), "target": target.String(), "recording": output, "body_capture_limit": bodyLimit})
	} else {
		_, err = fmt.Fprintf(a.Stdout, "Graybox recording\n\nListening:   %s\nForwarding:  %s\nRecording:   %s\nBody limit:  %d bytes\n", displayListen(listen), target, output, bodyLimit)
	}
	if err != nil {
		return ExitInternal, fmt.Errorf("write output: %w", err)
	}

	var errorMu sync.Mutex
	report := func(err error) {
		errorMu.Lock()
		defer errorMu.Unlock()
		fmt.Fprintf(a.Stderr, "graybox: %v\n", err)
	}
	handler := capture.NewProxyWithBodyLimit(target, store, bodyLimit, capture.ErrorHandlers{
		Transport: report, Persistence: report,
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()
	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return a.finishRecording(store, handler)
		}
		return ExitInternal, fmt.Errorf("serve proxy: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return ExitInternal, fmt.Errorf("shut down proxy: %w", err)
		}
		if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return ExitInternal, fmt.Errorf("serve proxy: %w", err)
		}
		return a.finishRecording(store, handler)
	}
}

func (a App) finishRecording(store recordingStore, handler *capture.Proxy) (int, error) {
	closeErr := store.Close()
	failures := handler.PersistenceFailures()
	if failures > 0 {
		noun := "exchanges"
		if failures == 1 {
			noun = "exchange"
		}
		fmt.Fprintf(a.Stderr, "Recording completed with errors:\n%d %s could not be persisted\n", failures, noun)
		if closeErr != nil {
			return ExitInternal, closeErr
		}
		return ExitInternal, nil
	}
	if closeErr != nil {
		return ExitInternal, closeErr
	}
	return ExitSuccess, nil
}

func parseTarget(value string) (*url.URL, error) {
	if value == "" {
		return nil, usageError{"--target is required"}
	}
	if !strings.Contains(value, "://") {
		return nil, usageError{fmt.Sprintf("target %q is missing a URL scheme; try http://%s", value, value)}
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, usageError{fmt.Sprintf("invalid target %q: %v", value, err)}
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, usageError{fmt.Sprintf("target %q must be an absolute HTTP or HTTPS URL", value)}
	}
	if parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" {
		return nil, usageError{fmt.Sprintf("target %q must not contain credentials, a query, or a fragment", value)}
	}
	return parsed, nil
}

func displayListen(listen string) string {
	if strings.HasPrefix(listen, ":") {
		return "http://0.0.0.0" + listen
	}
	return "http://" + listen
}

func writeJSON(w interface{ Write([]byte) (int, error) }, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
