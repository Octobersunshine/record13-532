package router

import (
	"context"
	"database/sql"
	"fmt"

	"sqlrouter/db"
	"sqlrouter/sqltype"
)

type Router struct {
	pool *db.Pool
}

type Tx struct {
	tx *sql.Tx
}

func New(pool *db.Pool) *Router {
	return &Router{pool: pool}
}

func (r *Router) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := r.pool.Master().BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{tx: tx}, nil
}

func (tx *Tx) Commit() error   { return tx.tx.Commit() }
func (tx *Tx) Rollback() error { return tx.tx.Rollback() }

func (tx *Tx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.tx.ExecContext(ctx, query, args...)
}

func (tx *Tx) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return tx.tx.QueryContext(ctx, query, args...)
}

func (tx *Tx) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return tx.tx.QueryRowContext(ctx, query, args...)
}

func (r *Router) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	conn, err := r.resolveDB(query)
	if err != nil {
		return nil, err
	}
	return conn.ExecContext(ctx, query, args...)
}

func (r *Router) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	conn, err := r.resolveDB(query)
	if err != nil {
		return nil, err
	}
	return conn.QueryContext(ctx, query, args...)
}

func (r *Router) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	conn, err := r.resolveDB(query)
	if err != nil {
		return nil
	}
	return conn.QueryRowContext(ctx, query, args...)
}

func (r *Router) resolveDB(query string) (*sql.DB, error) {
	switch sqltype.Classify(query) {
	case sqltype.Read:
		return r.pool.Slave(), nil
	case sqltype.Write:
		return r.pool.Master(), nil
	default:
		return nil, fmt.Errorf("cannot determine read/write for SQL: %q", query)
	}
}
