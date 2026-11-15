-- EMP-OFFL-012. Organization-state and plan disposition of queued operations
-- (EDR-200/201): one immutable, server-derived decision per state
-- acknowledgement. No grants, no backfill, no applied migration edited.

-- SQL mirror of ORG-009 AssessSuspension for a fact with no server-opened
-- entity: closed periods wholly before the declaration are skipped; a period
-- constrains unless the fact was declared before it and received inside its
-- 7-day grace (end instant included), or was declared during a resolved
-- interval. Elapsed seconds, never calendar days (EDR-119).
CREATE FUNCTION org_sync_suspension_disposition_of(started TIMESTAMPTZ[],ended TIMESTAMPTZ[],declared TIMESTAMPTZ,received TIMESTAMPTZ)
 RETURNS TEXT LANGUAGE plpgsql STABLE SET search_path=pg_catalog AS $$
DECLARE i INTEGER;result TEXT:='ALLOW';in_grace BOOLEAN;during BOOLEAN;
BEGIN
 IF declared IS NULL OR received IS NULL OR started IS NULL OR ended IS NULL OR cardinality(started)<>cardinality(ended) THEN RETURN NULL;END IF;
 FOR i IN 1..cardinality(started) LOOP
  IF started[i] IS NULL OR started[i]>received OR(ended[i] IS NOT NULL AND(ended[i]<started[i] OR ended[i]>received)) THEN RETURN NULL;END IF;
  CONTINUE WHEN ended[i] IS NOT NULL AND declared>=ended[i];
  in_grace:=declared<started[i] AND received<=started[i]+make_interval(secs=>604800);
  during:=ended[i] IS NOT NULL AND declared>=started[i] AND declared<ended[i];
  IF NOT in_grace AND NOT during THEN RETURN 'REFUSE';END IF;
  result:='REVIEW';
 END LOOP;
 RETURN result;
END $$;
CREATE FUNCTION org_sync_suspension_disposition(org UUID,declared TIMESTAMPTZ,received TIMESTAMPTZ)
 RETURNS TEXT LANGUAGE sql STABLE SET search_path=pg_catalog AS $$
 SELECT public.org_sync_suspension_disposition_of(COALESCE(array_agg(p.started_at ORDER BY p.started_at,p.id),'{}'),COALESCE(array_agg(p.ended_at ORDER BY p.started_at,p.id),'{}'),declared,received)
 FROM public.org_suspension_periods p WHERE p.org_id=org AND p.started_at<=received
$$;

-- EDR-201: the plan rule over the capability row, shared with Go DecidePlan
-- through contracts/testdata/enterprise/sync/state-intake-v1.psv.
CREATE FUNCTION org_sync_plan_disposition(state TEXT,revision BIGINT,changed TIMESTAMPTZ,declared TIMESTAMPTZ,received TIMESTAMPTZ)
 RETURNS TEXT LANGUAGE sql IMMUTABLE SET search_path=pg_catalog AS $$
 SELECT CASE
  WHEN declared IS NULL OR received IS NULL OR state IS NULL OR revision IS NULL THEN NULL
  WHEN state='ABSENT' THEN CASE WHEN revision=0 AND changed IS NULL THEN 'DENY' END
  WHEN state NOT IN('ENABLED','DISABLED') OR revision<1 OR changed IS NULL OR changed>received THEN NULL
  WHEN state='ENABLED' THEN 'APPLY'
  WHEN revision>1 AND declared<changed THEN 'HOLD'
  ELSE 'DENY' END
$$;

CREATE TABLE org_sync_state_intakes(
 org_id UUID NOT NULL,operation_id UUID NOT NULL,
 condition TEXT NOT NULL CHECK(condition IN('SUSPENSION','PLAN')),
 disposition TEXT NOT NULL CHECK(disposition IN('HOLD','REFUSE')),
 capability TEXT,capability_revision BIGINT,plan_changed_at TIMESTAMPTZ,
 declared_at TIMESTAMPTZ NOT NULL CHECK(isfinite(declared_at)),declared_submicro SMALLINT NOT NULL CHECK(declared_submicro BETWEEN 0 AND 999),
 received_at TIMESTAMPTZ NOT NULL CHECK(isfinite(received_at) AND received_at>'0001-01-01 00:00:00+00'::timestamptz AND received_at<'10000-01-01 00:00:00+00'::timestamptz),
 result TEXT NOT NULL,code TEXT NOT NULL,
 policy TEXT NOT NULL DEFAULT 'state-intake-v1' CHECK(policy='state-intake-v1'),
 PRIMARY KEY(org_id,operation_id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 FOREIGN KEY(org_id,capability) REFERENCES org_capabilities(org_id,capability),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_acknowledgements(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((condition='SUSPENSION' AND capability IS NULL AND capability_revision IS NULL AND plan_changed_at IS NULL
   AND((disposition='HOLD' AND((result='ACCEPTED_FOR_REVIEW' AND code='ORG_SUSPENDED_GRACE') OR(result='REJECTED' AND code='MALFORMED')))
    OR(disposition='REFUSE' AND result='REJECTED' AND code='ORG_SUSPENDED')))
  OR(condition='PLAN' AND capability IS NOT NULL AND capability_revision>1 AND plan_changed_at IS NOT NULL AND disposition='HOLD'
   AND((result='ACCEPTED_FOR_REVIEW' AND code='PLAN_CHANGED') OR(result='REJECTED' AND code='MALFORMED'))))
);
ALTER TABLE org_sync_state_intakes ENABLE ROW LEVEL SECURITY;ALTER TABLE org_sync_state_intakes FORCE ROW LEVEL SECURITY;
CREATE POLICY org_sync_state_intakes_scope ON org_sync_state_intakes USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_sync_state_intakes_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_sync_state_intakes FOR EACH STATEMENT EXECUTE FUNCTION org_sync_keep_history();
REVOKE ALL ON org_sync_state_intakes FROM PUBLIC;

-- The database re-derives the condition from history at the receipt instant.
CREATE FUNCTION org_sync_state_intake_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE o public.org_sync_operations%ROWTYPE;suspension TEXT;c public.org_capabilities%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 -- Serialize with operator suspension changes and capability changes.
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO o FROM public.org_sync_operations WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_device_enrollments e JOIN public.org_memberships p ON(p.org_id,p.id)=(e.org_id,e.membership_id)
  WHERE e.org_id=o.org_id AND e.id=o.enrollment_id AND e.membership_id=o.membership_id AND e.state='ENROLLED' AND p.state='ACTIVE' AND p.account_id=o.owner_account_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='state intake source unavailable';END IF;
 IF NEW.received_at>clock_timestamp() THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='state intake receipt unavailable';END IF;
 suspension=public.org_sync_suspension_disposition(NEW.org_id,NEW.declared_at,NEW.received_at);
 IF NEW.condition='SUSPENSION' THEN
  IF suspension IS DISTINCT FROM (CASE NEW.disposition WHEN 'HOLD' THEN 'REVIEW' ELSE 'REFUSE' END) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='state intake decision unavailable';END IF;
 ELSE
  SELECT * INTO c FROM public.org_capabilities WHERE org_id=NEW.org_id AND capability=NEW.capability;
  IF suspension IS DISTINCT FROM 'ALLOW' OR NOT FOUND OR c.revision<>NEW.capability_revision OR c.updated_at IS DISTINCT FROM NEW.plan_changed_at
   OR public.org_sync_plan_disposition((CASE WHEN c.enabled THEN 'ENABLED' ELSE 'DISABLED' END),c.revision,c.updated_at,NEW.declared_at,NEW.received_at) IS DISTINCT FROM 'HOLD'
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='state intake decision unavailable';END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_sync_state_intake_source BEFORE INSERT ON org_sync_state_intakes FOR EACH ROW EXECUTE FUNCTION org_sync_state_intake_source();

CREATE FUNCTION org_sync_state_intake_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NOT EXISTS(SELECT 1 FROM public.org_sync_acknowledgements a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.result=NEW.result AND a.code=NEW.code AND a.received_at=NEW.received_at AND a.entity_revision IS NULL)
 OR(NEW.result='ACCEPTED_FOR_REVIEW' AND NOT EXISTS(SELECT 1 FROM public.org_sync_review_items i WHERE i.org_id=NEW.org_id AND i.operation_id=NEW.operation_id AND i.code=NEW.code))
 OR EXISTS(SELECT 1 FROM public.org_sync_effects e WHERE e.org_id=NEW.org_id AND e.operation_id=NEW.operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='state intake publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_sync_state_intake_complete AFTER INSERT ON org_sync_state_intakes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_state_intake_complete();

-- State codes exist only with their recorded decision; ordinary
-- acknowledgements read no new table (nested branch, as 218).
CREATE FUNCTION org_sync_ack_state_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 IF NEW.code IN('ORG_SUSPENDED_GRACE','ORG_SUSPENDED','PLAN_CHANGED') THEN
  prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
  IF NOT EXISTS(SELECT 1 FROM public.org_sync_state_intakes s WHERE s.org_id=NEW.org_id AND s.operation_id=NEW.operation_id AND s.result=NEW.result AND s.code=NEW.code AND s.received_at=NEW.received_at)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='state intake acknowledgement source unavailable';END IF;
  PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_sync_ack_state_source AFTER INSERT ON org_sync_acknowledgements DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_ack_state_source();
