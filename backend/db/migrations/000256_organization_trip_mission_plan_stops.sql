-- EMP-TRIP-010. Stops and legs: planned versus actual (B-BR-EMP-475..480,
-- EDR-307..310). A PLAN command may carry 1..20 ordered free-text stops;
-- they are stored insert-only with the planning revision that PLAN produced,
-- written by its own transaction after the head moves and before its receipt.
-- The observed stops and legs are derived from the trip's logbook events and
-- are never stored. No existing table, function, row or usage source changes;
-- no unit, cost-centre, asset or employee key is added.

CREATE TABLE org_trip_mission_plan_stops(
 org_id UUID NOT NULL,
 mission_id UUID NOT NULL,
 revision BIGINT NOT NULL,
 sequence SMALLINT NOT NULL CHECK(sequence BETWEEN 1 AND 20),
 place_text TEXT NOT NULL CHECK(org_trip_mission_plan_text_valid(place_text)),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,mission_id,revision,sequence),
 CONSTRAINT org_trip_mission_plan_stops_plan_fkey FOREIGN KEY(org_id,mission_id,revision) REFERENCES org_trip_mission_plans(org_id,mission_id,revision),
 CHECK(isfinite(recorded_at))
);

-- A stop of the planning revision whose PLAN is running in this transaction,
-- under the organization root and mission locks: the head already moved to
-- that revision and is PLANNED, the plan's receipt does not exist yet and the
-- stops arrive in sequence.
CREATE FUNCTION org_trip_mission_plan_stop_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE p public.org_trip_mission_plans%ROWTYPE;m public.org_trip_missions%ROWTYPE;stored INTEGER;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO m FROM public.org_trip_missions WHERE org_id=NEW.org_id AND id=NEW.mission_id FOR UPDATE;
 SELECT * INTO p FROM public.org_trip_mission_plans WHERE org_id=NEW.org_id AND mission_id=NEW.mission_id AND revision=NEW.revision;
 IF m.id IS NULL OR p.mission_id IS NULL THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT count(*) INTO stored FROM public.org_trip_mission_plan_stops WHERE org_id=NEW.org_id AND mission_id=NEW.mission_id AND revision=NEW.revision;
 IF m.revision<>NEW.revision OR m.state<>'PLANNED' OR NEW.sequence<>stored+1
 OR EXISTS(SELECT 1 FROM public.org_trip_mission_commands c WHERE c.org_id=NEW.org_id AND c.operation_id=p.operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission plan stop source unavailable';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_mission_plan_stop_source BEFORE INSERT ON org_trip_mission_plan_stops FOR EACH ROW EXECUTE FUNCTION org_trip_mission_plan_stop_source();

-- At commit a stop exists only with the PLAN receipt of its revision.
CREATE FUNCTION org_trip_mission_plan_stop_published() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NOT EXISTS(SELECT 1 FROM public.org_trip_mission_plans p JOIN public.org_trip_mission_commands c ON(c.org_id,c.operation_id)=(p.org_id,p.operation_id)
  WHERE p.org_id=NEW.org_id AND p.mission_id=NEW.mission_id AND p.revision=NEW.revision AND c.kind='PLAN' AND c.mission_id=NEW.mission_id AND c.result_revision=NEW.revision)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission plan stop publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_trip_mission_plan_stop_published AFTER INSERT ON org_trip_mission_plan_stops DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_trip_mission_plan_stop_published();

ALTER TABLE org_trip_mission_plan_stops ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_trip_mission_plan_stops FORCE ROW LEVEL SECURITY;
CREATE POLICY org_trip_mission_plan_stops_scope ON org_trip_mission_plan_stops USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_trip_mission_plan_stops_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_trip_mission_plan_stops FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history();
REVOKE ALL ON org_trip_mission_plan_stops FROM PUBLIC;
