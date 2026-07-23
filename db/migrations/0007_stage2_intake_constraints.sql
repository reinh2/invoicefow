-- Stage 2 accepts only non-empty PDF/JPEG/PNG originals. The API validates
-- signatures before this boundary; these constraints make that accepted shape
-- explicit for every writer.
ALTER TABLE stored_objects
    ADD CONSTRAINT stored_objects_size_positive CHECK (size_bytes > 0),
    ADD CONSTRAINT stored_objects_media_type_supported CHECK (
        media_type IN ('application/pdf', 'image/jpeg', 'image/png')
    );

CREATE INDEX jobs_process_document_ready_idx
    ON jobs (next_attempt_at, created_at)
    WHERE status = 'ready' AND job_type = 'process_document';
