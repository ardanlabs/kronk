package moxiedb

import (
	"context"
	"database/sql"
	"encoding/json"
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

// Pagination uses a default limit of 100, a maximum of 1000, and zero-based offset.
type JobFilter struct {
	Mode, TargetDatasetID string
	Limit, Offset         int
}

type AttemptFilter struct {
	JobID, ExecutionStatus string
	Limit, Offset          int
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
}

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
const intentColumns = `intent_id,job_id,attempt_id,kind,checkpoint_id,payload,payload_sha256,state,created_at,updated_at,error_code`

func scanJob(s scanner) (Job, error) {
	var j Job
	err := s.Scan(&j.JobID, &j.Mode, &j.SourceUploadID, &j.TargetDatasetID, &j.BaseDatasetVersion, &j.CreatedAt)
	return j, dbError(err)
}

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
// filters before ordering by job created_at DESC, job_id DESC and paginating.
// Jobs without attempts have no namespace and are not catalog entries.
func (db *DB) ListCatalog(ctx context.Context, f CatalogFilter) ([]CatalogEntry, error) {
	if f.Namespace == "" {
		return nil, ErrConflict
	}
	limit, err := page(f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	columns := `j.` + strings.ReplaceAll(jobColumns, ",", ",j.") + `,a.` + strings.ReplaceAll(attemptColumns, ",", ",a.")
	rows, err := db.sql.QueryContext(ctx, `SELECT `+columns+` FROM jobs j
		JOIN LATERAL (SELECT * FROM job_attempts WHERE job_id=j.job_id ORDER BY attempt_number DESC LIMIT 1) a ON true
		WHERE a.temporal_namespace=$1 AND ($2='' OR j.mode=$2) AND ($3='' OR j.target_dataset_id=$3)
		AND ($4 OR COALESCE(a.execution_status,'')=ANY($5::text[]))
		ORDER BY j.created_at DESC,j.job_id DESC LIMIT $6 OFFSET $7`, f.Namespace, f.Mode, f.TargetDatasetID, len(f.Statuses) == 0, f.Statuses, limit, f.Offset)
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
	columns := `j.` + strings.ReplaceAll(jobColumns, ",", ",j.") + `,a.` + strings.ReplaceAll(attemptColumns, ",", ",a.")
	e, err := scanCatalog(db.sql.QueryRowContext(ctx, `SELECT `+columns+` FROM jobs j JOIN job_attempts a ON a.job_id=j.job_id WHERE a.temporal_namespace=$1 AND a.workflow_id=$2 AND a.run_id=$3`, namespace, workflowID, runID))
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
	r, err := db.sql.ExecContext(ctx, `INSERT INTO jobs(job_id,mode,source_upload_id,target_dataset_id,base_dataset_version) VALUES ($1,$2,$3,$4,$5) ON CONFLICT(job_id) DO UPDATE SET job_id=jobs.job_id WHERE jobs.mode=EXCLUDED.mode AND jobs.source_upload_id IS NOT DISTINCT FROM EXCLUDED.source_upload_id AND jobs.target_dataset_id IS NOT DISTINCT FROM EXCLUDED.target_dataset_id AND jobs.base_dataset_version IS NOT DISTINCT FROM EXCLUDED.base_dataset_version`, j.JobID, j.Mode, j.SourceUploadID, j.TargetDatasetID, j.BaseDatasetVersion)
	return changed(r, err)
}

func (db *DB) GetJob(ctx context.Context, id string) (Job, error) {
	return scanJob(db.sql.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE job_id=$1`, id))
}

// ListJobs orders newest first, breaking timestamp ties by job_id descending.
func (db *DB) ListJobs(ctx context.Context, f JobFilter) ([]Job, error) {
	limit, err := page(f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE ($1='' OR mode=$1) AND ($2='' OR target_dataset_id=$2) ORDER BY created_at DESC,job_id DESC LIMIT $3 OFFSET $4`, f.Mode, f.TargetDatasetID, limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, j)
	}
	return result, rows.Err()
}

// CreateAttempt binds identity/lineage, optionally with an already known run ID.
// Observation inputs are ignored. Replaying a pre-run request after linkage is safe.
func (db *DB) CreateAttempt(ctx context.Context, a Attempt) error {
	r, err := db.sql.ExecContext(ctx, `INSERT INTO job_attempts(attempt_id,job_id,attempt_number,previous_attempt_id,temporal_namespace,workflow_id,run_id) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(attempt_id) DO UPDATE SET attempt_id=job_attempts.attempt_id WHERE job_attempts.job_id=EXCLUDED.job_id AND job_attempts.attempt_number=EXCLUDED.attempt_number AND job_attempts.previous_attempt_id IS NOT DISTINCT FROM EXCLUDED.previous_attempt_id AND job_attempts.temporal_namespace=EXCLUDED.temporal_namespace AND job_attempts.workflow_id=EXCLUDED.workflow_id AND (EXCLUDED.run_id IS NULL OR job_attempts.run_id=EXCLUDED.run_id)`, a.AttemptID, a.JobID, a.AttemptNumber, a.PreviousAttemptID, a.TemporalNamespace, a.WorkflowID, a.RunID)
	return changed(r, err)
}

func (db *DB) GetAttempt(ctx context.Context, id string) (Attempt, error) {
	return scanAttempt(db.sql.QueryRowContext(ctx, `SELECT `+attemptColumns+` FROM job_attempts WHERE attempt_id=$1`, id))
}

// LinkAttemptRun may fill a missing run ID, but can never replace one.
func (db *DB) LinkAttemptRun(ctx context.Context, id, runID string) error {
	if runID == "" {
		return ErrConflict
	}
	r, err := db.sql.ExecContext(ctx, `UPDATE job_attempts SET run_id=$2 WHERE attempt_id=$1 AND (run_id IS NULL OR run_id=$2)`, id, runID)
	return changed(r, err)
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

// ListAttempts orders a job's attempts by number descending. Across jobs it
// orders by started_at descending (unknown starts last), then attempt_id descending.
func (db *DB) ListAttempts(ctx context.Context, f AttemptFilter) ([]Attempt, error) {
	limit, err := page(f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	order := `started_at DESC NULLS LAST,attempt_id DESC`
	if f.JobID != "" {
		order = `attempt_number DESC,attempt_id DESC`
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT `+attemptColumns+` FROM job_attempts WHERE ($1='' OR job_id=$1) AND ($2='' OR execution_status=$2) ORDER BY `+order+` LIMIT $3 OFFSET $4`, f.JobID, f.ExecutionStatus, limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Attempt{}
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// BeginIntent binds all command inputs; supplied state/timestamps are ignored.
// Replays remain valid after settlement, but changed payloads or digests conflict.
func (db *DB) BeginIntent(ctx context.Context, i Intent) error {
	if !validJSON(i.Payload) {
		return ErrConflict
	}
	r, err := db.sql.ExecContext(ctx, `INSERT INTO job_intents(intent_id,job_id,attempt_id,kind,checkpoint_id,payload,payload_sha256) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(intent_id) DO UPDATE SET intent_id=job_intents.intent_id WHERE job_intents.job_id=EXCLUDED.job_id AND job_intents.attempt_id=EXCLUDED.attempt_id AND job_intents.kind=EXCLUDED.kind AND job_intents.checkpoint_id IS NOT DISTINCT FROM EXCLUDED.checkpoint_id AND job_intents.payload=EXCLUDED.payload AND job_intents.payload_sha256=EXCLUDED.payload_sha256`, i.IntentID, i.JobID, i.AttemptID, i.Kind, i.CheckpointID, []byte(i.Payload), i.PayloadSHA256)
	return changed(r, err)
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
