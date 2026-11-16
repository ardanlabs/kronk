CREATE TABLE schema_migrations (
    version bigint PRIMARY KEY CHECK (version > 0),
    checksum text NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE catalog_initialization (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    state text NOT NULL DEFAULT 'initializing' CHECK (state IN ('initializing', 'ready')),
    initializing_at timestamptz NOT NULL DEFAULT now(),
    ready_at timestamptz,
    CHECK ((state = 'ready') = (ready_at IS NOT NULL))
);
INSERT INTO catalog_initialization (singleton) VALUES (true);

CREATE TABLE datasets (
    dataset_id text PRIMARY KEY CHECK (dataset_id <> ''),
    name text NOT NULL CHECK (octet_length(name) BETWEEN 1 AND 200),
    current_version integer NOT NULL CHECK (current_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE dataset_publications (
    publication_id text PRIMARY KEY CHECK (publication_id <> ''),
    dataset_id text CHECK (dataset_id <> ''),
    kind text NOT NULL CHECK (kind IN ('create', 'add', 'remove', 'import')),
    input_digest text NOT NULL CHECK (input_digest ~ '^[0-9a-f]{64}$'),
    base_version integer CHECK (base_version > 0),
    target_version integer CHECK (target_version > COALESCE(base_version, 0)),
    snapshot_path text CHECK (snapshot_path <> '' AND snapshot_path !~ '(^/|(^|/)\.\.(/|$)|\\|^[A-Za-z]:)'),
    snapshot_sha256 text CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'committed', 'conflict', 'needs_attention')),
    error_code text CHECK (length(error_code) <= 128),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (state <> 'committed' OR (dataset_id IS NOT NULL AND target_version IS NOT NULL AND snapshot_path IS NOT NULL AND snapshot_sha256 IS NOT NULL)),
    UNIQUE (publication_id, dataset_id, target_version)
);

CREATE TABLE dataset_versions (
    dataset_id text NOT NULL REFERENCES datasets(dataset_id) DEFERRABLE INITIALLY DEFERRED,
    version integer NOT NULL CHECK (version > 0),
    snapshot_path text NOT NULL UNIQUE CHECK (snapshot_path <> '' AND snapshot_path !~ '(^/|(^|/)\.\.(/|$)|\\|^[A-Za-z]:)'),
    snapshot_sha256 text NOT NULL CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
    snapshot_bytes bigint NOT NULL CHECK (snapshot_bytes >= 0),
    record_count bigint NOT NULL CHECK (record_count >= 0),
    training_ready boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    publication_id text NOT NULL UNIQUE,
    PRIMARY KEY (dataset_id, version),
    FOREIGN KEY (publication_id, dataset_id, version) REFERENCES dataset_publications(publication_id, dataset_id, target_version) DEFERRABLE INITIALLY DEFERRED
);
ALTER TABLE datasets ADD CONSTRAINT datasets_head_fk FOREIGN KEY (dataset_id, current_version)
    REFERENCES dataset_versions(dataset_id, version) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE dataset_version_documents (
    dataset_id text NOT NULL,
    version integer NOT NULL,
    document_id text NOT NULL CHECK (document_id <> ''),
    name text NOT NULL CHECK (name <> ''),
    source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    position integer NOT NULL CHECK (position >= 0),
    page_start integer NOT NULL CHECK (page_start >= 0),
    page_end integer NOT NULL CHECK ((page_start = 0 AND page_end = 0) OR (page_start > 0 AND page_end >= page_start)),
    record_count bigint NOT NULL CHECK (record_count >= 0),
    PRIMARY KEY (dataset_id, version, document_id),
    UNIQUE (dataset_id, version, position),
    UNIQUE (dataset_id, version, source_sha256, page_start, page_end),
    FOREIGN KEY (dataset_id, version) REFERENCES dataset_versions(dataset_id, version) DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE jobs (
    job_id text PRIMARY KEY CHECK (job_id <> ''),
    mode text NOT NULL CHECK (mode IN ('preparation', 'training', 'preparation_and_training')),
    source_upload_id text,
    target_dataset_id text,
    base_dataset_version integer,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((target_dataset_id IS NULL) = (base_dataset_version IS NULL)),
    FOREIGN KEY (target_dataset_id, base_dataset_version) REFERENCES dataset_versions(dataset_id, version)
);

CREATE TABLE job_attempts (
    attempt_id text PRIMARY KEY CHECK (attempt_id <> ''),
    job_id text NOT NULL REFERENCES jobs(job_id),
    attempt_number integer NOT NULL CHECK (attempt_number > 0),
    previous_attempt_id text,
    temporal_namespace text NOT NULL CHECK (temporal_namespace <> ''),
    workflow_id text NOT NULL CHECK (workflow_id <> ''),
    run_id text CHECK (run_id <> ''),
    execution_status text,
    stage text,
    step text,
    source_updated_at timestamptz,
    observed_at timestamptz,
    state_stale boolean NOT NULL DEFAULT false,
    observation_revision bigint NOT NULL DEFAULT 0 CHECK (observation_revision >= 0),
    started_at timestamptz,
    closed_at timestamptz,
    result_dataset_id text,
    result_dataset_version integer,
    observation jsonb CHECK (octet_length(observation::text) <= 65536),
    error_code text CHECK (octet_length(error_code) <= 128),
    error_message text CHECK (octet_length(error_message) <= 2048),
    UNIQUE (job_id, attempt_number),
    UNIQUE (attempt_id, job_id),
    CHECK (previous_attempt_id IS DISTINCT FROM attempt_id),
    CHECK ((result_dataset_id IS NULL) = (result_dataset_version IS NULL)),
    FOREIGN KEY (previous_attempt_id, job_id) REFERENCES job_attempts(attempt_id, job_id),
    FOREIGN KEY (result_dataset_id, result_dataset_version) REFERENCES dataset_versions(dataset_id, version)
);
CREATE UNIQUE INDEX job_attempts_execution_idx ON job_attempts(temporal_namespace, workflow_id, run_id) WHERE run_id IS NOT NULL;
CREATE INDEX job_attempts_job_idx ON job_attempts(job_id, attempt_number DESC);
CREATE INDEX job_attempts_status_idx ON job_attempts(execution_status, started_at DESC, attempt_id DESC);

CREATE TABLE job_intents (
    intent_id text PRIMARY KEY CHECK (intent_id <> ''),
    job_id text NOT NULL REFERENCES jobs(job_id),
    attempt_id text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('submit', 'preview_review', 'authorize_training', 'stop', 'pause', 'resume')),
    checkpoint_id text,
    payload jsonb NOT NULL CHECK (octet_length(payload::text) <= 65536),
    payload_sha256 text NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'accepted', 'rejected', 'needs_attention')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    error_code text CHECK (octet_length(error_code) <= 128),
    CHECK (kind NOT IN ('preview_review', 'authorize_training') OR (checkpoint_id IS NOT NULL AND checkpoint_id <> '')),
    FOREIGN KEY (attempt_id, job_id) REFERENCES job_attempts(attempt_id, job_id)
);
CREATE INDEX job_intents_pending_idx ON job_intents(created_at, intent_id) WHERE state IN ('pending', 'needs_attention');
