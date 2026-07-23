-- Stage 3 extraction snapshots retain bounded raw candidates and their
-- server-normalized representation. A partial proposal may legitimately lack
-- currency/total, so currency is nullable for extraction snapshots.
ALTER TABLE invoice_versions ALTER COLUMN currency DROP NOT NULL;
ALTER TABLE invoice_versions
    ADD COLUMN proposal jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN normalized jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN warnings jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN evidence jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN diagnostics jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN rounding_policy_version text NOT NULL DEFAULT 'money-v1';

ALTER TABLE invoice_versions
    ADD CONSTRAINT invoice_versions_extraction_snapshot_shape CHECK (
        jsonb_typeof(proposal) = 'object' AND
        jsonb_typeof(normalized) = 'object' AND
        jsonb_typeof(warnings) = 'array' AND
        jsonb_typeof(evidence) = 'array' AND
        jsonb_typeof(diagnostics) = 'array' AND
        rounding_policy_version <> ''
    );
