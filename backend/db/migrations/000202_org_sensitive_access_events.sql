-- EMP-PRIV-005. Scoped sensitive-access detail of organization audit events.
-- Each row extends exactly one org_audit_events row written in the same
-- audit-runner transaction, before a capability or content leaves the API.
-- It holds identifiers and closed codes only: no content, file name, type,
-- storage key, URL, capability token or coordinate. Membership references stay
-- historical like audit actors, so there is no membership foreign key.
CREATE TABLE org_sensitive_access_events (
    org_id UUID NOT NULL CHECK (org_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    operation_id UUID NOT NULL CHECK (operation_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    category TEXT NOT NULL CHECK (category IN ('DOCUMENT_CONTENT', 'EXPORT_FILE', 'LOCATION_LIVE', 'LOCATION_TRACK')),
    stage TEXT NOT NULL CHECK (stage IN ('ISSUED', 'SERVED')),
    membership_id UUID NOT NULL CHECK (membership_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    access_id UUID NOT NULL CHECK (access_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    PRIMARY KEY (org_id, operation_id),
    FOREIGN KEY (org_id) REFERENCES organizations (org_id),
    FOREIGN KEY (org_id, operation_id) REFERENCES org_audit_events (org_id, operation_id),
    -- A live position has no capability; a capability read is not its own grant.
    CHECK (category <> 'LOCATION_LIVE' OR (stage = 'SERVED' AND access_id = operation_id)),
    CHECK (stage <> 'SERVED' OR category = 'LOCATION_LIVE' OR access_id <> operation_id)
);
-- One issuance per capability; a served row finds its grant through it.
CREATE UNIQUE INDEX org_sensitive_access_grant_key
    ON org_sensitive_access_events (org_id, access_id) WHERE stage = 'ISSUED';

ALTER TABLE org_sensitive_access_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_sensitive_access_events FORCE ROW LEVEL SECURITY;
CREATE POLICY org_sensitive_access_events_scope ON org_sensitive_access_events
    USING (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid);
REVOKE ALL ON org_sensitive_access_events FROM PUBLIC;

-- Records are part of the append-only audit trail.
CREATE TRIGGER org_sensitive_access_events_immutable
BEFORE UPDATE OR DELETE OR TRUNCATE ON org_sensitive_access_events
FOR EACH STATEMENT EXECUTE FUNCTION org_audit_events_append_only();

-- A detail matches its event's registered action, target type and member
-- reason; a capability read references a member-bound issuance of the same
-- category, target and actor in this organization.
CREATE FUNCTION org_sensitive_access_guard() RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
    event_action TEXT;
    event_target_type TEXT;
    event_target UUID;
    event_actor UUID;
    event_reason TEXT;
    expected TEXT;
    expected_type TEXT;
BEGIN
    IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id', true), '')::uuid THEN
        RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = 'organization scope denied';
    END IF;
    SELECT action, target_type, target_id, actor_id, reason_code
        INTO event_action, event_target_type, event_target, event_actor, event_reason
        FROM public.org_audit_events WHERE org_id = NEW.org_id AND operation_id = NEW.operation_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = '23503', MESSAGE = 'sensitive access event not found';
    END IF;
    expected := CASE NEW.category || '/' || NEW.stage
        WHEN 'DOCUMENT_CONTENT/ISSUED' THEN 'document.access.issued'
        WHEN 'DOCUMENT_CONTENT/SERVED' THEN 'document.content.accessed'
        WHEN 'EXPORT_FILE/ISSUED' THEN 'export.access.issued'
        WHEN 'EXPORT_FILE/SERVED' THEN 'export.content.accessed'
        WHEN 'LOCATION_TRACK/ISSUED' THEN 'location.track.issued'
        WHEN 'LOCATION_TRACK/SERVED' THEN 'location.track.accessed'
        WHEN 'LOCATION_LIVE/SERVED' THEN 'location.live.accessed'
    END;
    expected_type := CASE NEW.category WHEN 'DOCUMENT_CONTENT' THEN 'document' WHEN 'EXPORT_FILE' THEN 'export' ELSE 'trip' END;
    IF expected IS NULL OR event_action IS DISTINCT FROM expected OR event_reason <> 'member.authorized'
        OR event_target_type IS DISTINCT FROM expected_type
        OR (NEW.category = 'LOCATION_LIVE' AND NEW.access_id <> NEW.operation_id)
        OR (NEW.stage = 'SERVED' AND NEW.category <> 'LOCATION_LIVE' AND NEW.access_id = NEW.operation_id) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'sensitive access does not match its event';
    END IF;
    IF NEW.stage = 'SERVED' AND NEW.category <> 'LOCATION_LIVE' AND NOT EXISTS (
        SELECT 1 FROM public.org_sensitive_access_events g
        JOIN public.org_audit_events e ON e.org_id = g.org_id AND e.operation_id = g.operation_id
        WHERE g.org_id = NEW.org_id AND g.access_id = NEW.access_id AND g.stage = 'ISSUED'
          AND g.category = NEW.category AND g.membership_id = NEW.membership_id
          AND e.target_id = event_target AND e.actor_id = event_actor) THEN
        RAISE EXCEPTION USING ERRCODE = '23503', MESSAGE = 'sensitive access grant not found';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_sensitive_access_events_guard
BEFORE INSERT ON org_sensitive_access_events
FOR EACH ROW EXECUTE FUNCTION org_sensitive_access_guard();

-- A sensitive audit event never commits without its scoped detail.
CREATE FUNCTION org_sensitive_access_complete() RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM public.org_sensitive_access_events
        WHERE org_id = NEW.org_id AND operation_id = NEW.operation_id) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'sensitive access detail missing';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_sensitive_access_events_complete
AFTER INSERT ON org_audit_events DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW WHEN (NEW.action IN ('document.access.issued', 'document.content.accessed', 'export.access.issued',
    'export.content.accessed', 'location.track.issued', 'location.track.accessed', 'location.live.accessed'))
EXECUTE FUNCTION org_sensitive_access_complete();
