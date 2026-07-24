-- Stage 5: an export may address only the document's immutable approved
-- version. The earlier composite FK confirms that the version belongs to the
-- same document; this forward-only FK additionally binds it to approval.

ALTER TABLE documents
    ADD CONSTRAINT documents_id_approved_version_key UNIQUE (id, approved_version_id);

ALTER TABLE exports
    ADD CONSTRAINT exports_version_is_approved_fk
    FOREIGN KEY (document_id, version_id)
    REFERENCES documents (id, approved_version_id)
    ON DELETE RESTRICT;
