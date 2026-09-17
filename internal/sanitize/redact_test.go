package sanitize

import (
	"net/http"
	"reflect"
	"testing"
)

func TestHeadersRedactsSensitiveValuesWithoutMutatingInput(t *testing.T) {
	original := http.Header{
		"Authorization":       {"Bearer secret"},
		"Proxy-Authorization": {"Basic abc"},
		"Cookie":              {"session=secret", "other=value"},
		"Set-Cookie":          {"token=secret"},
		"X-Request-Id":        {"visible", "second"},
	}
	got := Headers(original)
	for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie"} {
		for _, value := range got[name] {
			if value != RedactedValue {
				t.Fatalf("%s was not redacted: %q", name, value)
			}
		}
	}
	if !reflect.DeepEqual(got["X-Request-Id"], []string{"visible", "second"}) {
		t.Fatalf("ordinary header changed: %#v", got["X-Request-Id"])
	}
	got.Set("X-Request-ID", "changed")
	if original.Get("X-Request-ID") != "visible" {
		t.Fatal("redaction mutated its input")
	}
}
