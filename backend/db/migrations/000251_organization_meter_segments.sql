-- EMP-METER-005 (B-BR-EMP-481..486, EDR-311..313). A segment closes at most
-- once and a meter is removed at most once, each by one guarded UPDATE; a
-- reset or rollover opens the next segment of the same meter at the same
-- instant. A reading counts in the segment whose half-open interval holds its
-- instant; a METER_REMOVAL reading in the segment its removal closes; org_meter_readings.segment_id stays
-- the segment open at intake. Readings and their history stay insert-only and
-- nothing existing is rewritten.

-- POSSIBLE_ROLLOVER is the last canonical flag (EDR-312).
CREATE OR REPLACE FUNCTION org_meter_flags_canonical(flags TEXT[]) RETURNS BOOLEAN
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT flags IS NOT NULL AND array_ndims(flags) IS NOT DISTINCT FROM CASE WHEN cardinality(flags)=0 THEN NULL ELSE 1 END
 AND flags=ARRAY(SELECT c.f FROM unnest(ARRAY['READING_BELOW_PREVIOUS','READING_ABOVE_NEXT','IMPLAUSIBLE_RATE','FUTURE_INSTANT','POSSIBLE_ROLLOVER']) WITH ORDINALITY AS c(f,n) WHERE c.f=ANY(flags) ORDER BY c.n)
$$;

-- A confirmed rollover is the person cause ROLLOVER (VALID).
ALTER TABLE org_meter_reading_states DROP CONSTRAINT org_meter_reading_states_cause_check;
ALTER TABLE org_meter_reading_states ADD CONSTRAINT org_meter_reading_states_cause_check CHECK(cause IN('INTAKE','REEVALUATION','CONFIRMED','VOIDED','SUPERSEDED','ROLLOVER')),
 ADD CONSTRAINT org_meter_reading_states_rollover CHECK(cause<>'ROLLOVER' OR state='VALID');

-- Segment reasons, the opening operation of a RESET/ROLLOVER segment and the
-- closure written once by the boundary command.
ALTER TABLE org_meter_segments DROP CONSTRAINT org_meter_segments_kind_check;
ALTER TABLE org_meter_segments ADD CONSTRAINT org_meter_segments_kind_check CHECK(kind IN('INSTALLATION','RESET','ROLLOVER')),
 ADD COLUMN opened_operation_id UUID,ADD COLUMN closing_kind TEXT,ADD COLUMN closing_value BIGINT,ADD COLUMN closing_reading_id UUID,ADD COLUMN closing_operation_id UUID,
 ADD COLUMN closing_previous_reading_id UUID,ADD COLUMN closing_implied_usage BIGINT,
 ADD CONSTRAINT org_meter_segments_opening_operation CHECK((kind='INSTALLATION')=(opened_operation_id IS NULL)),
 ADD CONSTRAINT org_meter_segments_closure CHECK((closed_at IS NULL)=(closing_kind IS NULL) AND(closed_at IS NULL)=(closing_operation_id IS NULL)
 AND(closing_kind IS NULL OR closing_kind IN('REMOVAL','RESET','ROLLOVER')) AND(closing_kind IS NOT NULL OR(closing_value IS NULL AND closing_reading_id IS NULL))
 AND(closing_value IS NULL OR closing_value>=0) AND(closing_kind='REMOVAL' OR closing_kind IS NULL OR(closing_value IS NOT NULL AND closing_reading_id IS NULL))
 AND(closing_kind IS DISTINCT FROM 'REMOVAL' OR(closing_value IS NULL)=(closing_reading_id IS NULL))
 AND(closed_at IS NULL)=(closing_previous_reading_id IS NULL) AND(closing_value IS NULL)=(closing_implied_usage IS NULL) AND(closing_implied_usage IS NULL OR closing_implied_usage>=0)),
 ADD CONSTRAINT org_meter_segments_closing_reading_fkey FOREIGN KEY(org_id,meter_id,closing_reading_id) REFERENCES org_meter_readings(org_id,meter_id,id),
 ADD CONSTRAINT org_meter_segments_closing_previous_fkey FOREIGN KEY(org_id,meter_id,closing_previous_reading_id) REFERENCES org_meter_readings(org_id,meter_id,id),
 ADD CONSTRAINT org_meter_segments_opened_operation_fkey FOREIGN KEY(org_id,opened_operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 ADD CONSTRAINT org_meter_segments_closing_operation_fkey FOREIGN KEY(org_id,closing_operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE org_meters ADD COLUMN removal_operation_id UUID,
 ADD CONSTRAINT org_meters_removal CHECK((removed_at IS NULL)=(removal_operation_id IS NULL)),
 ADD CONSTRAINT org_meters_removal_operation_fkey FOREIGN KEY(org_id,removal_operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED;

-- The current state of a reading: its latest history revision, else the
-- recorded classification (B-BR-EMP-284).
CREATE FUNCTION org_meter_reading_state(selected_org UUID,selected_meter UUID,selected_reading UUID) RETURNS TEXT
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT COALESCE((SELECT s.state FROM public.org_meter_reading_states s WHERE s.org_id=selected_org AND s.meter_id=selected_meter AND s.reading_id=selected_reading ORDER BY s.revision DESC LIMIT 1),
 (SELECT r.state FROM public.org_meter_readings r WHERE r.org_id=selected_org AND r.meter_id=selected_meter AND r.id=selected_reading))
$$;

-- The segment a reading counts in (B-BR-EMP-481): a METER_REMOVAL reading
-- stays in the segment it was recorded in (the one its removal closes); any
-- other reading counts in the segment whose [opened, closed) holds its
-- instant. The fleet queries inline the same expression over 000198 columns.
CREATE FUNCTION org_meter_reading_segment(selected_org UUID,selected_meter UUID,selected_reading UUID) RETURNS UUID
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT CASE WHEN r.origin='METER_REMOVAL' THEN r.segment_id ELSE(SELECT g.id FROM public.org_meter_segments g WHERE(g.org_id,g.meter_id)=(r.org_id,r.meter_id)
 AND(g.opened_at,g.opened_submicro)<=(r.occurred_at,r.occurred_submicro) AND(g.closed_at IS NULL OR(r.occurred_at,r.occurred_submicro)<(g.closed_at,g.closed_submicro))
 ORDER BY g.opened_at DESC,g.opened_submicro DESC LIMIT 1) END
 FROM public.org_meter_readings r WHERE r.org_id=selected_org AND r.meter_id=selected_meter AND r.id=selected_reading
$$;

-- Removal and closure are the only updates; DELETE and TRUNCATE stay refused.
DROP TRIGGER org_meters_immutable ON org_meters;
CREATE TRIGGER org_meters_immutable BEFORE DELETE OR TRUNCATE ON org_meters FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();
DROP TRIGGER org_meter_segments_immutable ON org_meter_segments;
CREATE TRIGGER org_meter_segments_immutable BEFORE DELETE OR TRUNCATE ON org_meter_segments FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history();

CREATE FUNCTION org_meter_removal_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid OR OLD.org_id IS DISTINCT FROM NEW.org_id THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF OLD.removed_at IS NOT NULL OR NEW.removed_at IS NULL OR NEW.removal_operation_id IS NULL
 OR(NEW.id,NEW.asset_id,NEW.kind,NEW.label,NEW.modulus,NEW.installed_at,NEW.installed_submicro,NEW.created_operation_id)
 IS DISTINCT FROM(OLD.id,OLD.asset_id,OLD.kind,OLD.label,OLD.modulus,OLD.installed_at,OLD.installed_submicro,OLD.created_operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='meter history is immutable';END IF;
 IF(NEW.removed_at,NEW.removed_submicro)<=(OLD.installed_at,OLD.installed_submicro) OR EXISTS(SELECT 1 FROM public.org_meter_readings r WHERE r.org_id=NEW.org_id AND r.meter_id=NEW.id
 AND(r.occurred_at,r.occurred_submicro)>=(NEW.removed_at,NEW.removed_submicro) AND public.org_meter_reading_state(r.org_id,r.meter_id,r.id)<>'VOIDED'
 AND NOT EXISTS(SELECT 1 FROM public.org_meter_segments s WHERE s.org_id=r.org_id AND s.meter_id=r.meter_id AND s.closing_kind='REMOVAL' AND s.closing_reading_id=r.id))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='meter removal unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_meter_removal_source BEFORE UPDATE ON org_meters FOR EACH ROW EXECUTE FUNCTION org_meter_removal_guard();

CREATE FUNCTION org_meter_segment_close_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE m public.org_meters%ROWTYPE;r public.org_meter_readings%ROWTYPE;p public.org_meter_readings%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid OR OLD.org_id IS DISTINCT FROM NEW.org_id THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO m FROM public.org_meters WHERE org_id=NEW.org_id AND id=NEW.meter_id FOR UPDATE;
 IF OLD.closed_at IS NOT NULL OR NEW.closed_at IS NULL
 OR(NEW.id,NEW.meter_id,NEW.kind,NEW.opened_at,NEW.opened_submicro,NEW.opening_reading_id,NEW.opened_operation_id)
 IS DISTINCT FROM(OLD.id,OLD.meter_id,OLD.kind,OLD.opened_at,OLD.opened_submicro,OLD.opening_reading_id,OLD.opened_operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='meter history is immutable';END IF;
 IF(NEW.closed_at,NEW.closed_submicro)<=(NEW.opened_at,NEW.opened_submicro) OR m.removed_at IS NOT NULL
 OR(NEW.closing_kind='ROLLOVER' AND(m.modulus IS NULL OR NEW.closing_value IS DISTINCT FROM m.modulus))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='meter segment closure unavailable';END IF;
 IF NEW.closing_reading_id IS NOT NULL THEN
  SELECT * INTO r FROM public.org_meter_readings WHERE org_id=NEW.org_id AND meter_id=NEW.meter_id AND id=NEW.closing_reading_id;
  IF NOT FOUND OR r.segment_id<>NEW.id OR(r.occurred_at,r.occurred_submicro)<>(NEW.closed_at,NEW.closed_submicro) OR r.value<>NEW.closing_value OR r.operation_id<>NEW.closing_operation_id
  OR r.origin<>'METER_REMOVAL'
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='meter segment closure unavailable';END IF;
 END IF;
 -- The implied usage is measured from the latest VALID reading counting in
 -- this segment before the boundary; a rollover adds its wrap reading, which
 -- the chain check proves at commit.
 SELECT x.* INTO p FROM public.org_meter_readings x WHERE x.org_id=NEW.org_id AND x.meter_id=NEW.meter_id
 AND(x.occurred_at,x.occurred_submicro)>=(NEW.opened_at,NEW.opened_submicro) AND(x.occurred_at,x.occurred_submicro)<(NEW.closed_at,NEW.closed_submicro)
 AND public.org_meter_reading_state(x.org_id,x.meter_id,x.id)='VALID'
 ORDER BY x.occurred_at DESC,x.occurred_submicro DESC,x.received_at DESC,x.id DESC LIMIT 1;
 IF NOT FOUND OR p.id IS DISTINCT FROM NEW.closing_previous_reading_id OR NEW.closing_value<p.value
 OR(NEW.closing_kind IN('REMOVAL','RESET') AND NEW.closing_implied_usage IS DISTINCT FROM NEW.closing_value-p.value)
 OR(NEW.closing_kind='ROLLOVER' AND NEW.closing_implied_usage<NEW.closing_value-p.value)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='meter segment closure unavailable';END IF;
 -- Nothing that counts may be left at or after the boundary: a removal
 -- leaves no live reading, a reset or rollover no VALID one.
 IF EXISTS(SELECT 1 FROM public.org_meter_readings x WHERE x.org_id=NEW.org_id AND x.meter_id=NEW.meter_id AND x.id IS DISTINCT FROM NEW.closing_reading_id
 AND(x.occurred_at,x.occurred_submicro)>=(NEW.closed_at,NEW.closed_submicro)
 AND public.org_meter_reading_state(x.org_id,x.meter_id,x.id)=ANY(CASE WHEN NEW.closing_kind='REMOVAL' THEN ARRAY['VALID','SUSPECT'] ELSE ARRAY['VALID'] END))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='meter segment closure unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_meter_segment_close_source BEFORE UPDATE ON org_meter_segments FOR EACH ROW EXECUTE FUNCTION org_meter_segment_close_guard();

CREATE FUNCTION org_meter_segment_open_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.kind='INSTALLATION' THEN RETURN NEW;END IF;
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 PERFORM 1 FROM public.org_meters WHERE org_id=NEW.org_id AND id=NEW.meter_id AND removed_at IS NULL FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_meter_segments p WHERE p.org_id=NEW.org_id AND p.meter_id=NEW.meter_id AND p.closing_kind=NEW.kind
 AND(p.closed_at,p.closed_submicro)=(NEW.opened_at,NEW.opened_submicro) AND p.closing_operation_id=NEW.opened_operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='meter segment opening unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_meter_segment_open_source BEFORE INSERT ON org_meter_segments FOR EACH ROW EXECUTE FUNCTION org_meter_segment_open_guard();

-- At commit the chain of the touched meter is whole: every RESET/ROLLOVER
-- closure has its successor, a REMOVAL closure its removed meter (and the
-- reverse), a RESET segment its STANDALONE opening reading recorded in it and
-- a ROLLOVER segment the reading confirmed by its own operation.
CREATE FUNCTION org_meter_chain_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_meter UUID;
BEGIN
 IF TG_TABLE_NAME='org_meters' THEN selected_meter=NEW.id;ELSE selected_meter=NEW.meter_id;END IF;
 IF EXISTS(SELECT 1 FROM public.org_meter_segments s WHERE s.org_id=NEW.org_id AND s.meter_id=selected_meter AND s.closing_kind IN('RESET','ROLLOVER')
  AND NOT EXISTS(SELECT 1 FROM public.org_meter_segments n WHERE n.org_id=s.org_id AND n.meter_id=s.meter_id AND n.kind=s.closing_kind
  AND(n.opened_at,n.opened_submicro)=(s.closed_at,s.closed_submicro) AND n.opened_operation_id=s.closing_operation_id))
 OR EXISTS(SELECT 1 FROM public.org_meter_segments s JOIN public.org_meters m ON(m.org_id,m.id)=(s.org_id,s.meter_id) WHERE s.org_id=NEW.org_id AND s.meter_id=selected_meter AND s.closing_kind='REMOVAL'
  AND((m.removed_at,m.removed_submicro) IS DISTINCT FROM(s.closed_at,s.closed_submicro) OR m.removal_operation_id IS DISTINCT FROM s.closing_operation_id))
 OR EXISTS(SELECT 1 FROM public.org_meters m WHERE m.org_id=NEW.org_id AND m.id=selected_meter AND m.removed_at IS NOT NULL
  AND(EXISTS(SELECT 1 FROM public.org_meter_segments s WHERE(s.org_id,s.meter_id)=(m.org_id,m.id) AND s.closed_at IS NULL)
  OR NOT EXISTS(SELECT 1 FROM public.org_meter_segments s WHERE(s.org_id,s.meter_id)=(m.org_id,m.id) AND s.closing_kind='REMOVAL')))
 OR EXISTS(SELECT 1 FROM public.org_meter_segments s WHERE s.org_id=NEW.org_id AND s.meter_id=selected_meter AND s.kind='RESET'
  AND NOT EXISTS(SELECT 1 FROM public.org_meter_readings r WHERE(r.org_id,r.meter_id,r.id,r.segment_id)=(s.org_id,s.meter_id,s.opening_reading_id,s.id)
  AND(r.occurred_at,r.occurred_submicro)=(s.opened_at,s.opened_submicro) AND r.origin='STANDALONE' AND r.operation_id=s.opened_operation_id))
 OR EXISTS(SELECT 1 FROM public.org_meter_segments s WHERE s.org_id=NEW.org_id AND s.meter_id=selected_meter AND s.kind='ROLLOVER'
  AND NOT EXISTS(SELECT 1 FROM public.org_meter_readings r JOIN public.org_meter_reading_states x ON(x.org_id,x.meter_id,x.reading_id)=(r.org_id,r.meter_id,r.id)
  WHERE(r.org_id,r.meter_id,r.id)=(s.org_id,s.meter_id,s.opening_reading_id) AND(r.occurred_at,r.occurred_submicro)=(s.opened_at,s.opened_submicro)
  AND x.cause='ROLLOVER' AND x.operation_id=s.opened_operation_id))
 OR EXISTS(SELECT 1 FROM public.org_meter_segments s JOIN public.org_meter_segments n ON(n.org_id,n.meter_id,n.opened_operation_id)=(s.org_id,s.meter_id,s.closing_operation_id)
  JOIN public.org_meter_readings p ON(p.org_id,p.meter_id,p.id)=(s.org_id,s.meter_id,s.closing_previous_reading_id)
  JOIN public.org_meter_readings w ON(w.org_id,w.meter_id,w.id)=(n.org_id,n.meter_id,n.opening_reading_id)
  WHERE s.org_id=NEW.org_id AND s.meter_id=selected_meter AND s.closing_kind='ROLLOVER' AND n.kind='ROLLOVER'
  AND(w.value>=p.value OR s.closing_implied_usage IS DISTINCT FROM(s.closing_value-p.value)+w.value))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='meter segment chain incomplete';END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_meter_segment_chain AFTER INSERT OR UPDATE ON org_meter_segments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_meter_chain_complete();
CREATE CONSTRAINT TRIGGER org_meter_removal_chain AFTER UPDATE ON org_meters DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_meter_chain_complete();

-- The 000216 guard with one clause added and three changed: a
-- POSSIBLE_ROLLOVER flag needs a meter with a modulus, CONFIRMED also refuses
-- that flag, ROLLOVER needs it and the ROLLOVER segment its own operation
-- opened on this reading, and a supersession compares the segments the two
-- readings count in (B-BR-EMP-481). Every other line stays.
CREATE OR REPLACE FUNCTION org_meter_reading_state_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
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
 IF 'POSSIBLE_ROLLOVER'=ANY(NEW.flags) AND NOT EXISTS(SELECT 1 FROM public.org_meters WHERE org_id=NEW.org_id AND id=NEW.meter_id AND modulus IS NOT NULL) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='rollover flag needs a modulus';END IF;
 IF NEW.cause='INTAKE' THEN
  IF NEW.state IS DISTINCT FROM r.state THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reading intake classification mismatch';END IF;
 ELSIF prior_state='VOIDED' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reading already voided';
 ELSIF prior_state='VALID' AND NEW.state<>'VOIDED' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='valid reading transition unavailable';
 ELSIF NEW.cause='CONFIRMED' AND prior_flags&&ARRAY['READING_BELOW_PREVIOUS','READING_ABOVE_NEXT','POSSIBLE_ROLLOVER'] THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='monotonicity flag cannot be confirmed';
 ELSIF NEW.cause='ROLLOVER' AND(NOT('POSSIBLE_ROLLOVER'=ANY(prior_flags)) OR NOT EXISTS(SELECT 1 FROM public.org_meter_segments s WHERE s.org_id=NEW.org_id AND s.meter_id=NEW.meter_id
 AND s.kind='ROLLOVER' AND s.opening_reading_id=NEW.reading_id AND s.opened_operation_id=NEW.operation_id)) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reading rollover unavailable';
 ELSIF NEW.cause='REEVALUATION' AND(NEW.state=prior_state AND NEW.flags=prior_flags OR('FUTURE_INSTANT'=ANY(prior_flags)) IS DISTINCT FROM('FUTURE_INSTANT'=ANY(NEW.flags))) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reading reevaluation unavailable';
 END IF;
 IF NEW.cause='SUPERSEDED' THEN
  SELECT * INTO replacement FROM public.org_meter_readings WHERE org_id=NEW.org_id AND meter_id=NEW.meter_id AND id=NEW.superseded_by;
  IF NOT FOUND OR replacement.operation_id<>NEW.operation_id OR public.org_meter_reading_segment(NEW.org_id,NEW.meter_id,replacement.id)
  IS DISTINCT FROM public.org_meter_reading_segment(NEW.org_id,NEW.meter_id,r.id) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='supersession source unavailable';END IF;
 END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
