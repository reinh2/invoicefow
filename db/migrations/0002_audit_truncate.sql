-- Audit history is immutable, including table-wide operations.
CREATE TRIGGER audit_events_no_truncate
BEFORE TRUNCATE ON audit_events
FOR EACH STATEMENT
EXECUTE FUNCTION reject_audit_event_mutation();
