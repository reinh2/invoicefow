CREATE TABLE stored_objects (
    id uuid PRIMARY KEY,
    storage_key text NOT NULL UNIQUE CHECK (storage_key <> ''),
    sha256 bytea NOT NULL CHECK (octet_length(sha256) = 32),
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    media_type text NOT NULL CHECK (media_type <> ''),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE documents (
    id uuid PRIMARY KEY,
    object_id uuid REFERENCES stored_objects(id) ON DELETE RESTRICT,
    sha256 bytea NOT NULL CHECK (octet_length(sha256) = 32),
    status text NOT NULL CHECK (status IN ('uploaded', 'queued', 'processing', 'needs_review', 'approved', 'rejected', 'exported', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((object_id IS NULL AND status = 'failed') OR object_id IS NOT NULL),
    UNIQUE (sha256)
);

CREATE TABLE jobs (
    id uuid PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE RESTRICT,
    job_type text NOT NULL CHECK (job_type IN ('process_document', 'export_document')),
    status text NOT NULL CHECK (status IN ('ready', 'running', 'succeeded', 'dead_letter')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_token uuid,
    leased_until timestamptz,
    idempotency_key text NOT NULL UNIQUE CHECK (idempotency_key <> ''),
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'running') = (lease_token IS NOT NULL AND leased_until IS NOT NULL))
);
CREATE INDEX jobs_ready_idx ON jobs (next_attempt_at, created_at) WHERE status = 'ready';

CREATE TABLE job_attempts (
    id uuid PRIMARY KEY,
    job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
    attempt_number integer NOT NULL CHECK (attempt_number > 0),
    lease_token uuid NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    outcome text CHECK (outcome IN ('succeeded', 'retryable_failure', 'permanent_failure')),
    error_summary text,
    UNIQUE (job_id, attempt_number)
);

CREATE TABLE invoice_versions (
    id uuid PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE RESTRICT,
    version_number integer NOT NULL CHECK (version_number > 0),
    currency char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    total_minor bigint,
    source text NOT NULL CHECK (source IN ('extraction', 'human_review')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (total_minor IS NULL OR currency IS NOT NULL),
    UNIQUE (document_id, version_number)
);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE RESTRICT,
    sequence_number bigint NOT NULL CHECK (sequence_number > 0),
    action text NOT NULL CHECK (action <> ''),
    actor text NOT NULL CHECK (actor <> ''),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (document_id, sequence_number)
);

CREATE OR REPLACE FUNCTION reject_audit_event_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit events are append-only';
END;
$$;
CREATE TRIGGER audit_events_no_update BEFORE UPDATE ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_audit_event_mutation();
CREATE TRIGGER audit_events_no_delete BEFORE DELETE ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_audit_event_mutation();
