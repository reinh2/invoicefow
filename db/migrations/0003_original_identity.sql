-- Original bytes are identified once and cannot be retargeted later.
ALTER TABLE stored_objects
    ADD CONSTRAINT stored_objects_sha256_key UNIQUE (sha256);

CREATE OR REPLACE FUNCTION reject_stored_object_identity_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.storage_key IS DISTINCT FROM OLD.storage_key
       OR NEW.sha256 IS DISTINCT FROM OLD.sha256 THEN
        RAISE EXCEPTION 'stored object identity is immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER stored_objects_identity_immutable
BEFORE UPDATE ON stored_objects
FOR EACH ROW
EXECUTE FUNCTION reject_stored_object_identity_mutation();

CREATE OR REPLACE FUNCTION enforce_document_object_hash_match() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    object_hash bytea;
BEGIN
    IF NEW.object_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT sha256 INTO object_hash FROM stored_objects WHERE id = NEW.object_id;
    IF object_hash IS NULL OR object_hash IS DISTINCT FROM NEW.sha256 THEN
        RAISE EXCEPTION 'document hash must match its stored object hash';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER documents_object_hash_matches
BEFORE INSERT OR UPDATE OF object_id, sha256 ON documents
FOR EACH ROW
EXECUTE FUNCTION enforce_document_object_hash_match();
