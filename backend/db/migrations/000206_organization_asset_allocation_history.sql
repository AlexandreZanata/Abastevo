-- EMP-FLEET-005. Unit and cost-centre allocation history (B-BR-EMP-223..226,
-- EDR-154..155). Applied migrations stay unchanged. The single FLEET-002 unit
-- column (org_assets.unit_id) and the immutable initial allocation table
-- (org_asset_unit_allocations) are kept unchanged as the rollback image until
-- the next phase; this migration adds tables, guards and one exact data move.

-- EDR-154: a GiST exclusion over (uuid =, range &&) needs btree_gist
-- (PostgreSQL contrib, PostgreSQL License, trusted, in the pinned image).
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- B-BR-EMP-223: half-open [valid_from, valid_to) periods of whole microseconds,
-- same-organization references and no overlap per asset, whoever writes.
CREATE TABLE org_asset_unit_periods(
 org_id UUID NOT NULL,asset_id UUID NOT NULL,unit_id UUID NOT NULL,
 valid_from TIMESTAMPTZ NOT NULL,valid_to TIMESTAMPTZ,recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,asset_id,valid_from),
 CONSTRAINT org_asset_unit_periods_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_asset_unit_periods_unit_fkey FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id),
 CONSTRAINT org_asset_unit_periods_bounds_check CHECK(isfinite(valid_from) AND valid_from>='0001-01-01 00:00:00+00'::timestamptz AND valid_from<'10000-01-01 00:00:00+00'::timestamptz
  AND(valid_to IS NULL OR(isfinite(valid_to) AND valid_to>valid_from AND valid_to<'10000-01-01 00:00:00+00'::timestamptz)) AND isfinite(recorded_at)),
 CONSTRAINT org_asset_unit_periods_no_overlap EXCLUDE USING gist(org_id WITH =,asset_id WITH =,tstzrange(valid_from,valid_to,'[)') WITH &&)
);
CREATE TABLE org_asset_cost_centre_periods(
 org_id UUID NOT NULL,asset_id UUID NOT NULL,cost_centre_id UUID NOT NULL,
 valid_from TIMESTAMPTZ NOT NULL,valid_to TIMESTAMPTZ,recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,asset_id,valid_from),
 CONSTRAINT org_asset_cost_centre_periods_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_asset_cost_centre_periods_centre_fkey FOREIGN KEY(org_id,cost_centre_id) REFERENCES org_cost_centres(org_id,id),
 CONSTRAINT org_asset_cost_centre_periods_bounds_check CHECK(isfinite(valid_from) AND valid_from>='0001-01-01 00:00:00+00'::timestamptz AND valid_from<'10000-01-01 00:00:00+00'::timestamptz
  AND(valid_to IS NULL OR(isfinite(valid_to) AND valid_to>valid_from AND valid_to<'10000-01-01 00:00:00+00'::timestamptz)) AND isfinite(recorded_at)),
 CONSTRAINT org_asset_cost_centre_periods_no_overlap EXCLUDE USING gist(org_id WITH =,asset_id WITH =,tstzrange(valid_from,valid_to,'[)') WITH &&)
);

-- B-BR-EMP-225: the exact data move. Forced RLS is lifted only inside this
-- atomic migration (the 169/200 precedent) so the owner reads every
-- organization; it is restored before commit. Nothing is guessed: any
-- inconsistent source aborts the whole migration with no change.
ALTER TABLE org_assets NO FORCE ROW LEVEL SECURITY;
ALTER TABLE org_asset_unit_allocations NO FORCE ROW LEVEL SECURITY;
DO $$
BEGIN
 IF EXISTS(SELECT 1 FROM public.org_assets a WHERE
  (a.state='DRAFT' AND EXISTS(SELECT 1 FROM public.org_asset_unit_allocations p WHERE p.org_id=a.org_id AND p.asset_id=a.id))
  OR(a.state<>'DRAFT' AND(a.unit_id IS NULL
   OR(SELECT count(*) FROM public.org_asset_unit_allocations p WHERE p.org_id=a.org_id AND p.asset_id=a.id)<>1
   OR NOT EXISTS(SELECT 1 FROM public.org_asset_unit_allocations p WHERE p.org_id=a.org_id AND p.asset_id=a.id AND p.unit_id=a.unit_id AND p.ended_at IS NULL))))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset allocation history source inconsistent';END IF;
END $$;
-- The first period starts, and was recorded, at the database instant of the
-- FLEET-002 initial allocation; DRAFT assets and cost centres receive nothing.
INSERT INTO org_asset_unit_periods(org_id,asset_id,unit_id,valid_from,valid_to,recorded_at)
SELECT org_id,asset_id,unit_id,started_at,NULL,started_at FROM org_asset_unit_allocations ORDER BY org_id,asset_id;
ALTER TABLE org_asset_unit_allocations FORCE ROW LEVEL SECURITY;
ALTER TABLE org_assets FORCE ROW LEVEL SECURITY;

ALTER TABLE org_asset_unit_periods ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_asset_unit_periods FORCE ROW LEVEL SECURITY;
CREATE POLICY org_asset_unit_periods_scope ON org_asset_unit_periods USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_asset_unit_periods FROM PUBLIC;
ALTER TABLE org_asset_cost_centre_periods ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_asset_cost_centre_periods FORCE ROW LEVEL SECURITY;
CREATE POLICY org_asset_cost_centre_periods_scope ON org_asset_cost_centre_periods USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_asset_cost_centre_periods FROM PUBLIC;

-- B-BR-EMP-223: a new period names an asset of this organization that has
-- left DRAFT and starts no earlier than its first unit period (activation).
-- The caller already admitted the unit or cost centre under the root lock.
CREATE FUNCTION org_asset_period_source(selected_org UUID,selected_asset UUID,starts TIMESTAMPTZ) RETURNS void
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE asset_state TEXT;initial TIMESTAMPTZ;
BEGIN
 SELECT state INTO asset_state FROM public.org_assets WHERE org_id=selected_org AND id=selected_asset;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT min(started_at) INTO initial FROM public.org_asset_unit_allocations WHERE org_id=selected_org AND asset_id=selected_asset;
 IF asset_state='DRAFT' OR initial IS NULL OR starts<initial THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset allocation unavailable';END IF;
END $$;
-- Insert: admit an ACTIVE structure, prove the source and stamp the recording
-- instant. Update: only closing an open period once; never delete/truncate.
CREATE FUNCTION org_asset_unit_period_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.valid_to IS NOT NULL OR NEW.valid_to IS NULL OR ROW(NEW.org_id,NEW.asset_id,NEW.unit_id,NEW.valid_from,NEW.recorded_at) IS DISTINCT FROM ROW(OLD.org_id,OLD.asset_id,OLD.unit_id,OLD.valid_from,OLD.recorded_at)
  THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='asset allocation history is immutable';END IF;
  PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept allocation changes';END IF;
  RETURN NEW;
 END IF;
 PERFORM public.org_unit_admit(NEW.org_id,NEW.unit_id);
 PERFORM public.org_asset_period_source(NEW.org_id,NEW.asset_id,NEW.valid_from);
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_asset_unit_period_write BEFORE INSERT OR UPDATE ON org_asset_unit_periods FOR EACH ROW EXECUTE FUNCTION org_asset_unit_period_guard();
CREATE TRIGGER org_asset_unit_periods_immutable BEFORE DELETE OR TRUNCATE ON org_asset_unit_periods FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();
CREATE FUNCTION org_asset_cost_centre_period_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.valid_to IS NOT NULL OR NEW.valid_to IS NULL OR ROW(NEW.org_id,NEW.asset_id,NEW.cost_centre_id,NEW.valid_from,NEW.recorded_at) IS DISTINCT FROM ROW(OLD.org_id,OLD.asset_id,OLD.cost_centre_id,OLD.valid_from,OLD.recorded_at)
  THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='asset allocation history is immutable';END IF;
  PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept allocation changes';END IF;
  RETURN NEW;
 END IF;
 PERFORM public.org_cost_centre_admit(NEW.org_id,NEW.cost_centre_id);
 PERFORM public.org_asset_period_source(NEW.org_id,NEW.asset_id,NEW.valid_from);
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_asset_cost_centre_period_write BEFORE INSERT OR UPDATE ON org_asset_cost_centre_periods FOR EACH ROW EXECUTE FUNCTION org_asset_cost_centre_period_guard();
CREATE TRIGGER org_asset_cost_centre_periods_immutable BEFORE DELETE OR TRUNCATE ON org_asset_cost_centre_periods FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();

-- B-BR-EMP-224: activation keeps recording its FLEET-002 initial allocation;
-- the database opens the first unit period from it, with the same unit and
-- exact instant, in the same transaction (previous image included).
CREATE FUNCTION org_asset_initial_unit_period() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 INSERT INTO public.org_asset_unit_periods(org_id,asset_id,unit_id,valid_from,valid_to) VALUES(NEW.org_id,NEW.asset_id,NEW.unit_id,NEW.started_at,NEW.ended_at);
 RETURN NULL;
END $$;
CREATE TRIGGER org_asset_initial_unit_period AFTER INSERT ON org_asset_unit_allocations FOR EACH ROW EXECUTE FUNCTION org_asset_initial_unit_period();

-- B-BR-EMP-224 (AC2), checked at commit: a DRAFT asset has no unit period; any
-- other asset has a gap-free unit chain that starts at its initial allocation
-- and ends in exactly one open-ended period. Lifecycle commands are checked
-- only when the state changes, so other asset updates never read periods.
CREATE FUNCTION org_asset_unit_chain_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_org UUID;selected_asset UUID;asset_state TEXT;initial TIMESTAMPTZ;
BEGIN
 selected_org=NEW.org_id;
 IF TG_TABLE_NAME='org_assets' THEN selected_asset=NEW.id;ELSE selected_asset=NEW.asset_id;END IF;
 SELECT state INTO asset_state FROM public.org_assets WHERE org_id=selected_org AND id=selected_asset;
 IF NOT FOUND THEN RETURN NULL;END IF;
 IF asset_state='DRAFT' THEN
  IF EXISTS(SELECT 1 FROM public.org_asset_unit_periods WHERE org_id=selected_org AND asset_id=selected_asset) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset unit allocation history incomplete';END IF;
  RETURN NULL;
 END IF;
 SELECT min(started_at) INTO initial FROM public.org_asset_unit_allocations WHERE org_id=selected_org AND asset_id=selected_asset;
 IF initial IS NULL
  OR(SELECT count(*) FROM public.org_asset_unit_periods WHERE org_id=selected_org AND asset_id=selected_asset AND valid_to IS NULL)<>1
  OR(SELECT min(valid_from) FROM public.org_asset_unit_periods WHERE org_id=selected_org AND asset_id=selected_asset) IS DISTINCT FROM initial
  OR EXISTS(SELECT 1 FROM public.org_asset_unit_periods p WHERE p.org_id=selected_org AND p.asset_id=selected_asset AND p.valid_to IS NOT NULL
   AND NOT EXISTS(SELECT 1 FROM public.org_asset_unit_periods q WHERE q.org_id=p.org_id AND q.asset_id=p.asset_id AND q.valid_from=p.valid_to))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset unit allocation history incomplete';END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_asset_unit_periods_complete AFTER INSERT OR UPDATE ON org_asset_unit_periods DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_asset_unit_chain_complete();
CREATE CONSTRAINT TRIGGER org_asset_unit_chain_state AFTER UPDATE OF state ON org_assets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION org_asset_unit_chain_complete();

-- The 199 asset-use source with the two period tables recognised as history,
-- never as use (an asset with periods is not a DRAFT anyway).
CREATE OR REPLACE FUNCTION org_asset_current_usage(selected_org UUID,selected_asset UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_assets WHERE org_id=selected_org AND id=selected_asset) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_assets'::regclass
 AND NOT((c.conrelid='public.org_asset_tanks'::regclass AND c.conname='org_asset_tanks_asset_fkey')
 OR(c.conrelid='public.org_asset_expected'::regclass AND c.conname='org_asset_expected_asset_fkey')
 OR(c.conrelid='public.org_asset_unit_allocations'::regclass AND c.conname='org_asset_allocation_asset_fkey')
 OR(c.conrelid='public.org_asset_revisions'::regclass AND c.conname='org_asset_revision_asset_fkey')
 OR(c.conrelid='public.org_meters'::regclass AND c.conname='org_meters_asset_fkey')
 OR(c.conrelid='public.org_meter_held_readings'::regclass AND c.conname='org_meter_held_readings_asset_fkey')
 OR(c.conrelid='public.org_asset_unit_periods'::regclass AND c.conname='org_asset_unit_periods_asset_fkey')
 OR(c.conrelid='public.org_asset_cost_centre_periods'::regclass AND c.conname='org_asset_cost_centre_periods_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset));
END $$;

-- The 199 unit-use source: employee UNION/DISTINCT semantics preserved; an
-- ACTIVE asset counts when a period for the unit is open-ended or ends after
-- the database clock (current or scheduled). Ended periods are history.
CREATE OR REPLACE FUNCTION org_unit_current_usage(selected_org UUID,selected_unit UUID) RETURNS JSONB LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE employee_count BIGINT;asset_count BIGINT;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=selected_org AND id=selected_unit) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_units'::regclass
 AND NOT((c.conrelid='public.org_onboarding_commands'::regclass AND c.conname='org_onboarding_commands_org_id_unit_id_fkey')
 OR(c.conrelid='public.org_unit_commands'::regclass AND c.conname='org_unit_commands_unit_fkey')
 OR(c.conrelid='public.org_employee_profiles'::regclass AND c.conname='org_employee_profile_unit_fkey')
 OR(c.conrelid='public.org_teams'::regclass AND c.conname='org_team_unit_fkey')
 OR(c.conrelid='public.org_assets'::regclass AND c.conname='org_asset_unit_fkey')
 OR(c.conrelid='public.org_asset_unit_allocations'::regclass AND c.conname='org_asset_allocation_unit_fkey')
 OR(c.conrelid='public.org_meter_held_readings'::regclass AND c.conname='org_meter_held_readings_unit_fkey')
 OR(c.conrelid='public.org_asset_unit_periods'::regclass AND c.conname='org_asset_unit_periods_unit_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';END IF;
 SELECT count(DISTINCT employee_id) INTO employee_count FROM(
 SELECT e.id AS employee_id FROM public.org_employee_profiles p JOIN public.org_employee_identities e ON(e.org_id,e.id)=(p.org_id,p.employee_id)
 WHERE p.org_id=selected_org AND p.home_unit_id=selected_unit AND e.state='ACTIVE'
 UNION
 SELECT e.id AS employee_id FROM public.org_team_links l JOIN public.org_teams t ON(t.org_id,t.id)=(l.org_id,l.team_id)
 JOIN public.org_employee_identities e ON(e.org_id,e.id)=(l.org_id,l.employee_id)
 WHERE l.org_id=selected_org AND t.unit_id=selected_unit AND l.ended_at IS NULL AND e.state='ACTIVE') current_employees;
 SELECT count(DISTINCT p.asset_id) INTO asset_count FROM public.org_asset_unit_periods p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
 WHERE p.org_id=selected_org AND p.unit_id=selected_unit AND(p.valid_to IS NULL OR p.valid_to>clock_timestamp()) AND a.state='ACTIVE';
 RETURN jsonb_build_object('assets',asset_count,'employees',employee_count,'budgets',0,'operations',0);
END $$;

-- The 175 cost-centre-use source with its first actual consumer: an ACTIVE
-- asset with a current or scheduled period for the centre.
CREATE OR REPLACE FUNCTION org_cost_centre_current_usage(selected_org UUID,selected_centre UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_cost_centres WHERE org_id=selected_org AND id=selected_centre) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid IN('public.org_cost_centres'::regclass,'public.org_cost_centre_codes'::regclass)
  AND NOT((c.conrelid='public.org_cost_centre_codes'::regclass AND c.conname='org_cost_centre_codes_centre_fkey')
   OR(c.conrelid='public.org_cost_centre_commands'::regclass AND c.conname='org_cost_centre_commands_centre_fkey')
   OR(c.conrelid='public.org_cost_centres'::regclass AND c.conname='org_cost_centre_current_code_fkey')
   OR(c.conrelid='public.org_asset_cost_centre_periods'::regclass AND c.conname='org_asset_cost_centre_periods_centre_fkey'))) THEN
  RAISE EXCEPTION USING ERRCODE='P7804',MESSAGE='cost-centre usage source unavailable';
 END IF;
 RETURN jsonb_build_object('assets',(SELECT count(DISTINCT p.asset_id) FROM public.org_asset_cost_centre_periods p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
  WHERE p.org_id=selected_org AND p.cost_centre_id=selected_centre AND(p.valid_to IS NULL OR p.valid_to>clock_timestamp()) AND a.state='ACTIVE'),'employees',0,'budgets',0,'operations',0);
END $$;
