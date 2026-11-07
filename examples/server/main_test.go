package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOrderModes(t *testing.T) {
	for _, regressed := range []bool{false, true} {
		for _, authorized := range []bool{false, true} {
			req := httptest.NewRequest("GET", "/orders/42", nil)
			if authorized {
				req.Header.Set("Authorization", "Bearer demo-token")
			}
			response := httptest.NewRecorder()
			orderHandler(regressed).ServeHTTP(response, req)
			if !authorized {
				if response.Code != http.StatusUnauthorized {
					t.Fatalf("unauthenticated status %d", response.Code)
				}
				continue
			}
			var order struct {
				Total    int `json:"total_cents"`
				Discount int `json:"discount_cents"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &order); err != nil {
				t.Fatal(err)
			}
			expected := 4500
			if regressed {
				expected = 5000
			}
			if response.Code != 200 || order.Total != expected || order.Discount != 500 {
				t.Fatalf("unexpected order: %+v", order)
			}
		}
	}
}
