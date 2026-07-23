-- Extraction and human-review versions are immutable snapshots.
CREATE OR REPLACE FUNCTION reject_invoice_version_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'invoice versions are append-only';
END;
$$;

CREATE TRIGGER invoice_versions_no_update
BEFORE UPDATE ON invoice_versions
FOR EACH ROW
EXECUTE FUNCTION reject_invoice_version_mutation();

CREATE TRIGGER invoice_versions_no_delete
BEFORE DELETE ON invoice_versions
FOR EACH ROW
EXECUTE FUNCTION reject_invoice_version_mutation();
