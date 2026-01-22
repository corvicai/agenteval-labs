package scripts

import (
	"database/sql"
	"log"
	"math/rand"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func must(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %v", msg, err)
	}
}

func db_healthcheck() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	db, err := sql.Open("postgres", dsn)
	must(err, "open db")
	defer db.Close()

	payload := rand.Int63()

	start := time.Now()
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS healthcheck(id SERIAL PRIMARY KEY, payload BIGINT, created_at TIMESTAMPTZ DEFAULT now())`)
	must(err, "create table")
	log.Printf("create latency: %v", time.Since(start))

	start = time.Now()
	_, err = db.Exec(`INSERT INTO healthcheck(payload) VALUES($1)`, payload)
	must(err, "insert")
	log.Printf("insert latency: %v", time.Since(start))

	start = time.Now()
	var out int64
	err = db.QueryRow(`SELECT payload FROM healthcheck ORDER BY id DESC LIMIT 1`).Scan(&out)
	must(err, "select")
	log.Printf("select latency: %v (payload %d)", time.Since(start), out)
}
