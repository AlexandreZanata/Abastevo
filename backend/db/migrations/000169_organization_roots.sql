-- EMP-ORG-002: permanent private ownership roots. No runtime role or business
-- activation is seeded. has_snapshot=false is migration metadata, not a
-- PENDING_SETUP/ACTIVE organization or proof of membership/consent.
CREATE FUNCTION organization_kind_tags_valid(tags TEXT[]) RETURNS BOOLEAN
LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT tags IS NOT NULL
  AND (cardinality(tags)=0 OR array_ndims(tags)=1)
  AND cardinality(tags)<=5
  AND tags <@ ARRAY['CARRIER','DELIVERY','FARM','OTHER','SERVICES']::text[]
  AND NOT EXISTS(SELECT 1 FROM unnest(tags) AS item WHERE item IS NULL)
  AND tags=ARRAY(SELECT DISTINCT item COLLATE "C" FROM unnest(tags) AS item ORDER BY item COLLATE "C");
$$;

CREATE TABLE organizations(
 org_id UUID PRIMARY KEY CHECK(org_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 has_snapshot BOOLEAN NOT NULL DEFAULT FALSE,
 display_name TEXT,
 time_zone TEXT,
 kind_tags TEXT[],
 state TEXT,
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 suspension_reason TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CONSTRAINT organizations_snapshot_shape CHECK(
  (NOT has_snapshot AND display_name IS NULL AND time_zone IS NULL AND kind_tags IS NULL
   AND state IS NULL AND suspension_reason IS NULL)
  OR
  (has_snapshot AND display_name IS NOT NULL AND time_zone IS NOT NULL AND kind_tags IS NOT NULL
   AND state IS NOT NULL AND suspension_reason IS NOT NULL
   AND char_length(display_name)<=120 AND display_name !~ '[[:cntrl:]]'
   AND display_name !~ U&'[\202a-\202e\2066-\2069]'
   AND char_length(time_zone)<=128
   AND public.organization_kind_tags_valid(kind_tags)
   AND state IN ('PENDING_SETUP','ACTIVE','SUSPENDED','ARCHIVED','CLOSED')
   AND suspension_reason IN ('','BILLING','TERMS','SECURITY','LEGAL','OWNER_REQUEST')
   AND (state NOT IN ('PENDING_SETUP','ACTIVE') OR suspension_reason='')
   AND (state<>'SUSPENDED' OR suspension_reason<>'')
   AND (state IN ('PENDING_SETUP','CLOSED') OR (btrim(display_name)<>'' AND time_zone<>''))
  )
 )
);
REVOKE ALL ON organizations FROM PUBLIC;

-- SQLC's organization schema owns only this migration. This static migration
-- block resolves the pre-existing foundation tables at execution time.
-- Strong locks and NO FORCE exist only inside this atomic migration; no
-- concurrent connection observes a relaxed policy. Restore FORCE before commit.
DO $$
BEGIN
 ALTER TABLE public.org_audit_events NO FORCE ROW LEVEL SECURITY;
 ALTER TABLE public.org_capabilities NO FORCE ROW LEVEL SECURITY;
 INSERT INTO public.organizations(org_id)
 SELECT org_id FROM public.org_audit_events UNION SELECT org_id FROM public.org_capabilities;
 ALTER TABLE public.org_audit_events FORCE ROW LEVEL SECURITY;
 ALTER TABLE public.org_capabilities FORCE ROW LEVEL SECURITY;

 ALTER TABLE public.org_audit_events ADD CONSTRAINT org_audit_events_root_fk
  FOREIGN KEY(org_id) REFERENCES public.organizations(org_id) DEFERRABLE INITIALLY DEFERRED;
 ALTER TABLE public.org_capabilities ADD CONSTRAINT org_capabilities_root_fk
  FOREIGN KEY(org_id) REFERENCES public.organizations(org_id) DEFERRABLE INITIALLY DEFERRED;
 ALTER TABLE public.org_event_intents ADD CONSTRAINT org_event_intents_root_fk
  FOREIGN KEY(org_id) REFERENCES public.organizations(org_id) DEFERRABLE INITIALLY DEFERRED;
 ALTER TABLE public.org_event_receipts ADD CONSTRAINT org_event_receipts_root_fk
  FOREIGN KEY(org_id) REFERENCES public.organizations(org_id) DEFERRABLE INITIALLY DEFERRED;
END $$;

ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE ROW LEVEL SECURITY;
CREATE POLICY organizations_scope ON organizations
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);

CREATE FUNCTION organizations_keep_identity() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='organization identities are permanent';
END $$;
CREATE TRIGGER organizations_no_delete BEFORE DELETE OR TRUNCATE ON organizations
 FOR EACH STATEMENT EXECUTE FUNCTION organizations_keep_identity();

CREATE FUNCTION organizations_guard_insert() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.revision<>1 OR (NEW.has_snapshot AND NEW.state<>'PENDING_SETUP') THEN
  RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='new organizations start in pending setup';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER organizations_initial BEFORE INSERT ON organizations
 FOR EACH ROW EXECUTE FUNCTION organizations_guard_insert();

CREATE FUNCTION organizations_guard_update() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id<>OLD.org_id OR NOT NEW.has_snapshot OR
    OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='invalid organization revision or identity';
 END IF;
 IF OLD.state='CLOSED' THEN
  RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='organization is closed';
 END IF;
 IF NOT OLD.has_snapshot THEN
  IF NEW.state<>'PENDING_SETUP' THEN
   RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='legacy organization metadata must be reconciled';
  END IF;
 ELSIF NEW.state<>OLD.state AND NOT(
   (OLD.state='PENDING_SETUP' AND NEW.state IN ('ACTIVE','CLOSED')) OR
   (OLD.state='ACTIVE' AND NEW.state IN ('SUSPENDED','ARCHIVED')) OR
   (OLD.state='SUSPENDED' AND NEW.state IN ('ACTIVE','ARCHIVED')) OR
   (OLD.state='ARCHIVED' AND NEW.state IN ('ACTIVE','SUSPENDED','CLOSED'))
 ) THEN
  RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='organization transition is not allowed';
 END IF;
 NEW.created_at=OLD.created_at;
 NEW.updated_at=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER organizations_revision BEFORE UPDATE ON organizations
 FOR EACH ROW EXECUTE FUNCTION organizations_guard_update();
