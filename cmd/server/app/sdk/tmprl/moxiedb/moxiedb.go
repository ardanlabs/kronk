// Package moxiedb stores Moxie catalog and job metadata. It never owns snapshot files.
package moxiedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
