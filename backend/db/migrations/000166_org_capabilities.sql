-- EMP-PLAT-005. Absent rows are disabled. Organization-root references follow
-- in EMP-ORG-002, using a new migration. No activation or role grant is seeded.
CREATE TABLE org_capabilities (
    org_id UUID NOT NULL CHECK (org_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    capability TEXT NOT NULL CHECK (length(capability) BETWEEN 1 AND 64 AND capability ~ '^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)*$'),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (org_id, capability)
);
ALTER TABLE org_capabilities ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_capabilities FORCE ROW LEVEL SECURITY;
CREATE POLICY org_capabilities_scope ON org_capabilities
    USING (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid);
