-- EMP-PLAT-010: immutable event intent plus atomic consumer-effect receipt.
-- Delivery still uses the existing job_queue; no activation/job is seeded.
CREATE TABLE org_event_intents(
    org_id UUID NOT NULL,
    event_id UUID NOT NULL,
    consumer TEXT NOT NULL CHECK(length(consumer)<=64 AND consumer ~ '^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)*$'),
    event_name TEXT NOT NULL CHECK(length(event_name)<=64 AND event_name ~ '^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*){2,}$'),
    target_type TEXT NOT NULL CHECK(length(target_type)<=64 AND target_type ~ '^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)*$'),
    target_id UUID NOT NULL CHECK(target_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    revision BIGINT NOT NULL CHECK(revision>0),
    command_hash TEXT NOT NULL CHECK(command_hash ~ '^[0-9a-f]{64}$'),
    job_id UUID NOT NULL UNIQUE CHECK(job_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(org_id,event_id,consumer),
    FOREIGN KEY(org_id,event_id) REFERENCES org_audit_events(org_id,operation_id)
);
CREATE TABLE org_event_receipts(
    org_id UUID NOT NULL,
    event_id UUID NOT NULL,
    consumer TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(org_id,event_id,consumer),
    FOREIGN KEY(org_id,event_id,consumer) REFERENCES org_event_intents(org_id,event_id,consumer)
);
ALTER TABLE org_event_intents ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_event_intents FORCE ROW LEVEL SECURITY;
CREATE POLICY org_event_intents_scope ON org_event_intents
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
ALTER TABLE org_event_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_event_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY org_event_receipts_scope ON org_event_receipts
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE FUNCTION org_event_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='organization event records are append-only';
END $$;
CREATE TRIGGER org_event_intents_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_event_intents
 FOR EACH STATEMENT EXECUTE FUNCTION org_event_append_only();
CREATE TRIGGER org_event_receipts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_event_receipts
 FOR EACH STATEMENT EXECUTE FUNCTION org_event_append_only();
