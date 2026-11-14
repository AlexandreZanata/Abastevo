-- EMP-NOTIF-001: scoped in-app notification core (B-BR-EMP-215..218, EDR-150).
-- Additive and forced-RLS. Producers register immutable reference-only
-- requests inside their audited orgevent transaction; the notif.dispatch
-- consumer claims each request once. IN_APP is the only channel; no provider,
-- kind, job, preference or runtime grant is seeded here.
CREATE TABLE org_notification_requests(
    org_id UUID NOT NULL REFERENCES organizations(org_id),
    id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
    idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 128 AND idempotency_key ~ '^[a-z0-9][a-z0-9._:-]*$'),
    source_event_id UUID NOT NULL,
    source_event_name TEXT NOT NULL CHECK(length(source_event_name)<=64 AND source_event_name ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){2,}$'),
    source_target_type TEXT NOT NULL CHECK(length(source_target_type)<=64 AND source_target_type ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$'),
    source_target_id UUID NOT NULL CHECK(source_target_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    kind TEXT NOT NULL CHECK(length(kind)<=64 AND kind ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
    template TEXT NOT NULL CHECK(length(template)<=64 AND template ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
    params JSONB NOT NULL CHECK(jsonb_typeof(params)='object' AND octet_length(params::text)<=2048),
    link_path TEXT CHECK(link_path IS NULL OR (length(link_path) BETWEEN 2 AND 256 AND link_path ~ '^(/[a-z0-9][a-z0-9-]*)+$')),
    content_hash TEXT NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
    recipient_count INTEGER NOT NULL CHECK(recipient_count BETWEEN 1 AND 50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    dispatched_at TIMESTAMPTZ,
    PRIMARY KEY(org_id,id),
    UNIQUE(org_id,idempotency_key),
    FOREIGN KEY(org_id,source_event_id) REFERENCES org_audit_events(org_id,operation_id),
    CHECK(dispatched_at IS NULL OR dispatched_at>=created_at)
);
CREATE INDEX org_notification_requests_event ON org_notification_requests(org_id,source_event_id);

CREATE TABLE org_notification_recipients(
    org_id UUID NOT NULL,
    request_id UUID NOT NULL,
    membership_id UUID NOT NULL,
    PRIMARY KEY(org_id,request_id,membership_id),
    FOREIGN KEY(org_id,request_id) REFERENCES org_notification_requests(org_id,id),
    FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id)
);

CREATE TABLE org_notification_notices(
    org_id UUID NOT NULL,
    id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
    position BIGINT GENERATED ALWAYS AS IDENTITY,
    request_id UUID NOT NULL,
    membership_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    read_at TIMESTAMPTZ,
    PRIMARY KEY(org_id,id),
    UNIQUE(position),
    UNIQUE(org_id,request_id,membership_id),
    FOREIGN KEY(org_id,request_id,membership_id) REFERENCES org_notification_recipients(org_id,request_id,membership_id),
    CHECK(read_at IS NULL OR read_at>=created_at)
);
CREATE INDEX org_notification_notices_inbox ON org_notification_notices(org_id,membership_id,position DESC);

CREATE TABLE org_notification_deliveries(
    org_id UUID NOT NULL,
    request_id UUID NOT NULL,
    membership_id UUID NOT NULL,
    channel TEXT NOT NULL CHECK(channel='IN_APP'),
    outcome TEXT NOT NULL CHECK(outcome IN ('DELIVERED','SUPPRESSED')),
    reason TEXT CHECK(reason IN ('INELIGIBLE','MUTED','LIMITED')),
    notice_id UUID,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(org_id,request_id,membership_id,channel),
    FOREIGN KEY(org_id,request_id,membership_id) REFERENCES org_notification_recipients(org_id,request_id,membership_id),
    FOREIGN KEY(org_id,notice_id) REFERENCES org_notification_notices(org_id,id),
    CHECK((outcome='DELIVERED')=(notice_id IS NOT NULL)),
    CHECK((outcome='DELIVERED')=(reason IS NULL))
);

CREATE TABLE org_notification_preferences(
    org_id UUID NOT NULL,
    membership_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK(length(kind)<=64 AND kind ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
    muted BOOLEAN NOT NULL,
    revision BIGINT NOT NULL CHECK(revision>0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(org_id,membership_id,kind),
    FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id)
);

CREATE TABLE org_notification_budgets(
    org_id UUID NOT NULL,
    membership_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK(length(kind)<=64 AND kind ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
    window_start TIMESTAMPTZ NOT NULL CHECK(window_start=date_trunc('hour',window_start,'UTC')),
    used INTEGER NOT NULL CHECK(used BETWEEN 1 AND 20),
    PRIMARY KEY(org_id,membership_id,kind,window_start),
    FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id)
);

CREATE TABLE org_notification_commands(
    org_id UUID NOT NULL,
    operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    membership_id UUID NOT NULL,
    action TEXT NOT NULL CHECK(action IN ('notice.read','preference.set')),
    target TEXT NOT NULL CHECK(length(target) BETWEEN 1 AND 64),
    request_hash TEXT NOT NULL CHECK(request_hash ~ '^[0-9a-f]{64}$'),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(org_id,operation_id),
    FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id)
);

ALTER TABLE org_notification_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_notification_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY org_notification_requests_scope ON org_notification_requests
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
ALTER TABLE org_notification_recipients ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_notification_recipients FORCE ROW LEVEL SECURITY;
CREATE POLICY org_notification_recipients_scope ON org_notification_recipients
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
ALTER TABLE org_notification_notices ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_notification_notices FORCE ROW LEVEL SECURITY;
CREATE POLICY org_notification_notices_scope ON org_notification_notices
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
ALTER TABLE org_notification_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_notification_deliveries FORCE ROW LEVEL SECURITY;
CREATE POLICY org_notification_deliveries_scope ON org_notification_deliveries
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
ALTER TABLE org_notification_preferences ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_notification_preferences FORCE ROW LEVEL SECURITY;
CREATE POLICY org_notification_preferences_scope ON org_notification_preferences
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
ALTER TABLE org_notification_budgets ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_notification_budgets FORCE ROW LEVEL SECURITY;
CREATE POLICY org_notification_budgets_scope ON org_notification_budgets
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
ALTER TABLE org_notification_commands ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_notification_commands FORCE ROW LEVEL SECURITY;
CREATE POLICY org_notification_commands_scope ON org_notification_commands
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);

-- History, recipients and member receipts are append-only.
CREATE FUNCTION org_notification_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='organization notification records are append-only';
END $$;
CREATE TRIGGER org_notification_recipients_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_notification_recipients
 FOR EACH STATEMENT EXECUTE FUNCTION org_notification_append_only();
CREATE TRIGGER org_notification_deliveries_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_notification_deliveries
 FOR EACH STATEMENT EXECUTE FUNCTION org_notification_append_only();
CREATE TRIGGER org_notification_commands_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_notification_commands
 FOR EACH STATEMENT EXECUTE FUNCTION org_notification_append_only();
CREATE TRIGGER org_notification_requests_retained BEFORE DELETE OR TRUNCATE ON org_notification_requests
 FOR EACH STATEMENT EXECUTE FUNCTION org_notification_append_only();
CREATE TRIGGER org_notification_notices_retained BEFORE DELETE OR TRUNCATE ON org_notification_notices
 FOR EACH STATEMENT EXECUTE FUNCTION org_notification_append_only();
CREATE TRIGGER org_notification_preferences_retained BEFORE DELETE OR TRUNCATE ON org_notification_preferences
 FOR EACH STATEMENT EXECUTE FUNCTION org_notification_append_only();

-- A request changes only by its single PENDING -> dispatched claim.
CREATE FUNCTION org_notification_request_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.dispatched_at IS NOT NULL OR NEW.dispatched_at IS NULL
  OR (to_jsonb(NEW)-'dispatched_at') IS DISTINCT FROM (to_jsonb(OLD)-'dispatched_at') THEN
  RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='notification requests are dispatched once and otherwise immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_notification_requests_guard BEFORE UPDATE ON org_notification_requests
 FOR EACH ROW EXECUTE FUNCTION org_notification_request_guard();

-- A notice changes only by becoming read once.
CREATE FUNCTION org_notification_notice_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.read_at IS NOT NULL OR NEW.read_at IS NULL
  OR (to_jsonb(NEW)-'read_at') IS DISTINCT FROM (to_jsonb(OLD)-'read_at') THEN
  RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='notices only become read once';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_notification_notices_guard BEFORE UPDATE ON org_notification_notices
 FOR EACH ROW EXECUTE FUNCTION org_notification_notice_guard();

-- Preferences start at revision 1 and advance by exactly one per change.
CREATE FUNCTION org_notification_preference_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  IF NEW.revision<>1 THEN
   RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='preferences start at revision 1';
  END IF;
 ELSIF NEW.org_id IS DISTINCT FROM OLD.org_id OR NEW.membership_id IS DISTINCT FROM OLD.membership_id
  OR NEW.kind IS DISTINCT FROM OLD.kind OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='preference identity is immutable and revisions advance by one';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_notification_preferences_guard BEFORE INSERT OR UPDATE ON org_notification_preferences
 FOR EACH ROW EXECUTE FUNCTION org_notification_preference_guard();

-- Budget windows only count up within their fixed bound.
CREATE FUNCTION org_notification_budget_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM OLD.org_id OR NEW.membership_id IS DISTINCT FROM OLD.membership_id
  OR NEW.kind IS DISTINCT FROM OLD.kind OR NEW.window_start IS DISTINCT FROM OLD.window_start OR NEW.used<>OLD.used+1 THEN
  RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='notification budgets only count up';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_notification_budgets_guard BEFORE UPDATE ON org_notification_budgets
 FOR EACH ROW EXECUTE FUNCTION org_notification_budget_guard();

-- Every committed request carries exactly its declared recipients.
CREATE FUNCTION org_notification_request_complete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (SELECT count(*) FROM org_notification_recipients r WHERE r.org_id=NEW.org_id AND r.request_id=NEW.id)<>NEW.recipient_count THEN
  RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='notification request recipients are incomplete';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_notification_requests_complete AFTER INSERT ON org_notification_requests
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_notification_request_complete();
