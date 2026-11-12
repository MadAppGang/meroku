// Command neon_e2e_pgq is the Postgres client of the real-Neon end-to-end test
// (app/neon_e2e_test.go, build tag neon_e2e). It lives in its own module so the
// app module does not depend on a Postgres driver.
//
// It connects with the DSN in PGQ_DSN (never an argument, so it stays out of
// process listings), runs each argument as one statement on that connection,
// in order, and prints one JSON object per line: first the connection result,
// then one per statement. The DSN is never printed.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// result is one line of output.
type result struct {
	SQL   string     `json:"sql,omitempty"`
	OK    bool       `json:"ok"`
	Code  string     `json:"code,omitempty"`
	Error string     `json:"error,omitempty"`
	Rows  [][]string `json:"rows,omitempty"`
	Tag   string     `json:"tag,omitempty"`
}

func main() {
	enc := json.NewEncoder(os.Stdout)
	dsn := os.Getenv("PGQ_DSN")
	if dsn == "" {
		_ = enc.Encode(result{Error: "PGQ_DSN is empty"})
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		code, msg := describe(err)
		_ = enc.Encode(result{Code: code, Error: msg})
		os.Exit(1)
	}
	defer conn.Close(context.Background())
	_ = enc.Encode(result{OK: true, Tag: "CONNECTED"})

	for _, q := range os.Args[1:] {
		_ = enc.Encode(run(ctx, conn, q))
	}
}

func run(ctx context.Context, conn *pgx.Conn, q string) result {
	r := result{SQL: q}
	rows, err := conn.Query(ctx, q, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		r.Code, r.Error = describe(err)
		return r
	}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			r.Code, r.Error = describe(err)
			rows.Close()
			return r
		}
		row := make([]string, len(vals))
		for i, v := range vals {
			row[i] = fmt.Sprint(v)
		}
		r.Rows = append(r.Rows, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		r.Code, r.Error = describe(err)
		return r
	}
	r.OK = true
	r.Tag = rows.CommandTag().String()
	return r
}

// describe is the SQLSTATE and message of a Postgres error, or the error text.
func describe(err error) (string, string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code, pgErr.Message
	}
	return "", err.Error()
}
