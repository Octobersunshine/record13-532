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

func New(pool *db.Pool) *Router {
	return &Router{pool: pool}
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
