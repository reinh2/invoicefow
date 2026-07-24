-- Stage 7: bind the durable export projection to exactly one approved version
-- and one matching export job. These constraints defend the idempotency and
-- worker assumptions even when a future writer bypasses repository helpers.

ALTER TABLE exports
    ADD CONSTRAINT exports_document_version_type_unique UNIQUE (document_id, version_id, export_type);

ALTER TABLE jobs
    ADD CONSTRAINT jobs_id_document_id_key UNIQUE (id, document_id),
    ADD CONSTRAINT jobs_terminal_schedule_check CHECK (
        status NOT IN ('succeeded', 'dead_letter') OR next_attempt_at IS NULL
    );

ALTER TABLE exports
    DROP CONSTRAINT exports_job_id_fkey,
    ADD CONSTRAINT exports_job_id_unique UNIQUE (job_id),
    ADD CONSTRAINT exports_job_same_document_fk
        FOREIGN KEY (job_id, document_id)
        REFERENCES jobs (id, document_id)
        ON DELETE RESTRICT,
    ADD CONSTRAINT exports_terminal_schedule_check CHECK (
        status NOT IN ('succeeded', 'failed', 'dead_letter') OR next_attempt_at IS NULL
    ),
    ADD CONSTRAINT exports_retry_schedule_check CHECK (
        status <> 'retrying' OR next_attempt_at IS NOT NULL
    );

CREATE OR REPLACE FUNCTION check_export_job_type() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.job_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM jobs WHERE id = NEW.job_id AND document_id = NEW.document_id AND job_type = 'export_document'
    ) THEN
        RAISE EXCEPTION 'export job must be an export_document job for the same document';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER exports_job_type_guard
BEFORE INSERT OR UPDATE OF job_id, document_id ON exports
FOR EACH ROW EXECUTE FUNCTION check_export_job_type();

CREATE INDEX jobs_export_document_ready_idx
    ON jobs (next_attempt_at, created_at)
    WHERE status = 'ready' AND job_type = 'export_document';
