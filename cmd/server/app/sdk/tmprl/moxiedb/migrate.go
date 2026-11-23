package moxiedb

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
)

const SchemaVersion = 2

//go:embed migrations/001_init.sql
var migration001 []byte

//go:embed migrations/002_deletions.sql
var migration002 []byte

// CheckSchema verifies the ordered externally installed migrations, without writes.
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
		migrations := [][]byte{migration001, migration002}
		if version != int64(count) || count > len(migrations) || checksum != fmt.Sprintf("%x", sha256.Sum256(migrations[count-1])) {
			return fmt.Errorf("%w: schema version or checksum mismatch", ErrConflict)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != SchemaVersion {
		return fmt.Errorf("%w: schema migration missing", ErrConflict)
	}
	for _, table := range []string{"schema_migrations", "catalog_initialization", "datasets", "dataset_publications", "dataset_versions", "dataset_version_documents", "jobs", "job_attempts", "job_intents", "deletion_operations"} {
		var exists bool
		if err := db.sql.QueryRowContext(ctx, `SELECT EXISTS (SELECT FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relname=$1 AND c.relkind IN ('r','p'))`, table).Scan(&exists); err != nil {
			return fmt.Errorf("check schema table %s: %w", table, err)
		}
		if !exists {
			return fmt.Errorf("%w: schema table %s missing", ErrConflict, table)
		}
	}
	for _, table := range []string{"datasets", "dataset_versions"} {
		var exists bool
		if err := db.sql.QueryRowContext(ctx, `SELECT EXISTS(SELECT FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=$1 AND column_name='deleted_at' AND data_type='timestamp with time zone')`, table).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: schema column %s.deleted_at missing", ErrConflict, table)
		}
	}
	return nil
}
