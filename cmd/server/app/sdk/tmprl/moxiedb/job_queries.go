package moxiedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const MaxPayloadBytes = 65536

// Job is immutable submission metadata. Nullable fields retain SQL absence.
type Job struct {
	JobID              string
	Mode               string
	SourceUploadID     sql.NullString
	TargetDatasetID    sql.NullString
	BaseDatasetVersion sql.NullInt64
	CreatedAt          time.Time
}

// AttemptObservation is a bounded runner observation, never workflow authority.
// A nil Observation means SQL NULL; JSON null remains the bytes "null".
type AttemptObservation struct {
	ExecutionStatus, Stage, Step sql.NullString
	SourceUpdatedAt, ObservedAt  sql.NullTime
	StateStale                   bool
	StartedAt, ClosedAt          sql.NullTime
	ResultDatasetID              sql.NullString
	ResultDatasetVersion         sql.NullInt64
	Observation                  json.RawMessage
	ErrorCode, ErrorMessage      sql.NullString
}

type Attempt struct {
	AttemptID, JobID              string
	AttemptNumber                 int
	PreviousAttemptID             sql.NullString
	TemporalNamespace, WorkflowID string
	RunID                         sql.NullString
	ObservationRevision           int64
	AttemptObservation
}

// Intent binds a stable ID to a normalized command and caller-computed digest.
// The caller owns command normalization; storage compares both JSONB and digest.
type Intent struct {
	IntentID, JobID, AttemptID, Kind string
	CheckpointID                     sql.NullString
	Payload                          json.RawMessage
	PayloadSHA256, State             string
	CreatedAt, UpdatedAt             time.Time
	ErrorCode                        sql.NullString
}

// CatalogEntry combines immutable submission metadata, one attempt, and its
// pending/needs_attention intents. Payloads remain internal storage data.
type CatalogEntry struct {
	Job     Job
	Attempt Attempt
	Pending []Intent
}

// CatalogFilter filters latest attempts before newest-job pagination.
// Namespace is required; an empty Statuses slice includes unobserved attempts.
type CatalogFilter struct {
	Namespace, Mode, TargetDatasetID string
	Statuses                         []string
	Limit, Offset                    int
	Oldest                           bool // Oldest lists jobs oldest first instead of newest first.
}

// page applies a default limit of 100, a maximum of 1000, and a zero-based offset.
func page(limit, offset int) (int, error) {
	if limit < 0 || limit > 1000 || offset < 0 {
		return 0, ErrConflict
	}
	if limit == 0 {
		limit = 100
	}
	return limit, nil
}

func validJSON(p json.RawMessage) bool { return len(p) <= MaxPayloadBytes && json.Valid(p) }

func nullableJSON(p json.RawMessage) any {
	if p == nil {
		return nil
	}
	return []byte(p)
}

const jobColumns = `job_id,mode,source_upload_id,target_dataset_id,base_dataset_version,created_at`
const attemptColumns = `attempt_id,job_id,attempt_number,previous_attempt_id,temporal_namespace,workflow_id,run_id,observation_revision,execution_status,stage,step,source_updated_at,observed_at,state_stale,started_at,closed_at,result_dataset_id,result_dataset_version,observation,error_code,error_message`

// catalogColumns selects a job row joined with one of its attempts (j, a).
var catalogColumns = `j.` + strings.ReplaceAll(jobColumns, ",", ",j.") + `,a.` + strings.ReplaceAll(attemptColumns, ",", ",a.")

const intentColumns = `intent_id,job_id,attempt_id,kind,checkpoint_id,payload,payload_sha256,state,created_at,updated_at,error_code`

func scanAttempt(s scanner) (Attempt, error) {
	var a Attempt
	var observation []byte
	err := s.Scan(&a.AttemptID, &a.JobID, &a.AttemptNumber, &a.PreviousAttemptID, &a.TemporalNamespace, &a.WorkflowID, &a.RunID, &a.ObservationRevision, &a.ExecutionStatus, &a.Stage, &a.Step, &a.SourceUpdatedAt, &a.ObservedAt, &a.StateStale, &a.StartedAt, &a.ClosedAt, &a.ResultDatasetID, &a.ResultDatasetVersion, &observation, &a.ErrorCode, &a.ErrorMessage)
	a.Observation = observation
	return a, dbError(err)
}

func scanIntent(s scanner) (Intent, error) {
	var i Intent
	var payload []byte
	err := s.Scan(&i.IntentID, &i.JobID, &i.AttemptID, &i.Kind, &i.CheckpointID, &payload, &i.PayloadSHA256, &i.State, &i.CreatedAt, &i.UpdatedAt, &i.ErrorCode)
	i.Payload = payload
	return i, dbError(err)
}

func scanCatalog(s scanner) (CatalogEntry, error) {
	var e CatalogEntry
	j, a := &e.Job, &e.Attempt
	var observation []byte
	err := s.Scan(&j.JobID, &j.Mode, &j.SourceUploadID, &j.TargetDatasetID, &j.BaseDatasetVersion, &j.CreatedAt,
		&a.AttemptID, &a.JobID, &a.AttemptNumber, &a.PreviousAttemptID, &a.TemporalNamespace, &a.WorkflowID, &a.RunID, &a.ObservationRevision, &a.ExecutionStatus, &a.Stage, &a.Step, &a.SourceUpdatedAt, &a.ObservedAt, &a.StateStale, &a.StartedAt, &a.ClosedAt, &a.ResultDatasetID, &a.ResultDatasetVersion, &observation, &a.ErrorCode, &a.ErrorMessage)
	a.Observation = observation
	e.Pending = []Intent{}
	return e, dbError(err)
}

// ListCatalog selects the highest-numbered attempt per job, then applies all
// filters before ordering by job created_at and job_id (newest first unless
// Oldest) and paginating.
// Jobs without attempts have no namespace and are not catalog entries.
func (db *DB) ListCatalog(ctx context.Context, f CatalogFilter) ([]CatalogEntry, error) {
	if f.Namespace == "" {
		return nil, ErrConflict
	}
	limit, err := page(f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	order := "j.created_at DESC,j.job_id DESC"
	if f.Oldest {
		order = "j.created_at,j.job_id"
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT `+catalogColumns+` FROM jobs j
		JOIN LATERAL (SELECT * FROM job_attempts WHERE job_id=j.job_id ORDER BY attempt_number DESC LIMIT 1) a ON true
		WHERE a.temporal_namespace=$1 AND ($2='' OR j.mode=$2) AND ($3='' OR j.target_dataset_id=$3)
		AND ($4 OR COALESCE(a.execution_status,'')=ANY($5::text[]))
		ORDER BY `+order+` LIMIT $6 OFFSET $7`, f.Namespace, f.Mode, f.TargetDatasetID, len(f.Statuses) == 0, f.Statuses, limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []CatalogEntry{}
	for rows.Next() {
		e, err := scanCatalog(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := db.catalogPending(ctx, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// GetCatalogExecution looks up an exact execution, including historical attempts.
// Namespace and runID are required; it never substitutes a latest execution.
func (db *DB) GetCatalogExecution(ctx context.Context, namespace, workflowID, runID string) (CatalogEntry, error) {
	if namespace == "" || runID == "" {
		return CatalogEntry{}, ErrConflict
	}
	e, err := scanCatalog(db.sql.QueryRowContext(ctx, `SELECT `+catalogColumns+` FROM jobs j JOIN job_attempts a ON a.job_id=j.job_id WHERE a.temporal_namespace=$1 AND a.workflow_id=$2 AND a.run_id=$3`, namespace, workflowID, runID))
	if err != nil {
		return CatalogEntry{}, err
	}
	entries := []CatalogEntry{e}
	if err := db.catalogPending(ctx, entries); err != nil {
		return CatalogEntry{}, err
	}
	return entries[0], nil
}

// Fetch only the selected attempts' intents, rather than scanning the global queue.
func (db *DB) catalogPending(ctx context.Context, entries []CatalogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	ids := make([]string, len(entries))
	index := make(map[string]int, len(entries))
	for n, e := range entries {
		ids[n] = e.Attempt.AttemptID
		index[ids[n]] = n
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT `+intentColumns+` FROM job_intents WHERE attempt_id=ANY($1::text[]) AND state IN ('pending','needs_attention') ORDER BY created_at,intent_id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		i, err := scanIntent(rows)
		if err != nil {
			return err
		}
		n := index[i.AttemptID]
		entries[n].Pending = append(entries[n].Pending, i)
	}
	return rows.Err()
}

// CreateJob inserts or verifies immutable inputs. CreatedAt is assigned by SQL.
func (db *DB) CreateJob(ctx context.Context, j Job) error {
	tx, err := db.catalogTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT FROM jobs WHERE job_id=$1)`, j.JobID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if err := guardDataset(ctx, tx, j.TargetDatasetID.String, int(j.BaseDatasetVersion.Int64)); err != nil {
			return err
		}
	}
	r, err := tx.ExecContext(ctx, `INSERT INTO jobs(job_id,mode,source_upload_id,target_dataset_id,base_dataset_version) VALUES ($1,$2,$3,$4,$5) ON CONFLICT(job_id) DO UPDATE SET job_id=jobs.job_id WHERE jobs.mode=EXCLUDED.mode AND jobs.source_upload_id IS NOT DISTINCT FROM EXCLUDED.source_upload_id AND jobs.target_dataset_id IS NOT DISTINCT FROM EXCLUDED.target_dataset_id AND jobs.base_dataset_version IS NOT DISTINCT FROM EXCLUDED.base_dataset_version`, j.JobID, j.Mode, j.SourceUploadID, j.TargetDatasetID, j.BaseDatasetVersion)
	if err := changed(r, err); err != nil {
		return err
	}
	return dbError(tx.Commit())
}

// CreateAttempt binds identity/lineage, optionally with an already known run ID.
// Observation inputs are ignored. Replaying a pre-run request after linkage is safe.
func (db *DB) CreateAttempt(ctx context.Context, a Attempt) error {
	tx, err := db.catalogTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := guardExecution(ctx, tx, a.TemporalNamespace, a.WorkflowID, a.RunID.String); err != nil {
		return err
	}
	stored, err := scanAttempt(tx.QueryRowContext(ctx, `SELECT `+attemptColumns+` FROM job_attempts WHERE attempt_id=$1`, a.AttemptID))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if err == nil {
		if err := guardExecution(ctx, tx, stored.TemporalNamespace, stored.WorkflowID, stored.RunID.String); err != nil {
			return err
		}
	}
	r, err := tx.ExecContext(ctx, `INSERT INTO job_attempts(attempt_id,job_id,attempt_number,previous_attempt_id,temporal_namespace,workflow_id,run_id) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(attempt_id) DO UPDATE SET attempt_id=job_attempts.attempt_id WHERE job_attempts.job_id=EXCLUDED.job_id AND job_attempts.attempt_number=EXCLUDED.attempt_number AND job_attempts.previous_attempt_id IS NOT DISTINCT FROM EXCLUDED.previous_attempt_id AND job_attempts.temporal_namespace=EXCLUDED.temporal_namespace AND job_attempts.workflow_id=EXCLUDED.workflow_id AND (EXCLUDED.run_id IS NULL OR job_attempts.run_id=EXCLUDED.run_id)`, a.AttemptID, a.JobID, a.AttemptNumber, a.PreviousAttemptID, a.TemporalNamespace, a.WorkflowID, a.RunID)
	if err := changed(r, err); err != nil {
		return err
	}
	return dbError(tx.Commit())
}

func (db *DB) GetAttempt(ctx context.Context, id string) (Attempt, error) {
	return scanAttempt(db.sql.QueryRowContext(ctx, `SELECT `+attemptColumns+` FROM job_attempts WHERE attempt_id=$1`, id))
}

// LinkAttemptRun may fill a missing run ID, but can never replace one.
func (db *DB) LinkAttemptRun(ctx context.Context, id, runID string) error {
	if runID == "" {
		return ErrConflict
	}
	tx, err := db.catalogTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	a, err := scanAttempt(tx.QueryRowContext(ctx, `SELECT `+attemptColumns+` FROM job_attempts WHERE attempt_id=$1`, id))
	if errors.Is(err, ErrNotFound) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if err := guardExecution(ctx, tx, a.TemporalNamespace, a.WorkflowID, runID); err != nil {
		return err
	}
	r, err := tx.ExecContext(ctx, `UPDATE job_attempts SET run_id=$2 WHERE attempt_id=$1 AND (run_id IS NULL OR run_id=$2)`, id, runID)
	if err := changed(r, err); err != nil {
		return err
	}
	return dbError(tx.Commit())
}

// UpdateAttemptObservation uses SQL CAS, independent of runner source timestamps.
// A successful update increments observation_revision exactly once.
func (db *DB) UpdateAttemptObservation(ctx context.Context, id string, expectedRevision int64, o AttemptObservation) error {
	if expectedRevision < 0 || (o.Observation != nil && !validJSON(o.Observation)) {
		return ErrConflict
	}
	r, err := db.sql.ExecContext(ctx, `UPDATE job_attempts SET execution_status=$3,stage=$4,step=$5,source_updated_at=$6,observed_at=$7,state_stale=$8,started_at=$9,closed_at=$10,result_dataset_id=$11,result_dataset_version=$12,observation=$13,error_code=$14,error_message=$15,observation_revision=observation_revision+1 WHERE attempt_id=$1 AND observation_revision=$2`, id, expectedRevision, o.ExecutionStatus, o.Stage, o.Step, o.SourceUpdatedAt, o.ObservedAt, o.StateStale, o.StartedAt, o.ClosedAt, o.ResultDatasetID, o.ResultDatasetVersion, nullableJSON(o.Observation), o.ErrorCode, o.ErrorMessage)
	return changed(r, err)
}

// BeginIntent binds all command inputs; supplied state/timestamps are ignored.
// Replays remain valid after settlement, but changed payloads or digests conflict.
func (db *DB) BeginIntent(ctx context.Context, i Intent) error {
	if !validJSON(i.Payload) {
		return ErrConflict
	}
	tx, err := db.catalogTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	e, err := scanCatalog(tx.QueryRowContext(ctx, `SELECT `+catalogColumns+` FROM jobs j JOIN job_attempts a ON a.job_id=j.job_id WHERE a.attempt_id=$1 AND j.job_id=$2`, i.AttemptID, i.JobID))
	if errors.Is(err, ErrNotFound) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if i.Kind == "submit" || i.Kind == "authorize_training" {
		if err := guardDataset(ctx, tx, e.Job.TargetDatasetID.String, int(e.Job.BaseDatasetVersion.Int64)); err != nil {
			return err
		}
		if err := guardDataset(ctx, tx, e.Attempt.ResultDatasetID.String, int(e.Attempt.ResultDatasetVersion.Int64)); err != nil {
			return err
		}
	}
	if err := guardExecution(ctx, tx, e.Attempt.TemporalNamespace, e.Attempt.WorkflowID, e.Attempt.RunID.String); err != nil {
		return err
	}
	r, err := tx.ExecContext(ctx, `INSERT INTO job_intents(intent_id,job_id,attempt_id,kind,checkpoint_id,payload,payload_sha256) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(intent_id) DO UPDATE SET intent_id=job_intents.intent_id WHERE job_intents.job_id=EXCLUDED.job_id AND job_intents.attempt_id=EXCLUDED.attempt_id AND job_intents.kind=EXCLUDED.kind AND job_intents.checkpoint_id IS NOT DISTINCT FROM EXCLUDED.checkpoint_id AND job_intents.payload=EXCLUDED.payload AND job_intents.payload_sha256=EXCLUDED.payload_sha256`, i.IntentID, i.JobID, i.AttemptID, i.Kind, i.CheckpointID, []byte(i.Payload), i.PayloadSHA256)
	if err := changed(r, err); err != nil {
		return err
	}
	return dbError(tx.Commit())
}

func (db *DB) GetIntent(ctx context.Context, id string) (Intent, error) {
	return scanIntent(db.sql.QueryRowContext(ctx, `SELECT `+intentColumns+` FROM job_intents WHERE intent_id=$1`, id))
}

func (db *DB) PendingIntents(ctx context.Context) ([]Intent, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+intentColumns+` FROM job_intents WHERE state IN ('pending','needs_attention') ORDER BY created_at,intent_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Intent{}
	for rows.Next() {
		i, err := scanIntent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, i)
	}
	return result, rows.Err()
}

// SettleIntent allows pending/needs_attention to settle and exact settled replays.
// Accepted/rejected outcomes cannot be changed or returned to pending.
func (db *DB) SettleIntent(ctx context.Context, id, state string, code sql.NullString) error {
	if state != "accepted" && state != "rejected" && state != "needs_attention" {
		return ErrConflict
	}
	r, err := db.sql.ExecContext(ctx, `UPDATE job_intents SET state=$2,error_code=$3,updated_at=now() WHERE intent_id=$1 AND (state IN ('pending','needs_attention') OR (state=$2 AND error_code IS NOT DISTINCT FROM $3))`, id, state, code)
	return changed(r, err)
}

// CAS/write-identity methods return ErrConflict for absent or nonmatching rows.
func changed(r sql.Result, err error) error {
	if err != nil {
		return dbError(err)
	}
	n, err := r.RowsAffected()
	if err == nil && n == 0 {
		return ErrConflict
	}
	return err
}

// catalogTx serializes execution identity changes with deletion registration.
// Dataset locks are always acquired after this lock; CommitVersion needs only
// the dataset lock and never acquires the catalog lock.
func (db *DB) catalogTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(705990122)`); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func guardDataset(ctx context.Context, tx *sql.Tx, id string, version int) error {
	if id == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, id); err != nil {
		return err
	}
	var live bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT FROM dataset_versions v JOIN datasets d USING(dataset_id) WHERE v.dataset_id=$1 AND v.version=$2 AND v.deleted_at IS NULL AND d.deleted_at IS NULL) AND NOT EXISTS(SELECT FROM deletion_operations WHERE dataset_id=$1 AND state='pending')`, id, version).Scan(&live)
	if err != nil {
		return err
	}
	if !live {
		return ErrConflict
	}
	return nil
}

func guardExecution(ctx context.Context, tx *sql.Tx, namespace, workflowID, runID string) error {
	var deleted bool
	err := tx.QueryRowContext(ctx, deletedExecutionSQL, namespace, workflowID, runID).Scan(&deleted)
	if err != nil {
		return err
	}
	if deleted {
		return ErrConflict
	}
	return nil
}

// DeleteExecutionCatalog removes only the exact run. Lineage pointing at the
// selected attempt fails closed (the FK rolls back), never removes siblings.
// Registration of the deletion must precede removal to prevent resurrection.
func (db *DB) DeleteExecutionCatalog(ctx context.Context, namespace, workflowID, runID string) error {
	if namespace == "" || workflowID == "" || runID == "" {
		return ErrConflict
	}
	tx, err := db.catalogTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var registered bool
	if err := tx.QueryRowContext(ctx, deletedExecutionSQL, namespace, workflowID, runID).Scan(&registered); err != nil {
		return err
	}
	if !registered {
		return ErrConflict
	}
	var attemptID, jobID string
	err = tx.QueryRowContext(ctx, `SELECT attempt_id,job_id FROM job_attempts WHERE temporal_namespace=$1 AND workflow_id=$2 AND run_id=$3 FOR UPDATE`, namespace, workflowID, runID).Scan(&attemptID, &jobID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM job_intents WHERE attempt_id=$1`, attemptID); err != nil {
		return dbError(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM job_attempts WHERE attempt_id=$1`, attemptID); err != nil {
		return dbError(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM jobs WHERE job_id=$1 AND NOT EXISTS(SELECT FROM job_attempts WHERE job_id=$1)`, jobID); err != nil {
		return dbError(err)
	}
	return dbError(tx.Commit())
}

// DatasetExecutions includes every attempt using or producing the dataset,
// including historical attempts, unlinked runs and other namespaces.
func (db *DB) DatasetExecutions(ctx context.Context, datasetID string) ([]CatalogEntry, error) {
	if datasetID == "" {
		return nil, ErrConflict
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT `+catalogColumns+` FROM jobs j JOIN job_attempts a ON a.job_id=j.job_id WHERE j.target_dataset_id=$1 OR a.result_dataset_id=$1 ORDER BY j.created_at,j.job_id,a.attempt_number`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []CatalogEntry{}
	for rows.Next() {
		e, err := scanCatalog(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := db.catalogPending(ctx, entries); err != nil {
		return nil, err
	}
	return entries, nil
}
