package main

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCommerce(t *testing.T) {
	t.Setenv("API_AUTH", "Bearer synthetic-a")
	t.Setenv("OTHER_AUTH", "Bearer synthetic-b")
	db, e := openDB(t.TempDir()+"/db", true)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	a := &app{db}
	for _, c := range []struct {
		method, path, body string
		status             int
	}{{"GET", "/products/1", "", 200}, {"POST", "/discounts/apply", `{"product_id":1,"quantity":1,"code":"SAVE10"}`, 200}, {"POST", "/orders", `{"product_id":1,"quantity":2}`, 201}, {"GET", "/orders/41", "", 403}} {
		r := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		r.Header.Set("Authorization", os.Getenv("API_AUTH"))
		w := httptest.NewRecorder()
		a.serve(w, r)
		if w.Code != c.status {
			t.Errorf("%s: %d %s", c.path, w.Code, w.Body.String())
		}
	}
}
