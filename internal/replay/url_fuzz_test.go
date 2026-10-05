package replay

import (
	"net/url"
	"testing"
)

func FuzzBuildURL(f *testing.F) {
	for _, seed := range []string{"/a?x=1", "https://evil.example/path?q=2", "//evil.example/a", "/%2f%2fevil.example?x=%ff", "?", "%", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, recorded string) {
		target := &url.URL{Scheme: "https", Host: "target.example:443", Path: "/prefix", RawQuery: "discard=1"}
		before := *target
		got, err := BuildURL(target, recorded)
		if *target != before {
			t.Fatal("target mutated")
		}
		parsed, parseErr := url.Parse(recorded)
		if parseErr != nil {
			if err == nil {
				t.Fatal("invalid recorded URL accepted")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if got.Scheme != target.Scheme || got.Host != target.Host || got.RawQuery != parsed.RawQuery || got.ForceQuery != parsed.ForceQuery || got.Fragment != "" {
			t.Fatalf("URL boundaries changed: %s", got)
		}
		roundTrip, err := url.Parse(got.String())
		if err != nil || roundTrip.Scheme != target.Scheme || roundTrip.Host != target.Host {
			t.Fatalf("serialized URL boundaries changed: %s", got)
		}
	})
}
