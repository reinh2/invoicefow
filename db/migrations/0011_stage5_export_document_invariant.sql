-- Stage 5: make the export/document/version relationship database-enforced.
-- This is forward-only; the existing single-column FK remains useful for
-- direct version identity checks while this composite FK prevents a version
-- belonging to another document from being attached to an export.

ALTER TABLE invoice_versions
    ADD CONSTRAINT invoice_versions_document_id_id_key UNIQUE (document_id, id);

ALTER TABLE exports
    ADD CONSTRAINT exports_version_same_document_fk
    FOREIGN KEY (document_id, version_id)
    REFERENCES invoice_versions (document_id, id)
    ON DELETE RESTRICT;
