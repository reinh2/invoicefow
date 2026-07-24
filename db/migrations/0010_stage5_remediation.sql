-- Stage 5 remediation: remove persisted webhook destinations and expose safe
-- export state/projection fields. This is intentionally forward-only.

ALTER TABLE exports RENAME COLUMN destination TO destination_ref;

ALTER TABLE exports
    ADD COLUMN destination_label text NOT NULL DEFAULT 'Server-configured export',
    ADD COLUMN attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    ADD COLUMN next_attempt_at timestamptz;

-- Existing 0009 rows may contain the old URL. Replace it before the new
-- opaque reference becomes the only durable representation.
UPDATE exports
SET destination_ref = CASE WHEN export_type = 'csv' THEN 'local:csv-download' ELSE 'server:webhook:v1' END,
    destination_label = CASE WHEN export_type = 'csv' THEN 'CSV download' ELSE 'Server-configured webhook' END;

ALTER TABLE exports
    DROP CONSTRAINT IF EXISTS exports_status_check;

ALTER TABLE exports
    ADD CONSTRAINT exports_status_check CHECK (status IN ('pending', 'retrying', 'succeeded', 'failed', 'dead_letter'));

ALTER TABLE exports
    ADD CONSTRAINT exports_destination_ref_check CHECK (destination_ref <> '' AND destination_ref !~* '^[a-z][a-z0-9+.-]*://');

-- Remove the old destination field from already persisted audit projections.
-- Audit history remains append-only at runtime; this one-time remediation is
-- performed inside the migration before the append-only trigger is restored.
DROP TRIGGER IF EXISTS audit_events_no_update ON audit_events;
UPDATE audit_events
SET payload = (payload - 'destination') || jsonb_build_object(
    'destination_ref', CASE WHEN payload->>'export_type' = 'csv' THEN 'local:csv-download' ELSE 'server:webhook:v1' END,
    'destination_label', CASE WHEN payload->>'export_type' = 'csv' THEN 'CSV download' ELSE 'Server-configured webhook' END
)
WHERE action = 'export_enqueued' AND payload ? 'destination';
CREATE TRIGGER audit_events_no_update BEFORE UPDATE ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_audit_event_mutation();

CREATE INDEX exports_retry_idx ON exports (next_attempt_at) WHERE status = 'retrying';
