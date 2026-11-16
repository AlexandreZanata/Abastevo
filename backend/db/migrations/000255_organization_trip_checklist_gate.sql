-- EMP-TRIP-019. Checklist gate at trip start and finish (B-BR-EMP-511..516,
-- EDR-331..334). Migration 000255 (EMP-TRIP-010 moves to 000256; 000250
-- stays unused). Additive: a trip start may name its departure checklist run
-- and a finish its return run through a soft reference decided at receipt;
-- an accepted start links its departure run once, a finish links a counting
-- return run, and a held start stores its gate reason. Two insert-only
-- forced-RLS tables and their source guards; no existing function, table or
-- row changes.

-- The departure validity window of a run at an instant: the run template's
-- policy revision in force at that instant by the server's history (the
-- latest revision effective at or before it, else revision 1). NULL when the
-- run is not stored in the organization.
CREATE FUNCTION org_trip_checklist_validity(p_org UUID,p_run UUID,p_at TIMESTAMPTZ) RETURNS INTEGER
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT COALESCE((SELECT p.validity_seconds FROM public.org_checklist_policy_revisions p JOIN public.org_checklist_runs r ON(r.org_id,r.template_id)=(p.org_id,p.template_id)
  WHERE r.org_id=p_org AND r.id=p_run AND p.effective_at<=p_at ORDER BY p.revision DESC LIMIT 1),
  (SELECT p.validity_seconds FROM public.org_checklist_policy_revisions p JOIN public.org_checklist_runs r ON(r.org_id,r.template_id)=(p.org_id,p.template_id)
  WHERE r.org_id=p_org AND r.id=p_run AND p.revision=1))
 $$;

-- Whether a run's server outcome was BLOCKING at an instant (EDR-285: from
-- its declared completion until its release was recorded).
CREATE FUNCTION org_trip_checklist_blocking_at(p_org UUID,p_run UUID,p_at TIMESTAMPTZ,p_submicro SMALLINT) RETURNS BOOLEAN
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_checklist_runs r LEFT JOIN public.org_checklist_run_releases l ON(l.org_id,l.run_id)=(r.org_id,r.id)
  WHERE r.org_id=p_org AND r.id=p_run AND r.recheck_outcome='BLOCKING' AND(r.occurred_at,r.occurred_submicro)<=(p_at,p_submicro)
  AND(l.recorded_at IS NULL OR(p_at,p_submicro)<(l.recorded_at,0::smallint)))
 $$;

-- 1. One insert-only link per trip and context; a run is the evidence of one
-- trip (B-BR-EMP-514/515).
CREATE TABLE org_trip_checklist_links(
 org_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 context TEXT NOT NULL CHECK(context IN('DEPARTURE','RETURN')),
 run_id UUID NOT NULL,
 operation_id UUID NOT NULL,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,trip_id,context),
 CONSTRAINT org_trip_checklist_links_run_key UNIQUE(org_id,run_id),
 CONSTRAINT org_trip_checklist_links_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_checklist_links_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_trip_checklist_links_run_fkey FOREIGN KEY(org_id,run_id) REFERENCES org_checklist_runs(org_id,id),
 CONSTRAINT org_trip_checklist_links_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CHECK(isfinite(recorded_at))
);

-- Under the organization root and trip locks (runs are read without a row
-- lock: every run insert and release takes the root lock first, EDR-334):
-- a departure is the trip's own start operation on an IN_PROGRESS trip
-- naming a DEPARTURE run of its asset by its author that was neither
-- BLOCKING nor outside its validity at the declared start; a return is the
-- trip's finish operation naming a RETURN run of its asset by the finishing
-- member completed within the trip.
CREATE FUNCTION org_trip_checklist_link_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;r public.org_checklist_runs%ROWTYPE;f public.org_trip_finishes%ROWTYPE;validity INTEGER;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 SELECT * INTO r FROM public.org_checklist_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 IF t.id IS NULL OR r.id IS NULL THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF r.context<>NEW.context OR r.asset_id<>t.asset_id THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist link source unavailable';END IF;
 IF NEW.context='DEPARTURE' THEN
  validity=public.org_trip_checklist_validity(NEW.org_id,NEW.run_id,t.occurred_at);
  IF t.state<>'IN_PROGRESS' OR t.operation_id<>NEW.operation_id OR r.author_membership_id<>t.author_membership_id OR validity IS NULL
  OR public.org_trip_checklist_blocking_at(NEW.org_id,NEW.run_id,t.occurred_at,t.occurred_submicro)
  OR(t.occurred_at,t.occurred_submicro)<(r.occurred_at,r.occurred_submicro)
  OR(t.occurred_at,t.occurred_submicro)>=(r.occurred_at+validity*interval '1 second',r.occurred_submicro)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist link source unavailable';END IF;
 ELSE
  SELECT * INTO f FROM public.org_trip_finishes WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id AND operation_id=NEW.operation_id;
  IF NOT FOUND OR r.author_membership_id<>f.author_membership_id
  OR(r.occurred_at,r.occurred_submicro)<(t.occurred_at,t.occurred_submicro) OR(r.occurred_at,r.occurred_submicro)>(f.occurred_at,f.occurred_submicro)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist link source unavailable';END IF;
 END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_checklist_link_source BEFORE INSERT ON org_trip_checklist_links FOR EACH ROW EXECUTE FUNCTION org_trip_checklist_link_source();

-- 2. The gate reason beside a held start (B-BR-EMP-513, EDR-333).
CREATE TABLE org_trip_checklist_holds(
 org_id UUID NOT NULL,
 operation_id UUID NOT NULL,
 review_item_id UUID NOT NULL,
 reason TEXT NOT NULL CHECK(reason IN('CHECKLIST_REQUIRED','CHECKLIST_BLOCKING','CHECKLIST_EXPIRED','ORPHAN_REFERENCE')),
 run_id UUID,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,operation_id),
 CONSTRAINT org_trip_checklist_holds_held_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_trip_held_starts(org_id,operation_id),
 CONSTRAINT org_trip_checklist_holds_review_fkey FOREIGN KEY(org_id,review_item_id,operation_id) REFERENCES org_sync_review_items(org_id,id,operation_id),
 CONSTRAINT org_trip_checklist_holds_run_fkey FOREIGN KEY(org_id,run_id) REFERENCES org_checklist_runs(org_id,id),
 CONSTRAINT org_trip_checklist_holds_run_check CHECK(reason NOT IN('CHECKLIST_BLOCKING','CHECKLIST_EXPIRED') OR run_id IS NOT NULL),
 CHECK(isfinite(recorded_at))
);

-- Under the organization root lock: the held start of the same review item;
-- BLOCKING and EXPIRED name a DEPARTURE run of the held start's asset by its
-- author whose history proves the reason at the held start's instant.
CREATE FUNCTION org_trip_checklist_hold_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE h public.org_trip_held_starts%ROWTYPE;r public.org_checklist_runs%ROWTYPE;author UUID;validity INTEGER;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO h FROM public.org_trip_held_starts WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id AND review_item_id=NEW.review_item_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist hold source unavailable';END IF;
 IF NEW.reason IN('CHECKLIST_BLOCKING','CHECKLIST_EXPIRED') THEN
  SELECT * INTO r FROM public.org_checklist_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
  SELECT o.membership_id INTO author FROM public.org_sync_operations o WHERE o.org_id=NEW.org_id AND o.operation_id=NEW.operation_id;
  validity=public.org_trip_checklist_validity(NEW.org_id,NEW.run_id,h.occurred_at);
  IF r.id IS NULL OR r.context<>'DEPARTURE' OR r.asset_id<>h.asset_id OR r.author_membership_id IS DISTINCT FROM author OR validity IS NULL
  OR(NEW.reason='CHECKLIST_BLOCKING' AND NOT public.org_trip_checklist_blocking_at(NEW.org_id,NEW.run_id,h.occurred_at,h.occurred_submicro))
  OR(NEW.reason='CHECKLIST_EXPIRED' AND(public.org_trip_checklist_blocking_at(NEW.org_id,NEW.run_id,h.occurred_at,h.occurred_submicro)
   OR((h.occurred_at,h.occurred_submicro)>=(r.occurred_at,r.occurred_submicro)
   AND(h.occurred_at,h.occurred_submicro)<(r.occurred_at+validity*interval '1 second',r.occurred_submicro))))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist hold reason unavailable';END IF;
 END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_checklist_hold_source BEFORE INSERT ON org_trip_checklist_holds FOR EACH ROW EXECUTE FUNCTION org_trip_checklist_hold_source();

DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_trip_checklist_links','org_trip_checklist_holds'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
  EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history()',name||'_immutable',name);
  EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;
