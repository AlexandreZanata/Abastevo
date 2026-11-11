-- EMP-PLAT-003. Organization audit references survive account offboarding.
-- The organization-root FK is added by EMP-ORG-002 after that schema exists.
CREATE TABLE org_audit_events (
    org_id UUID NOT NULL CHECK (org_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    operation_id UUID NOT NULL CHECK (operation_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    actor_id UUID NOT NULL CHECK (actor_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    delegation_id UUID CHECK (delegation_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    action TEXT NOT NULL CHECK (length(action) BETWEEN 1 AND 64 AND action ~ '^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)*$'),
    target_type TEXT NOT NULL CHECK (length(target_type) BETWEEN 1 AND 64 AND target_type ~ '^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)*$'),
    target_id UUID NOT NULL CHECK (target_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    reason_code TEXT NOT NULL DEFAULT '' CHECK (reason_code = '' OR (length(reason_code) <= 64 AND reason_code ~ '^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)*$')),
    request_hash TEXT NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (org_id, operation_id)
);
CREATE INDEX org_audit_events_page_idx ON org_audit_events (org_id, recorded_at DESC, operation_id DESC);

ALTER TABLE org_audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_audit_events FORCE ROW LEVEL SECURITY;
CREATE POLICY org_audit_events_scope ON org_audit_events
    USING (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid);

-- Statement-level protection also blocks TRUNCATE and zero-row mutations.
-- A separately reviewed retention migration/procedure owns eventual lifecycle.
CREATE FUNCTION org_audit_events_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'organization audit events are append-only';
END $$;
CREATE TRIGGER org_audit_events_immutable
BEFORE UPDATE OR DELETE OR TRUNCATE ON org_audit_events
FOR EACH STATEMENT EXECUTE FUNCTION org_audit_events_append_only();
