package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStrictParsing(t *testing.T) {
	good := []byte("redact:\n  json: [/client_secret, /items/0/token]\nreplay:\n  headers:\n    Authorization: 'Bearer ${API_TOKEN}'\n")
	cfg, err := Parse(good)
	if err != nil || cfg.Replay.Headers["Authorization"] != "Bearer ${API_TOKEN}" {
		t.Fatalf("%+v %v", cfg, err)
	}
	for _, input := range []string{
		"unknown: secret", "redact:\n  unknown: secret", "replay:\n  header: secret", "replay: []", "redact:\n  json: [false]", "redact:\n  json: null", "replay:\n  headers:\n    Authorization: 123", "replay:\n  headers:\n    Authorization: secret\n    Authorization: other", "redact: &rules {}\nreplay: *rules", "redact: {}\n---\nreplay: {}", "[broken", "", "null",
	} {
		if _, err := Parse([]byte(input)); err == nil || strings.Contains(err.Error(), "other") {
			t.Fatalf("accepted or leaked malformed config %q: %v", input, err)
		}
	}
}

func TestDiscovery(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if _, err := Load(""); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("missing.yaml"); err == nil {
		t.Fatal("explicit missing path accepted")
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("graybox.yaml", "redact:\n  query: [access_token]\n")
	cfg, err := Load("")
	if err != nil || len(cfg.Redact.Query) != 1 {
		t.Fatal(cfg, err)
	}
	path := filepath.Join(dir, "explicit.yaml")
	write(path, "redact:\n  form: [client_secret]\n")
	cfg, err = Load(path)
	if err != nil || len(cfg.Redact.Form) != 1 || len(cfg.Redact.Query) != 0 {
		t.Fatal(cfg, err)
	}
}
