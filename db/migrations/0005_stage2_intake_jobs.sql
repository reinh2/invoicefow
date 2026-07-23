ALTER TABLE documents ADD COLUMN audit_sequence bigint NOT NULL DEFAULT 0 CHECK (audit_sequence >= 0);

CREATE UNIQUE INDEX jobs_one_process_per_document
    ON jobs (document_id) WHERE job_type = 'process_document';
CREATE INDEX jobs_lease_recovery_idx ON jobs (leased_until) WHERE status = 'running';
CREATE UNIQUE INDEX job_attempts_one_open_per_job ON job_attempts (job_id) WHERE finished_at IS NULL;

ALTER TABLE job_attempts ADD CONSTRAINT job_attempts_finish_consistency CHECK (
    (finished_at IS NULL AND outcome IS NULL AND error_summary IS NULL) OR
    (finished_at IS NOT NULL AND outcome IS NOT NULL)
);
