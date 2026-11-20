// Package moxiedb stores Moxie catalog and job metadata. It never owns snapshot files.
package moxiedb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

var (
	ErrNotFound = errors.New("moxiedb: not found")
	ErrConflict = errors.New("moxiedb: conflict")
)

// DB owns a concurrency-safe connection pool. Open does not install or migrate schema.
type DB struct{ sql *sql.DB }

func Open(ctx context.Context, url string) (*DB, error) {
	if strings.TrimSpace(url) == "" {
		return nil, errors.New("moxiedb: invalid connection configuration")
	}
	config, err := pgx.ParseConfig(url)
	if err != nil {
		// Parse errors can quote the original DSN, including its credentials.
		return nil, errors.New("moxiedb: invalid connection configuration")
	}
	db := stdlib.OpenDB(*config)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping moxiedb: %w", err)
	}
	return &DB{sql: db}, nil
}

func (db *DB) Close() error { return db.sql.Close() }

// IsUnavailable identifies connection failures without treating schema, query,
// or constraint errors as outages. It does not retry SQL or replay transactions.
func IsUnavailable(err error) bool {
	if errors.Is(err, driver.ErrBadConn) {
		return true
	}
	if _, ok := errors.AsType[*net.OpError](err); ok {
		return true
	}
	if _, ok := errors.AsType[*pgconn.ConnectError](err); ok {
		return true
	}
	if pg, ok := errors.AsType[*pgconn.PgError](err); ok {
		return strings.HasPrefix(pg.Code, "08") || pg.Code == "57P01" || pg.Code == "57P02" || pg.Code == "57P03" || pg.Code == "53300"
	}
	return false
}

func dbError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if pg, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pg.Code {
		case "23505", "23503", "23514", "40001", "40P01":
			return fmt.Errorf("%w: %s", ErrConflict, pg.ConstraintName)
		}
	}
	return err
}

type scanner interface{ Scan(...any) error }
