package moxiedb

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
)

const SchemaVersion = 1

//go:embed migrations/001_init.sql
var migration001 []byte

// CheckSchema verifies the exact externally installed migration, without writes.
func (db *DB) CheckSchema(ctx context.Context) error {
	rows, err := db.sql.QueryContext(ctx, "SELECT version, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version int64
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return err
		}
		count++
		if version != SchemaVersion || checksum != fmt.Sprintf("%x", sha256.Sum256(migration001)) {
			return fmt.Errorf("%w: schema version or checksum mismatch", ErrConflict)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("%w: schema migration missing", ErrConflict)
	}
	for _, table := range []string{"schema_migrations", "catalog_initialization", "datasets", "dataset_publications", "dataset_versions", "dataset_version_documents", "jobs", "job_attempts", "job_intents"} {
		var exists bool
		if err := db.sql.QueryRowContext(ctx, `SELECT EXISTS (SELECT FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relname=$1 AND c.relkind IN ('r','p'))`, table).Scan(&exists); err != nil {
			return fmt.Errorf("check schema table %s: %w", table, err)
		}
		if !exists {
			return fmt.Errorf("%w: schema table %s missing", ErrConflict, table)
		}
	}
	return nil
}
