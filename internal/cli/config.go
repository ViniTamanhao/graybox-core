package cli

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/ViniTamanhao/graybox-core/internal/config"
	"github.com/ViniTamanhao/graybox-core/internal/replay"
	"github.com/ViniTamanhao/graybox-core/internal/sanitize"
)

func loadConfig(path string) (config.Config, sanitize.Rules, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return cfg, sanitize.Rules{}, usageError{err.Error()}
	}
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
					return nil, replay.Replacements{}, newSecretRedactor(secrets), usageError{"environment variable contains an invalid HTTP header value"}
				}
				headers.Set(field, resolved)
			} else {
				group.destination[field] = resolved
			}
		}
	}
	return headers, replacement, newSecretRedactor(secrets), nil
}
