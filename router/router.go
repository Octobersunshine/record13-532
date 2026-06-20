package router

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"sqlrouter/db"
	"sqlrouter/sqltype"
	"sqlrouter/stats"
)

type Router struct {
	pool      *db.Pool
	collector *stats.Collector
}

type Tx struct {
	tx        *sql.Tx
	collector *stats.Collector
}

func New(pool *db.Pool, collector *stats.Collector) *Router {
	return &Router{pool: pool, collector: collector}
}

func (r *Router) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := r.pool.Master().BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{tx: tx, collector: r.collector}, nil
}

func (tx *Tx) Commit() error   { return tx.tx.Commit() }
func (tx *Tx) Rollback() error { return tx.tx.Rollback() }

func (tx *Tx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	res, err := tx.tx.ExecContext(ctx, query, args...)
	tx.collector.Record(stats.Master, query, time.Since(start))
	return res, err
}

func (tx *Tx) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := tx.tx.QueryContext(ctx, query, args...)
	tx.collector.Record(stats.Master, query, time.Since(start))
	return rows, err
}

func (tx *Tx) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	start := time.Now()
	row := tx.tx.QueryRowContext(ctx, query, args...)
	tx.collector.Record(stats.Master, query, time.Since(start))
	return row
}

func (r *Router) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	conn, target, err := r.resolveDB(query)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	res, err := conn.ExecContext(ctx, query, args...)
	r.collector.Record(target, query, time.Since(start))
	return res, err
}

func (r *Router) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	conn, target, err := r.resolveDB(query)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	rows, err := conn.QueryContext(ctx, query, args...)
	r.collector.Record(target, query, time.Since(start))
	return rows, err
}

func (r *Router) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	conn, target, err := r.resolveDB(query)
	if err != nil {
		return nil
	}
	start := time.Now()
	row := conn.QueryRowContext(ctx, query, args...)
	r.collector.Record(target, query, time.Since(start))
	return row
}

func (r *Router) resolveDB(query string) (*sql.DB, stats.Target, error) {
	switch sqltype.Classify(query) {
	case sqltype.Read:
		return r.pool.Slave(), stats.Slave, nil
	case sqltype.Write:
		return r.pool.Master(), stats.Master, nil
	default:
		return nil, stats.Target(99), fmt.Errorf("cannot determine read/write for SQL: %q", query)
	}
}
