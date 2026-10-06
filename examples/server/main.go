// Command server is a tiny authenticated order API for trying Graybox.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /orders/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer demo-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// A $50 order with a $5 discount. Amounts are integer cents.
		subtotal := 5000
		discount := 500
		total := subtotal - discount

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			ID            int    `json:"id"`
			Currency      string `json:"currency"`
			SubtotalCents int    `json:"subtotal_cents"`
			DiscountCents int    `json:"discount_cents"`
			TotalCents    int    `json:"total_cents"`
		}{42, "USD", subtotal, discount, total})
	})

	address := "127.0.0.1:8080"
	if configured := os.Getenv("GRAYBOX_EXAMPLE_LISTEN"); configured != "" {
		address = configured
	}
	log.Printf("Example API listening on %s", address)
	server := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
