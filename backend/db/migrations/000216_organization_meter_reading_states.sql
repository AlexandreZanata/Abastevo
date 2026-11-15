-- EMP-METER-003 / EMP-METER-004 (B-BR-EMP-280..285, EDR-184). Readings stay
-- insert-only evidence; their classification history is a separate
-- append-only table. Revision 1 (INTAKE) equals the reading's recorded
-- classification; the latest revision is the current state; a reading with
-- no history (recorded before this migration) keeps its recorded state.
-- Person decisions (CONFIRMED, VOIDED, SUPERSEDED) are written by
-- EMP-METER-004 into the same table.
CREATE FUNCTION org_meter_flags_canonical(flags TEXT[]) RETURNS BOOLEAN
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT flags IS NOT NULL AND array_ndims(flags) IS NOT DISTINCT FROM CASE WHEN cardinality(flags)=0 THEN NULL ELSE 1 END
 AND flags=ARRAY(SELECT c.f FROM unnest(ARRAY['READING_BELOW_PREVIOUS','READING_ABOVE_NEXT','IMPLAUSIBLE_RATE','FUTURE_INSTANT']) WITH ORDINALITY AS c(f,n) WHERE c.f=ANY(flags) ORDER BY c.n)
$$;
CREATE TABLE org_meter_reading_states(
 org_id UUID NOT NULL,meter_id UUID NOT NULL,reading_id UUID NOT NULL,
 revision INTEGER NOT NULL CHECK(revision BETWEEN 1 AND 1000000),
 state TEXT NOT NULL CHECK(state IN('VALID','SUSPECT','VOIDED')),
 flags TEXT[] NOT NULL DEFAULT '{}' CHECK(org_meter_flags_canonical(flags)),
 retroactive BOOLEAN NOT NULL DEFAULT false,
 cause TEXT NOT NULL CHECK(cause IN('INTAKE','REEVALUATION','CONFIRMED','VOIDED','SUPERSEDED')),
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_membership_id UUID,reason TEXT,superseded_by UUID,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(recorded_at)),
 PRIMARY KEY(org_id,meter_id,reading_id,revision),
 CONSTRAINT org_meter_reading_states_operation_key UNIQUE(org_id,meter_id,reading_id,operation_id),
 CONSTRAINT org_meter_reading_states_reading_fkey FOREIGN KEY(org_id,meter_id,reading_id) REFERENCES org_meter_readings(org_id,meter_id,id),
 CONSTRAINT org_meter_reading_states_actor_fkey FOREIGN KEY(org_id,actor_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_meter_reading_states_replacement_fkey FOREIGN KEY(org_id,meter_id,superseded_by) REFERENCES org_meter_readings(org_id,meter_id,id),
 CONSTRAINT org_meter_reading_states_flags_state CHECK((state='SUSPECT')=(cardinality(flags)>0)),
 CONSTRAINT org_meter_reading_states_note CHECK(state<>'VOIDED' OR NOT retroactive),
 CONSTRAINT org_meter_reading_states_system CHECK(cause NOT IN('INTAKE','REEVALUATION') OR(actor_membership_id IS NULL AND reason IS NULL)),
 CONSTRAINT org_meter_reading_states_person CHECK(cause IN('INTAKE','REEVALUATION') OR(actor_membership_id IS NOT NULL AND reason IS NOT NULL)),
 CONSTRAINT org_meter_reading_states_reason CHECK(reason IS NULL OR org_asset_text_valid(reason,280,true)),
 CONSTRAINT org_meter_reading_states_intake CHECK(cause<>'INTAKE' OR(revision=1 AND state IN('VALID','SUSPECT'))),
 CONSTRAINT org_meter_reading_states_terminal CHECK((state='VOIDED')=(cause IN('VOIDED','SUPERSEDED'))),
 CONSTRAINT org_meter_reading_states_confirmed CHECK(cause<>'CONFIRMED' OR state='VALID'),
 CONSTRAINT org_meter_reading_states_supersession CHECK((cause='SUPERSEDED')=(superseded_by IS NOT NULL) AND(superseded_by IS NULL OR superseded_by<>reading_id))
);
CREATE UNIQUE INDEX org_meter_reading_states_one_void ON org_meter_reading_states(org_id,meter_id,reading_id) WHERE state='VOIDED';
CREATE UNIQUE INDEX org_meter_reading_states_one_supersession ON org_meter_reading_states(org_id,meter_id,superseded_by) WHERE superseded_by IS NOT NULL;
CREATE INDEX org_meter_reading_states_operation ON org_meter_reading_states(org_id,operation_id);
-- org_meter_readings.state is from now on the recorded classification at
-- intake (VALID or SUSPECT); the current state is the latest revision here.

-- Defence in depth for fleet §6.5: serialise on the organization root and
-- the meter like the observation guard, then admit only the next revision
-- and the documented transitions.
CREATE FUNCTION org_meter_reading_state_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE r public.org_meter_readings%ROWTYPE;p public.org_meter_reading_states%ROWTYPE;prior_state TEXT;prior_flags TEXT[];prior_revision INTEGER;replacement public.org_meter_readings%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 PERFORM 1 FROM public.org_meters WHERE org_id=NEW.org_id AND id=NEW.meter_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private meter not found';END IF;
 SELECT * INTO r FROM public.org_meter_readings WHERE org_id=NEW.org_id AND meter_id=NEW.meter_id AND id=NEW.reading_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reading not found';END IF;
 SELECT * INTO p FROM public.org_meter_reading_states WHERE org_id=NEW.org_id AND meter_id=NEW.meter_id AND reading_id=NEW.reading_id ORDER BY revision DESC LIMIT 1;
 IF FOUND THEN prior_state=p.state;prior_flags=p.flags;prior_revision=p.revision;ELSE prior_state=r.state;prior_flags='{}';prior_revision=0;END IF;
 IF NEW.revision IS DISTINCT FROM prior_revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reading state revision unavailable';END IF;
 IF NEW.cause='INTAKE' THEN
  IF NEW.state IS DISTINCT FROM r.state THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reading intake classification mismatch';END IF;
 ELSIF prior_state='VOIDED' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reading already voided';
 ELSIF prior_state='VALID' AND NEW.state<>'VOIDED' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='valid reading transition unavailable';
 ELSIF NEW.cause='CONFIRMED' AND prior_flags&&ARRAY['READING_BELOW_PREVIOUS','READING_ABOVE_NEXT'] THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='monotonicity flag cannot be confirmed';
 ELSIF NEW.cause='REEVALUATION' AND(NEW.state=prior_state AND NEW.flags=prior_flags OR('FUTURE_INSTANT'=ANY(prior_flags)) IS DISTINCT FROM('FUTURE_INSTANT'=ANY(NEW.flags))) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reading reevaluation unavailable';
 END IF;
 IF NEW.cause='SUPERSEDED' THEN
  SELECT * INTO replacement FROM public.org_meter_readings WHERE org_id=NEW.org_id AND meter_id=NEW.meter_id AND id=NEW.superseded_by;
  IF NOT FOUND OR replacement.operation_id<>NEW.operation_id OR replacement.segment_id<>r.segment_id THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='supersession source unavailable';END IF;
 END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_meter_reading_state_source BEFORE INSERT ON org_meter_reading_states FOR EACH ROW EXECUTE FUNCTION org_meter_reading_state_guard();
ALTER TABLE org_meter_reading_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_meter_reading_states FORCE ROW LEVEL SECURITY;
CREATE POLICY org_meter_reading_states_scope ON org_meter_reading_states USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_meter_reading_states_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_meter_reading_states FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();
REVOKE ALL ON org_meter_reading_states FROM PUBLIC;

-- The 201 guard unchanged except one clause: an accepted held review may now
-- apply its reading with the validator's SUSPECT classification
-- (B-BR-EMP-284); every other source, time, value and identity check stays.
CREATE OR REPLACE FUNCTION org_meter_observation_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE m public.org_meters%ROWTYPE;s public.org_meter_segments%ROWTYPE;original_received TIMESTAMPTZ;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO m FROM public.org_meters WHERE org_id=NEW.org_id AND id=NEW.meter_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private meter not found';END IF;
 SELECT * INTO s FROM public.org_meter_segments WHERE org_id=NEW.org_id AND meter_id=NEW.meter_id AND id=NEW.segment_id;
 IF NOT FOUND OR(NEW.occurred_at,NEW.occurred_submicro)<(m.installed_at,m.installed_submicro) OR(m.removed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(m.removed_at,m.removed_submicro))
 OR(NEW.occurred_at,NEW.occurred_submicro)<(s.opened_at,s.opened_submicro) OR(s.closed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(s.closed_at,s.closed_submicro))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='observation outside meter service interval';END IF;
 IF NEW.value>(CASE m.kind WHEN 'ODOMETER' THEN 1000000000000 ELSE 3600000000 END) OR(m.modulus IS NOT NULL AND NEW.value>=m.modulus)
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid meter observation';END IF;
 IF NEW.review_resolution_id IS NOT NULL THEN
  SELECT a.received_at INTO original_received FROM public.org_sync_review_resolutions z JOIN public.org_sync_review_items i ON(i.org_id,i.id)=(z.org_id,z.review_item_id)
  JOIN public.org_meter_held_readings h ON(h.org_id,h.review_item_id)=(i.org_id,i.id) JOIN public.org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
  JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
  WHERE z.org_id=NEW.org_id AND z.id=NEW.review_resolution_id AND z.outcome='ACCEPTED' AND z.reading_id=NEW.id AND NEW.id=h.reading_id
  AND NEW.meter_id=h.meter_id AND NEW.segment_id=h.segment_id AND NEW.value=h.value AND(NEW.occurred_at,NEW.occurred_submicro)=(h.occurred_at,h.occurred_submicro)
  AND NEW.entry_zone=h.entry_zone AND NEW.device_offset_seconds=h.device_offset_seconds AND NEW.operation_id=o.operation_id AND NEW.request_hash=h.request_hash
  AND NEW.author_membership_id=o.membership_id AND NEW.capture_kind='ENROLLED_DEVICE' AND NEW.enrollment_id=o.enrollment_id AND NEW.web_session_id IS NULL AND NEW.origin=h.origin AND NEW.method=h.method AND NEW.state IN('VALID','SUSPECT');
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reviewed observation source unavailable';END IF;
  NEW.received_at=original_received;RETURN NEW;
 END IF;
 IF EXISTS(SELECT 1 FROM public.org_meter_held_readings WHERE org_id=NEW.org_id AND(reading_id=NEW.id OR operation_id=NEW.operation_id)) THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_meter_readings_pkey',MESSAGE='meter reading identity unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.id=m.asset_id AND a.disposed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(a.disposed_at,a.disposed_submicro)) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='observation after asset disposal requires review';END IF;
 IF NEW.capture_kind='ENROLLED_DEVICE' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_device_enrollments WHERE org_id=NEW.org_id AND id=NEW.enrollment_id AND membership_id=NEW.author_membership_id AND state='ENROLLED') THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='observation source not found';END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM public.company_web_sessions c JOIN public.org_memberships p ON p.account_id=c.account_id WHERE p.org_id=NEW.org_id AND p.id=NEW.author_membership_id AND c.id=NEW.web_session_id) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='observation source not found';END IF;
 END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;
