ALTER TABLE datasets ADD COLUMN deleted_at timestamptz;
ALTER TABLE dataset_versions ADD COLUMN deleted_at timestamptz;

CREATE TABLE deletion_operations (
    operation_id text PRIMARY KEY CHECK (operation_id <> ''),
    namespace text NOT NULL CHECK (namespace <> ''),
    workflow_id text NOT NULL CHECK (workflow_id <> ''),
    run_id text NOT NULL CHECK (run_id <> ''),
    dataset_id text CHECK (dataset_id <> ''),
    dataset_version integer CHECK (dataset_version > 0),
    manifest jsonb NOT NULL CHECK (jsonb_typeof(manifest) = 'object'),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','completed')),
    error_code text CHECK (error_code ~ '^[a-zA-Z0-9_.-]{1,128}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE (namespace,workflow_id,run_id),
    CHECK (dataset_version IS NULL OR dataset_id IS NOT NULL),
    CHECK ((state = 'completed') = (completed_at IS NOT NULL)),
    CHECK (state <> 'completed' OR error_code IS NULL)
);
CREATE UNIQUE INDEX deletion_operations_pending_dataset_idx ON deletion_operations(dataset_id)
    WHERE state = 'pending' AND dataset_id IS NOT NULL;
