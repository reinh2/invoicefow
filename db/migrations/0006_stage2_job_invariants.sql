ALTER TABLE jobs ADD CONSTRAINT jobs_attempts_bounded CHECK (attempts <= max_attempts);
ALTER TABLE jobs ADD CONSTRAINT jobs_finished_lease_empty CHECK (
    status = 'running' OR (lease_token IS NULL AND leased_until IS NULL)
);
CREATE INDEX documents_audit_sequence_idx ON documents (id, audit_sequence);
