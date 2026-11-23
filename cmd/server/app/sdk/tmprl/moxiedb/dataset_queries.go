package moxiedb

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
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
	var blocked bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT FROM deletion_operations WHERE dataset_id=$1 AND state='pending') OR EXISTS(SELECT FROM dataset_versions WHERE dataset_id=$1 AND version=$2 AND deleted_at IS NOT NULL)`, d.ID, v.Version).Scan(&blocked); err != nil {
		return err
	}
	if blocked {
		return ErrConflict
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
		_, err = tx.ExecContext(ctx, `INSERT INTO datasets(dataset_id,name,current_version,created_at,updated_at) VALUES ($1,$2,$3,COALESCE($4,now()),COALESCE($5,now())) ON CONFLICT(dataset_id) DO UPDATE SET current_version=EXCLUDED.current_version,updated_at=EXCLUDED.updated_at,deleted_at=NULL`, d.ID, d.Name, v.Version, nullableTime(d.CreatedAt), nullableTime(d.UpdatedAt))
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
	return scanVersion(db.sql.QueryRowContext(ctx, `SELECT `+versionColumns+` FROM dataset_versions WHERE dataset_id=$1 AND deleted_at IS NULL AND version=CASE WHEN $2=0 THEN (SELECT max(version) FROM dataset_versions WHERE dataset_id=$1 AND deleted_at IS NULL) ELSE $2 END`, datasetID, version))
}

func (db *DB) ListDatasets(ctx context.Context) ([]Dataset, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT dataset_id,name,(SELECT max(version) FROM dataset_versions v WHERE v.dataset_id=datasets.dataset_id AND v.deleted_at IS NULL),created_at,updated_at FROM datasets WHERE deleted_at IS NULL ORDER BY updated_at DESC,dataset_id DESC`)
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
	rows, err := db.sql.QueryContext(ctx, `SELECT `+versionColumns+` FROM dataset_versions WHERE dataset_id=$1 AND deleted_at IS NULL ORDER BY version DESC`, datasetID)
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

// GetDataset returns allocator metadata even when every version is tombstoned.
func (db *DB) GetDataset(ctx context.Context, id string) (Dataset, error) {
	var d Dataset
	err := db.sql.QueryRowContext(ctx, `SELECT dataset_id,name,current_version,created_at,updated_at FROM datasets WHERE dataset_id=$1`, id).Scan(&d.ID, &d.Name, &d.CurrentVersion, &d.CreatedAt, &d.UpdatedAt)
	return d, dbError(err)
}

// NextVersion is a high-water lookup, not a reservation. CommitVersion serializes allocation.
func (db *DB) NextVersion(ctx context.Context, id string) (int, error) {
	var next int
	err := db.sql.QueryRowContext(ctx, `SELECT COALESCE(max(version),0)+1 FROM dataset_versions WHERE dataset_id=$1`, id).Scan(&next)
	return next, err
}

// Deletion retains metadata only; the caller owns physical deletion and recovery.
type Deletion struct {
	ID, Namespace, WorkflowID, RunID string
	DatasetID                        string
	DatasetVersion                   int
	Manifest                         json.RawMessage
	State, ErrorCode                 string
	CreatedAt, UpdatedAt             time.Time
	CompletedAt                      sql.NullTime
}

const deletionColumns = `operation_id,namespace,workflow_id,run_id,dataset_id,dataset_version,manifest,state,error_code,created_at,updated_at,completed_at`

func scanDeletion(s scanner) (Deletion, error) {
	var d Deletion
	var dataset, code sql.NullString
	var version sql.NullInt64
	var manifest []byte
	err := s.Scan(&d.ID, &d.Namespace, &d.WorkflowID, &d.RunID, &dataset, &version, &manifest, &d.State, &code, &d.CreatedAt, &d.UpdatedAt, &d.CompletedAt)
	d.DatasetID, d.ErrorCode = dataset.String, code.String
	d.DatasetVersion, d.Manifest = int(version.Int64), manifest
	return d, dbError(err)
}

// BeginDeletion binds the exact requested scope, never an inferred dataset scope.
// Only JSONB-equivalent manifests and identical inputs can replay an operation.
func (db *DB) BeginDeletion(ctx context.Context, d Deletion) error {
	if d.ID == "" || d.Namespace == "" || d.WorkflowID == "" || d.RunID == "" || !json.Valid(d.Manifest) || d.DatasetVersion < 0 || (d.DatasetID == "" && d.DatasetVersion != 0) || (d.State != "" && d.State != "pending") || d.ErrorCode != "" {
		return ErrConflict
	}
	// Validate execution identities without interpreting snapshots or widening scope.
	var object map[string]json.RawMessage
	if err := json.Unmarshal(d.Manifest, &object); err != nil || object == nil {
		return ErrConflict
	}
	if raw, ok := object["executions"]; ok {
		var executions []struct {
			Namespace  string `json:"namespace"`
			WorkflowID string `json:"workflow_id"`
			RunID      string `json:"run_id"`
		}
		if err := json.Unmarshal(raw, &executions); err != nil || string(raw) == "null" {
			return ErrConflict
		}
		for _, e := range executions {
			if e.Namespace == "" || e.WorkflowID == "" || e.RunID == "" {
				return ErrConflict
			}
		}
	}
	tx, err := db.catalogTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if d.DatasetID != "" {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, d.DatasetID); err != nil {
			return err
		}
	}
	// Admission and control delivery use the same catalog lock. Reject a
	// training attempt added since the preview, or an action still being delivered,
	// before deleting any history or files. Lineage to a surviving attempt also
	// prevents cleanup rather than silently changing that attempt's provenance.
	var changedScope bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS (
	  SELECT FROM jobs j JOIN job_attempts a USING(job_id)
	  WHERE j.mode='training' AND j.target_dataset_id=$1 AND j.base_dataset_version=$2
	    AND NOT ($3::jsonb->'executions' @> jsonb_build_array(jsonb_build_object('namespace',a.temporal_namespace,'workflow_id',a.workflow_id,'run_id',a.run_id)))
	) OR EXISTS (
	  SELECT FROM job_attempts a WHERE (($3::jsonb->'executions' @> jsonb_build_array(jsonb_build_object('namespace',a.temporal_namespace,'workflow_id',a.workflow_id,'run_id',a.run_id))) OR (a.temporal_namespace=$4 AND a.workflow_id=$5 AND a.run_id=$6))
	    AND (COALESCE(a.execution_status,'') IN ('','queued','running','recovering','waiting','pause_requested','paused') OR EXISTS(SELECT FROM job_intents i WHERE i.attempt_id=a.attempt_id AND i.state IN ('pending','needs_attention')))
	) OR EXISTS (
	  SELECT FROM job_attempts child JOIN job_attempts parent ON parent.attempt_id=child.previous_attempt_id
	  WHERE $3::jsonb->'executions' @> jsonb_build_array(jsonb_build_object('namespace',parent.temporal_namespace,'workflow_id',parent.workflow_id,'run_id',parent.run_id))
	    AND NOT ($3::jsonb->'executions' @> jsonb_build_array(jsonb_build_object('namespace',child.temporal_namespace,'workflow_id',child.workflow_id,'run_id',child.run_id)))
	)`, d.DatasetID, d.DatasetVersion, []byte(d.Manifest), d.Namespace, d.WorkflowID, d.RunID).Scan(&changedScope)
	if err != nil {
		return err
	}
	if changedScope {
		return ErrConflict
	}
	r, err := tx.ExecContext(ctx, `INSERT INTO deletion_operations(operation_id,namespace,workflow_id,run_id,dataset_id,dataset_version,manifest) VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,0),$7) ON CONFLICT(operation_id) DO UPDATE SET operation_id=deletion_operations.operation_id WHERE deletion_operations.namespace=EXCLUDED.namespace AND deletion_operations.workflow_id=EXCLUDED.workflow_id AND deletion_operations.run_id=EXCLUDED.run_id AND deletion_operations.dataset_id IS NOT DISTINCT FROM EXCLUDED.dataset_id AND deletion_operations.dataset_version IS NOT DISTINCT FROM EXCLUDED.dataset_version AND deletion_operations.manifest=EXCLUDED.manifest`, d.ID, d.Namespace, d.WorkflowID, d.RunID, d.DatasetID, d.DatasetVersion, []byte(d.Manifest))
	if err := changed(r, err); err != nil {
		return err
	}
	return dbError(tx.Commit())
}

func (db *DB) GetDeletion(ctx context.Context, namespace, workflowID, runID string) (Deletion, error) {
	if namespace == "" || workflowID == "" || runID == "" {
		return Deletion{}, ErrConflict
	}
	return scanDeletion(db.sql.QueryRowContext(ctx, `SELECT `+deletionColumns+` FROM deletion_operations WHERE namespace=$1 AND workflow_id=$2 AND run_id=$3`, namespace, workflowID, runID))
}

func (db *DB) PendingDeletions(ctx context.Context) ([]Deletion, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+deletionColumns+` FROM deletion_operations WHERE state='pending' ORDER BY created_at,operation_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Deletion{}
	for rows.Next() {
		d, err := scanDeletion(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

var deletionCode = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,128}$`)

func (db *DB) SetDeletionError(ctx context.Context, id, code string) error {
	if code != "" && !deletionCode.MatchString(code) {
		return ErrConflict
	}
	r, err := db.sql.ExecContext(ctx, `UPDATE deletion_operations SET error_code=NULLIF($2,''),updated_at=now() WHERE operation_id=$1 AND state='pending'`, id, code)
	return changed(r, err)
}

// CompleteDeletion is called only after files and catalog removals have succeeded.
func (db *DB) CompleteDeletion(ctx context.Context, id string) error {
	r, err := db.sql.ExecContext(ctx, `UPDATE deletion_operations SET state='completed',error_code=NULL,updated_at=now(),completed_at=COALESCE(completed_at,now()) WHERE operation_id=$1`, id)
	return changed(r, err)
}

const deletedExecutionSQL = `SELECT EXISTS (SELECT FROM deletion_operations WHERE (namespace=$1 AND workflow_id=$2 AND run_id=$3) OR manifest->'executions' @> jsonb_build_array(jsonb_build_object('namespace',$1::text,'workflow_id',$2::text,'run_id',$3::text)))`

func (db *DB) IsDeletedExecution(ctx context.Context, namespace, workflowID, runID string) (bool, error) {
	if namespace == "" || workflowID == "" || runID == "" {
		return false, ErrConflict
	}
	var deleted bool
	err := db.sql.QueryRowContext(ctx, deletedExecutionSQL, namespace, workflowID, runID).Scan(&deleted)
	return deleted, err
}

// ApplyDeletion tombstones only the selected versions
// in one transaction. It never releases dataset allocation numbers or FK targets.
func (db *DB) ApplyDeletion(ctx context.Context, id string, deletedVersions []int) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Acquire the dataset lock before locking the operation, matching BeginDeletion.
	d, err := scanDeletion(tx.QueryRowContext(ctx, `SELECT `+deletionColumns+` FROM deletion_operations WHERE operation_id=$1`, id))
	if err != nil {
		return err
	}
	if d.DatasetID == "" && len(deletedVersions) > 0 {
		return ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, d.DatasetID); err != nil {
		return err
	}
	d, err = scanDeletion(tx.QueryRowContext(ctx, `SELECT `+deletionColumns+` FROM deletion_operations WHERE operation_id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	if d.State != "pending" {
		return ErrConflict
	}
	seen := map[int]bool{}
	for _, version := range deletedVersions {
		if version <= 0 || seen[version] {
			return ErrConflict
		}
		seen[version] = true
		r, err := tx.ExecContext(ctx, `UPDATE dataset_versions SET deleted_at=COALESCE(deleted_at,now()),record_count=0,snapshot_bytes=0,training_ready=false WHERE dataset_id=$1 AND version=$2`, d.DatasetID, version)
		if err := changed(r, err); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM dataset_version_documents WHERE dataset_id=$1 AND version=$2`, d.DatasetID, version); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE datasets SET deleted_at=CASE WHEN EXISTS(SELECT FROM dataset_versions WHERE dataset_id=$1 AND deleted_at IS NULL) THEN NULL ELSE COALESCE(deleted_at,now()) END,updated_at=now() WHERE dataset_id=$1`, d.DatasetID); err != nil {
		return err
	}
	return dbError(tx.Commit())
}
