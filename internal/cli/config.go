package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/ViniTamanhao/graybox-core/internal/config"
	"github.com/ViniTamanhao/graybox-core/internal/recording"
	"github.com/ViniTamanhao/graybox-core/internal/replay"
	"github.com/ViniTamanhao/graybox-core/internal/sanitize"
	"github.com/ViniTamanhao/graybox-core/internal/storage"
)

func loadConfig(path string) (config.Config, sanitize.Rules, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return cfg, sanitize.Rules{}, usageError{err.Error()}
	}
	return validateConfig(cfg)
}

func validateConfig(cfg config.Config) (config.Config, sanitize.Rules, error) {
	rules := sanitize.Rules{Headers: cfg.Redact.Headers, JSON: cfg.Redact.JSON, Query: cfg.Redact.Query, Form: cfg.Redact.Form}
	invalid := func(message string) (config.Config, sanitize.Rules, error) {
		return config.Config{}, sanitize.Rules{}, usageError{"invalid graybox configuration: " + message}
	}
	for _, name := range rules.Headers {
		if !validHTTPHeaderName(name) {
			return invalid("invalid redaction header name")
		}
	}
	for _, paths := range [][]string{rules.JSON, keys(cfg.Replay.JSON)} {
		for _, path := range paths {
			if _, err := sanitize.Pointer(path); err != nil {
				return invalid(err.Error())
			}
		}
	}
	paths := keys(cfg.Replay.JSON)
	for i, path := range paths {
		for _, other := range paths[i+1:] {
			if strings.HasPrefix(other, path+"/") {
				return invalid("overlapping JSON replacement paths")
			}
		}
	}
	for _, fields := range [][]string{rules.Query, rules.Form, keys(cfg.Replay.Query), keys(cfg.Replay.Form)} {
		for _, field := range fields {
			if field == "" || strings.ContainsAny(field, "\r\n\x00") {
				return invalid("invalid query/form field name")
			}
		}
	}
	seen := map[string]bool{}
	for _, name := range keys(cfg.Replay.Headers) {
		normalized := strings.ToLower(name)
		if !validHTTPHeaderName(name) || unsupportedSecretHeader(name) || seen[normalized] {
			return invalid("invalid, unsupported, or duplicate replay header")
		}
		seen[normalized] = true
	}
	for _, group := range []map[string]string{cfg.Replay.Headers, cfg.Replay.JSON, cfg.Replay.Query, cfg.Replay.Form} {
		for _, expression := range group {
			if _, _, _, err := environmentReference(expression); err != nil {
				return invalid(err.Error())
			}
		}
	}
	return cfg, rules, nil
}

// A single ${ENV_VAR} reference, with optional literal prefix/suffix. This is
// deliberately not a template engine and never accepts literal-only secrets.
func environmentReference(expression string) (string, string, string, error) {
	prefix, rest, ok := strings.Cut(expression, "${")
	name, suffix, closed := strings.Cut(rest, "}")
	if !ok || !closed || !validEnvironmentVariableName(name) || strings.ContainsAny(prefix+suffix, "${}") {
		return "", "", "", fmt.Errorf("replay values require exactly one ${ENV_VAR} reference")
	}
	return prefix, name, suffix, nil
}

func keys(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

// resolveRequestConfig extends secret headers without putting resolved values
// in the parsed config. CLI header mappings win before environment resolution.
func resolveRequestConfig(cfg config.Config, flags []string) (http.Header, replay.Replacements, secretRedactor, error) {
	mappings, err := parseSecretHeaderMappings(flags)
	if err != nil {
		return nil, replay.Replacements{}, secretRedactor{}, err
	}
	expressions := make(map[string]string, len(cfg.Replay.Headers))
	for name, value := range cfg.Replay.Headers {
		expressions[http.CanonicalHeaderKey(name)] = value
	}
	for _, mapping := range mappings {
		expressions[mapping.Header] = "${" + mapping.EnvVar + "}"
	}
	headers := make(http.Header)
	replacement := replay.Replacements{JSON: make(map[string]string), Query: make(map[string]string), Form: make(map[string]string)}
	var secrets []string
	groups := []struct {
		source, destination map[string]string
		header              bool
	}{
		{expressions, nil, true}, {cfg.Replay.JSON, replacement.JSON, false}, {cfg.Replay.Query, replacement.Query, false}, {cfg.Replay.Form, replacement.Form, false},
	}
	for _, group := range groups {
		for _, field := range keys(group.source) {
			prefix, name, suffix, err := environmentReference(group.source[field])
			if err != nil {
				return nil, replay.Replacements{}, newSecretRedactor(secrets), usageError{err.Error()}
			}
			value, exists := os.LookupEnv(name)
			if !exists || value == "" {
				reason := "is not set"
				if exists {
					reason = "is empty"
				}
				return nil, replay.Replacements{}, newSecretRedactor(secrets), usageError{fmt.Sprintf("environment variable %q for runtime replacement %s", name, reason)}
			}
			secrets = append(secrets, value)
			resolved := prefix + value + suffix
			secrets = append(secrets, resolved)
			if group.header {
				if !validHTTPHeaderValue(resolved) {
					return nil, replay.Replacements{}, newSecretRedactor(secrets), usageError{fmt.Sprintf("environment variable %q contains an invalid HTTP header value for %s", name, field)}
				}
				headers.Set(field, resolved)
			} else {
				group.destination[field] = resolved
			}
		}
	}
	return headers, replacement, newSecretRedactor(secrets), nil
}

// resolveApplicableReplacements checks selected requests without sending traffic.
// Only referenced fields require credentials; unsafe requests still fail later
// in the shared replay engine. Keep unused rule keys so ambiguity cannot bypass
// execution-time validation. This preflight retains one exchange at a time.
func resolveApplicableReplacements(ctx context.Context, source replay.Source, id *int64, cfg config.Config, redactor secretRedactor) (replay.Replacements, secretRedactor, error) {
	raw := replay.Replacements{JSON: cfg.Replay.JSON, Form: cfg.Replay.Form, Query: cfg.Replay.Query}
	if len(raw.JSON)+len(raw.Form)+len(raw.Query) == 0 {
		return raw, redactor, nil
	}
	needed := config.Config{Replay: config.Replay{JSON: make(map[string]string), Form: make(map[string]string), Query: make(map[string]string)}}
	inspect := func(exchange recording.Exchange) {
		if exchange.Request.Truncated || !exchange.Request.Complete {
			return
		}
		parsed, err := url.Parse(exchange.Request.URL)
		if err != nil {
			return
		}
		matched, err := raw.Applicable(&http.Request{URL: parsed, Header: exchange.Request.Headers}, exchange.Request.Body)
		if err != nil {
			return
		}
		for key, value := range matched.JSON {
			needed.Replay.JSON[key] = value
		}
		for key, value := range matched.Form {
			needed.Replay.Form[key] = value
		}
		for key, value := range matched.Query {
			needed.Replay.Query[key] = value
		}
	}
	if id != nil {
		exchange, err := source.Get(ctx, *id)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return replay.Replacements{}, redactor, err
		}
		if err == nil {
			inspect(exchange)
		}
	} else {
		summaries, err := source.List(ctx, recording.Filter{})
		if err != nil {
			return replay.Replacements{}, redactor, err
		}
		for _, summary := range summaries {
			exchange, err := source.Get(ctx, summary.ID)
			if err != nil {
				return replay.Replacements{}, redactor, err
			}
			inspect(exchange)
		}
	}
	_, resolved, fieldsRedactor, err := resolveRequestConfig(needed, nil)
	combined := newSecretRedactor(append(append([]string(nil), redactor.values...), fieldsRedactor.values...))
	if err != nil {
		return replay.Replacements{}, combined, err
	}
	retain := func(rules, resolved map[string]string) map[string]string {
		result := make(map[string]string, len(rules))
		for key := range rules {
			result[key] = resolved[key]
		}
		return result
	}
	return replay.Replacements{JSON: retain(raw.JSON, resolved.JSON), Form: retain(raw.Form, resolved.Form), Query: retain(raw.Query, resolved.Query)}, combined, nil
}

// recordingRedaction combines current and optional capture policy. Older
// recordings remain supported; marker inference in replay provides a fallback.
func recordingRedaction(ctx context.Context, metadata metadataReader, current sanitize.Rules) (sanitize.Rules, error) {
	raw, err := metadata.Metadata(ctx, "redaction_policy")
	if errors.Is(err, storage.ErrNotFound) {
		return current, nil
	}
	if err != nil {
		return sanitize.Rules{}, err
	}
	// Policy metadata is a JSON object (a YAML subset). Reuse strict config
	// parsing/validation rather than maintain a second policy schema.
	cfg, err := config.Parse([]byte(`{"redact":` + raw + `}`))
	if err != nil {
		return sanitize.Rules{}, fmt.Errorf("%w: invalid recorded redaction policy", storage.ErrInvalidRecording)
	}
	_, capture, err := validateConfig(cfg)
	if err != nil {
		return sanitize.Rules{}, fmt.Errorf("%w: invalid recorded redaction policy", storage.ErrInvalidRecording)
	}
	return current.Merge(capture), nil
}
