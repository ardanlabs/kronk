package moxiedb

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"
)

type Dataset struct {
	ID, Name             string
	CurrentVersion       int
	CreatedAt, UpdatedAt time.Time
}

type Version struct {
	DatasetID                    string
	Version                      int
	SnapshotPath, SnapshotSHA256 string
	SnapshotBytes, RecordCount   int64
	TrainingReady                bool
	CreatedAt                    time.Time
	PublicationID                string
}

type Document struct {
	ID, Name, SourceSHA256       string
	Position, PageStart, PageEnd int
	RecordCount                  int64
}

type Publication struct {
	ID, DatasetID, Kind, InputDigest               string
	BaseVersion, TargetVersion                     int
	SnapshotPath, SnapshotSHA256, State, ErrorCode string
	CreatedAt, UpdatedAt                           time.Time
}

const publicationColumns = `publication_id,dataset_id,kind,input_digest,base_version,target_version,snapshot_path,snapshot_sha256,state,error_code,created_at,updated_at`
const versionColumns = `dataset_id,version,snapshot_path,snapshot_sha256,snapshot_bytes,record_count,training_ready,created_at,publication_id`

func scanPublication(s scanner) (Publication, error) {
	var p Publication
	var base, target sql.NullInt64
	var datasetID, path, hash, code sql.NullString
	err := s.Scan(&p.ID, &datasetID, &p.Kind, &p.InputDigest, &base, &target, &path, &hash, &p.State, &code, &p.CreatedAt, &p.UpdatedAt)
	p.DatasetID, p.ErrorCode = datasetID.String, code.String
	p.BaseVersion, p.TargetVersion = int(base.Int64), int(target.Int64)
	p.SnapshotPath, p.SnapshotSHA256 = path.String, hash.String
	return p, dbError(err)
}

func scanVersion(s scanner) (Version, error) {
	var v Version
	err := s.Scan(&v.DatasetID, &v.Version, &v.SnapshotPath, &v.SnapshotSHA256, &v.SnapshotBytes, &v.RecordCount, &v.TrainingReady, &v.CreatedAt, &v.PublicationID)
	return v, dbError(err)
}

func samePublication(a, b Publication) bool {
	return a.ID == b.ID && a.DatasetID == b.DatasetID && a.Kind == b.Kind && a.InputDigest == b.InputDigest && a.BaseVersion == b.BaseVersion
}

// BeginPublication binds an operation ID permanently to its immutable inputs.
// Supplied outcomes, state and timestamps are ignored: a new intent is always
// pending with unknown outcome until CommitVersion binds the verified snapshot.
func (db *DB) BeginPublication(ctx context.Context, p Publication) error {
	_, err := db.sql.ExecContext(ctx, `INSERT INTO dataset_publications (publication_id,dataset_id,kind,input_digest,base_version) VALUES ($1,NULLIF($2,''),$3,$4,NULLIF($5,0)) ON CONFLICT (publication_id) DO NOTHING`, p.ID, p.DatasetID, p.Kind, p.InputDigest, p.BaseVersion)
	if err != nil {
		return dbError(err)
	}
	stored, err := db.GetPublication(ctx, p.ID)
	if err != nil {
		return err
	}
	if !samePublication(stored, p) {
		return ErrConflict
	}
	return nil
}

func (db *DB) GetPublication(ctx context.Context, id string) (Publication, error) {
	return scanPublication(db.sql.QueryRowContext(ctx, `SELECT `+publicationColumns+` FROM dataset_publications WHERE publication_id=$1`, id))
}

func (db *DB) PendingPublications(ctx context.Context) ([]Publication, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+publicationColumns+` FROM dataset_publications WHERE state IN ('pending','needs_attention') ORDER BY created_at,publication_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Publication{}
	for rows.Next() {
		p, err := scanPublication(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (db *DB) MarkPublication(ctx context.Context, id, state, code string) error {
	if state != "conflict" && state != "needs_attention" {
		return ErrConflict
	}
	r, err := db.sql.ExecContext(ctx, `UPDATE dataset_publications SET state=$2,error_code=NULLIF($3,''),updated_at=now() WHERE publication_id=$1 AND (state IN ('pending','needs_attention') OR state=$2)`, id, state, code)
	if err != nil {
		return dbError(err)
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		if _, err := db.GetPublication(ctx, id); err != nil {
			return err
		}
		return ErrConflict
	}
	return nil
}

// CommitVersion atomically publishes a verified snapshot. All file work must have
// finished before this call. Existing immutable rows must match on replay.
func (db *DB) CommitVersion(ctx context.Context, d Dataset, v Version, docs []Document) error {
	if d.ID != v.DatasetID || d.CurrentVersion != v.Version {
		return ErrConflict
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Also serialize first publication, before a datasets row exists. A hash
	// collision only serializes unrelated datasets; it cannot change identity.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, d.ID); err != nil {
		return err
	}
	p, err := scanPublication(tx.QueryRowContext(ctx, `SELECT `+publicationColumns+` FROM dataset_publications WHERE publication_id=$1 FOR UPDATE`, v.PublicationID))
	if err != nil {
		return err
	}
	if p.DatasetID != d.ID || (p.TargetVersion != 0 && p.TargetVersion != v.Version) || (p.SnapshotPath != "" && p.SnapshotPath != v.SnapshotPath) || (p.SnapshotSHA256 != "" && p.SnapshotSHA256 != v.SnapshotSHA256) || p.State == "conflict" {
		return ErrConflict
	}
	var head int
	var name string
	err = tx.QueryRowContext(ctx, `SELECT current_version,name FROM datasets WHERE dataset_id=$1 FOR UPDATE`, d.ID).Scan(&head, &name)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && name != d.Name {
		return ErrConflict
	}
	stored, e := scanVersion(tx.QueryRowContext(ctx, `SELECT `+versionColumns+` FROM dataset_versions WHERE dataset_id=$1 AND version=$2`, v.DatasetID, v.Version))
	switch {
	case e == nil:
		stored.CreatedAt = time.Time{}
		v.CreatedAt = time.Time{}
		if stored != v {
			return ErrConflict
		}
	case errors.Is(e, ErrNotFound):
		if p.State == "committed" || v.Version != head+1 || p.BaseVersion > head {
			return ErrConflict
		}
		switch p.Kind {
		case "create":
			if head != 0 || v.Version != 1 {
				return ErrConflict
			}
		case "add":
			if p.BaseVersion <= 0 {
				return ErrConflict
			}
		case "import":
			if p.BaseVersion != 0 && p.BaseVersion != head {
				return ErrConflict
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO dataset_versions (`+versionColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,COALESCE($8,now()),$9)`, v.DatasetID, v.Version, v.SnapshotPath, v.SnapshotSHA256, v.SnapshotBytes, v.RecordCount, v.TrainingReady, nullableTime(v.CreatedAt), v.PublicationID)
		if err != nil {
			return dbError(err)
		}
		for _, doc := range docs {
			_, err = tx.ExecContext(ctx, `INSERT INTO dataset_version_documents(dataset_id,version,document_id,name,source_sha256,position,page_start,page_end,record_count) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, v.DatasetID, v.Version, doc.ID, doc.Name, doc.SourceSHA256, doc.Position, doc.PageStart, doc.PageEnd, doc.RecordCount)
			if err != nil {
				return dbError(err)
			}
		}
	default:
		return e
	}
	// Compare the entire document set, including count and ordering, on replay.
	rows, err := tx.QueryContext(ctx, `SELECT document_id,name,source_sha256,position,page_start,page_end,record_count FROM dataset_version_documents WHERE dataset_id=$1 AND version=$2 ORDER BY position,document_id`, v.DatasetID, v.Version)
	if err != nil {
		return err
	}
	actual := []Document{}
	for rows.Next() {
		var doc Document
		if err = rows.Scan(&doc.ID, &doc.Name, &doc.SourceSHA256, &doc.Position, &doc.PageStart, &doc.PageEnd, &doc.RecordCount); err != nil {
			rows.Close()
			return err
		}
		actual = append(actual, doc)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	expected := append([]Document{}, docs...)
	slices.SortFunc(expected, func(a, b Document) int { return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID)) })
	if !slices.Equal(actual, expected) {
		return ErrConflict
	}
	if head < v.Version {
		_, err = tx.ExecContext(ctx, `INSERT INTO datasets(dataset_id,name,current_version,created_at,updated_at) VALUES ($1,$2,$3,COALESCE($4,now()),COALESCE($5,now())) ON CONFLICT(dataset_id) DO UPDATE SET current_version=EXCLUDED.current_version,updated_at=EXCLUDED.updated_at`, d.ID, d.Name, v.Version, nullableTime(d.CreatedAt), nullableTime(d.UpdatedAt))
		if err != nil {
			return dbError(err)
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE dataset_publications SET target_version=$2,snapshot_path=$3,snapshot_sha256=$4,state='committed',error_code=NULL,updated_at=now() WHERE publication_id=$1`, p.ID, v.Version, v.SnapshotPath, v.SnapshotSHA256)
	if err != nil {
		return dbError(err)
	}
	return dbError(tx.Commit())
}

func nullableTime(t time.Time) sql.NullTime { return sql.NullTime{Time: t, Valid: !t.IsZero()} }

func (db *DB) GetVersion(ctx context.Context, datasetID string, version int) (Version, error) {
	return scanVersion(db.sql.QueryRowContext(ctx, `SELECT `+versionColumns+` FROM dataset_versions WHERE dataset_id=$1 AND version=CASE WHEN $2=0 THEN (SELECT current_version FROM datasets WHERE dataset_id=$1) ELSE $2 END`, datasetID, version))
}

func (db *DB) ListDatasets(ctx context.Context) ([]Dataset, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT dataset_id,name,current_version,created_at,updated_at FROM datasets ORDER BY updated_at DESC,dataset_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Dataset{}
	for rows.Next() {
		var d Dataset
		if err = rows.Scan(&d.ID, &d.Name, &d.CurrentVersion, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (db *DB) ListVersions(ctx context.Context, datasetID string) ([]Version, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+versionColumns+` FROM dataset_versions WHERE dataset_id=$1 ORDER BY version DESC`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Version{}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (db *DB) MarkCatalogReady(ctx context.Context) error {
	r, err := db.sql.ExecContext(ctx, `UPDATE catalog_initialization SET state='ready',ready_at=COALESCE(ready_at,now()) WHERE singleton`)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}
