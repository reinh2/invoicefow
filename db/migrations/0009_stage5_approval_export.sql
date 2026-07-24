-- Stage 5: explicit version approval and export records.
ALTER TABLE documents
    ADD COLUMN approved_version_id uuid REFERENCES invoice_versions(id) ON DELETE RESTRICT,
    ADD COLUMN approved_at timestamptz;

ALTER TABLE documents
    ADD CONSTRAINT documents_approval_state CHECK (
        (status IN ('approved', 'exported') AND approved_version_id IS NOT NULL AND approved_at IS NOT NULL) OR
        (status NOT IN ('approved', 'exported') AND approved_version_id IS NULL AND approved_at IS NULL)
    );

CREATE OR REPLACE FUNCTION check_document_approval_immutability() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.approved_version_id IS NOT NULL AND NEW.approved_version_id IS DISTINCT FROM OLD.approved_version_id THEN
        RAISE EXCEPTION 'approved_version_id is immutable once approved';
    END IF;
    IF NEW.approved_version_id IS NOT NULL THEN
        IF NOT EXISTS (SELECT 1 FROM invoice_versions WHERE id = NEW.approved_version_id AND document_id = NEW.id) THEN
            RAISE EXCEPTION 'approved_version_id must reference a version of the same document';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER documents_approval_immutability
BEFORE UPDATE ON documents
FOR EACH ROW EXECUTE FUNCTION check_document_approval_immutability();

CREATE TABLE exports (
    id uuid PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE RESTRICT,
    version_id uuid NOT NULL REFERENCES invoice_versions(id) ON DELETE RESTRICT,
    export_type text NOT NULL CHECK (export_type IN ('csv', 'webhook')),
    status text NOT NULL CHECK (status IN ('pending', 'succeeded', 'failed')),
    idempotency_key text NOT NULL UNIQUE CHECK (idempotency_key <> ''),
    destination text NOT NULL CHECK (destination <> ''),
    job_id uuid REFERENCES jobs(id) ON DELETE SET NULL,
    error_summary text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX exports_document_idx ON exports (document_id, created_at DESC);
