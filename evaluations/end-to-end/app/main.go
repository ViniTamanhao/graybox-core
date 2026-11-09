package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	_ "modernc.org/sqlite"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type app struct{ db *sql.DB }
type purchase struct {
	Product  int    `json:"product_id"`
	Quantity int    `json:"quantity"`
	Code     string `json:"code"`
}
type price struct {
	Subtotal int `json:"subtotal_cents"`
	Discount int `json:"discount_cents"`
	Total    int `json:"total_cents"`
}

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, s string) {
	respond(w, status, map[string]string{"error": s})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		problem(w, 400, "invalid_json")
		return false
	}
	return true
}
func (a *app) quote(p purchase) (price, string) {
	if p.Quantity < 1 || p.Quantity > 100 {
		return price{}, "invalid_quantity"
	}
	var unit int
	if a.db.QueryRow("SELECT cents FROM products WHERE id=?", p.Product).Scan(&unit) != nil {
		return price{}, "invalid_product"
	}
	subtotal := unit * p.Quantity
	discount := 0
	switch p.Code {
	case "":
	case "SAVE10":
		discount = (subtotal*10 + 50) / 100
	default:
		return price{}, "invalid_discount"
	}
	return price{subtotal, discount, subtotal - discount}, ""
}
func (a *app) serve(w http.ResponseWriter, r *http.Request) {
	owner := ""
	h := r.Header.Get("Authorization")
	if h != "" && h == os.Getenv("API_AUTH") {
		owner = "alice"
	} else if h != "" && h == os.Getenv("OTHER_AUTH") {
		owner = "bob"
	}
	if owner == "" {
		problem(w, 401, "unauthorized")
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 2 && parts[0] == "products" && r.Method == "GET" {
		id, e := strconv.Atoi(parts[1])
		var cents int
		if e != nil || a.db.QueryRow("SELECT cents FROM products WHERE id=?", id).Scan(&cents) != nil {
			problem(w, 404, "not_found")
			return
		}
		respond(w, 200, map[string]int{"id": id, "price_cents": cents})
		return
	}
	if (r.URL.Path == "/orders" || r.URL.Path == "/discounts/apply") && r.Method == "POST" {
		var p purchase
		if !decode(w, r, &p) {
			return
		}
		v, err := a.quote(p)
		if err != "" {
			problem(w, 400, err)
			return
		}
		if r.URL.Path == "/discounts/apply" {
			respond(w, 200, v)
			return
		}
		result, e := a.db.Exec("INSERT INTO orders(owner,state,subtotal,discount,total) VALUES(?,'pending',?,?,?)", owner, v.Subtotal, v.Discount, v.Total)
		if e != nil {
			problem(w, 500, "storage")
			return
		}
		id, e := result.LastInsertId()
		if e != nil {
			problem(w, 500, "storage")
			return
		}
		respond(w, 201, map[string]any{"id": id, "state": "pending", "subtotal_cents": v.Subtotal, "discount_cents": v.Discount, "total_cents": v.Total})
		return
	}
	if len(parts) >= 2 && parts[0] == "orders" {
		id, e := strconv.Atoi(parts[1])
		if e != nil {
			problem(w, 404, "not_found")
			return
		}
		tx, e := a.db.Begin()
		if e != nil {
			problem(w, 500, "storage")
			return
		}
		defer tx.Rollback()
		var who, state string
		var v price
		e = tx.QueryRow("SELECT owner,state,subtotal,discount,total FROM orders WHERE id=?", id).Scan(&who, &state, &v.Subtotal, &v.Discount, &v.Total)
		if e != nil {
			problem(w, 404, "not_found")
			return
		}
		if who != owner {
			problem(w, 403, "forbidden")
			return
		}
		if len(parts) == 3 && parts[2] == "cancel" && r.Method == "POST" {
			if state != "pending" {
				problem(w, 409, "invalid_state")
				return
			}
			if _, e = tx.Exec("UPDATE orders SET state='cancelled' WHERE id=?", id); e != nil {
				problem(w, 500, "storage")
				return
			}
			state = "cancelled"
		} else if len(parts) != 2 || r.Method != "GET" {
			problem(w, 404, "not_found")
			return
		}
		if e = tx.Commit(); e != nil {
			problem(w, 500, "storage")
			return
		}
		respond(w, 200, map[string]any{"id": id, "state": state, "subtotal_cents": v.Subtotal, "discount_cents": v.Discount, "total_cents": v.Total})
		return
	}
	problem(w, 404, "not_found")
}
func openDB(path string, reset bool) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	schema := `CREATE TABLE IF NOT EXISTS products(id INTEGER PRIMARY KEY, cents INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS orders(id INTEGER PRIMARY KEY AUTOINCREMENT, owner TEXT, state TEXT, subtotal INTEGER, discount INTEGER, total INTEGER);`
	if reset {
		schema = `DROP TABLE IF EXISTS orders; DROP TABLE IF EXISTS products;` + schema
	}
	_, err = db.Exec(schema)
	if err != nil {
		db.Close()
		return nil, err
	}
	_, err = db.Exec(`INSERT OR IGNORE INTO products VALUES(1,1999),(2,5),(3,10000);
 INSERT OR IGNORE INTO orders VALUES(40,'alice','pending',1999,200,1799),(41,'bob','pending',5,0,5),(42,'alice','shipped',10000,1000,9000);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
func main() {
	path := flag.String("db", "commerce.sqlite", "disposable database")
	reset := flag.Bool("reset", false, "reset seed and exit; only while server stopped")
	flag.Parse()
	if os.Getenv("API_AUTH") == "" || os.Getenv("OTHER_AUTH") == "" {
		log.Fatal("runtime credentials required")
	}
	db, err := openDB(*path, *reset)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if *reset {
		return
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("http://" + l.Addr().String())
	server := http.Server{Handler: http.HandlerFunc((&app{db}).serve), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.Serve(l))
}
