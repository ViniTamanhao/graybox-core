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

	"github.com/opemori/graybox-core/internal/capture"
	"github.com/opemori/graybox-core/internal/storage"
)

func (a App) runRecord(ctx context.Context, args []string) (int, error) {
	var listen, targetValue, output string
	var jsonOutput, help bool
	usage := func() {
		fmt.Fprint(a.Stdout, `Usage: graybox record --target URL [options]

Proxy HTTP traffic to an upstream target and record it in a .graybox file.

Options:
  --listen ADDRESS   listen address (default :9000)
  --target URL       upstream HTTP or HTTPS URL (required)
  --output FILE      output recording (default session.graybox)
  --json             emit machine-readable startup information
  -h, --help         show this help

Example:
  graybox record --listen :9000 --target http://localhost:8080 --output bug.graybox
`)
	}
	fs := a.newFlagSet("record", usage)
	fs.StringVar(&listen, "listen", ":9000", "")
	fs.StringVar(&targetValue, "target", "", "")
	fs.StringVar(&output, "output", "session.graybox", "")
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
	target, err := parseTarget(targetValue)
	if err != nil {
		return ExitUsage, err
	}
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return ExitInternal, fmt.Errorf("listen on %q: %w", listen, err)
	}
	defer listener.Close()
	store, err := storage.Create(ctx, output, a.Version)
	if err != nil {
		return ExitInternal, fmt.Errorf("cannot create recording %q: %w", output, err)
	}
	defer store.Close()
	if err := store.SetMetadata(ctx, "target_url", target.String()); err != nil {
		return ExitInternal, err
	}

	if jsonOutput {
		err = writeJSON(a.Stdout, map[string]any{"listen": displayListen(listen), "target": target.String(), "recording": output})
	} else {
		_, err = fmt.Fprintf(a.Stdout, "Graybox recording\n\nListening:   %s\nForwarding:  %s\nRecording:   %s\n", displayListen(listen), target, output)
	}
	if err != nil {
		return ExitInternal, fmt.Errorf("write output: %w", err)
	}

	var errorMu sync.Mutex
	handler := capture.NewProxy(target, store, func(err error) {
		errorMu.Lock()
		defer errorMu.Unlock()
		fmt.Fprintf(a.Stderr, "graybox: %v\n", err)
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()
	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return ExitSuccess, nil
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
		if err := store.Close(); err != nil {
			return ExitInternal, err
		}
		return ExitSuccess, nil
	}
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
		return "http://localhost" + listen
	}
	return "http://" + listen
}

func writeJSON(w interface{ Write([]byte) (int, error) }, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
