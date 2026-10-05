package diff

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalizeRejectsInvalidJSONNumbers(t *testing.T) {
	for _, raw := range []string{"01", "-01", "1e1_0", "1e_1", " 1", "1 ", "+1", ".1", "1.", "NaN"} {
		if _, ok := canonicalizeJSONNumber(json.Number(raw)); ok {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func FuzzJSONNumbers(f *testing.F) {
	for _, seed := range []string{"1", "-0", "1.00", "10e-1", "1e999999999999999", "01", "1e1_0", "", "null"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		number := json.Number(raw)
		_, ok := canonicalizeJSONNumber(number)
		valid := len(raw) > 0 && (raw[0] == '-' || (raw[0] >= '0' && raw[0] <= '9')) && strings.TrimSpace(raw) == raw && json.Valid([]byte(raw))
		if ok != valid {
			t.Fatalf("validity mismatch: %q", raw)
		}
		if !jsonNumbersEqual(number, number) {
			t.Fatal("non-reflexive equality")
		}
		if !ok {
			if jsonNumbersEqual(number, json.Number(raw+"!")) {
				t.Fatal("invalid values did not fall back to token equality")
			}
			return
		}
		mantissa, exponent := raw, ""
		if index := strings.IndexAny(raw, "eE"); index >= 0 {
			mantissa, exponent = raw[:index], raw[index:]
		}
		if strings.Contains(mantissa, ".") {
			mantissa += "0"
		} else {
			mantissa += ".0"
		}
		equivalent := json.Number(mantissa + exponent)
		if !jsonNumbersEqual(number, equivalent) || !jsonNumbersEqual(equivalent, number) {
			t.Fatalf("equivalent numbers differ: %q %q", number, equivalent)
		}
	})
}
