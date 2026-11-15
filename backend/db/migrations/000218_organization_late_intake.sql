-- EMP-OFFL-009. Intake-only late delivery after an enrollment period's access
-- ended (EDR-197..199). One immutable, server-derived decision per late
-- acknowledgement; no runtime grants, no backfill, no applied migration edited.

-- EDR-197: the pure time decision, shared with Go DecideLate through
-- contracts/testdata/enterprise/sync/late-intake-v1.psv. Elapsed seconds, never
-- calendar days; the window includes its end instant (as ORG-009's grace). A
-- claim held before the end is server evidence and outranks the declaration.
CREATE FUNCTION org_sync_late_disposition(ended TIMESTAMPTZ,window_days INTEGER,declared TIMESTAMPTZ,claimed TIMESTAMPTZ,received TIMESTAMPTZ)
 RETURNS TEXT LANGUAGE sql STABLE SET search_path=pg_catalog AS $$
 SELECT CASE
  WHEN ended IS NULL OR window_days IS NULL OR declared IS NULL OR received IS NULL OR NOT isfinite(ended) OR NOT isfinite(received) OR NOT isfinite(declared)
   OR(claimed IS NOT NULL AND NOT isfinite(claimed)) OR window_days NOT BETWEEN 0 AND 60 OR received<ended THEN NULL
  WHEN received>ended+make_interval(secs=>window_days::double precision*86400) THEN 'CLOSED'
  WHEN claimed IS NOT NULL AND claimed<ended THEN 'HOLD_CLAIM'
  WHEN declared<ended THEN 'HOLD_DECLARED'
  ELSE 'ENDED' END
$$;

-- The exact late source: a period revoked ACCESS_ENDED, still owned by the same
-- existing account, whose key was never revoked LOST_OR_COMPROMISED in this
-- organization. The end instant is the membership end for a membership-sourced
-- revocation, else the revocation command's instant.
CREATE FUNCTION org_sync_late_end(org UUID,member UUID,enrollment UUID,owner UUID)
 RETURNS TABLE(end_source TEXT,ended_at TIMESTAMPTZ) LANGUAGE sql STABLE SET search_path=pg_catalog AS $$
 SELECT CASE e.source_type WHEN 'MEMBERSHIP' THEN 'MEMBERSHIP' ELSE 'DEVICE' END,
  CASE e.source_type WHEN 'MEMBERSHIP' THEN m.ended_at ELSE e.revoked_at END
 FROM public.org_device_enrollments e JOIN public.org_memberships m ON(m.org_id,m.id)=(e.org_id,e.membership_id)
 WHERE e.org_id=org AND e.id=enrollment AND e.membership_id=member AND m.account_id=owner AND m.access_model='EXISTING_ACCOUNT'
  AND e.state='REVOKED' AND e.revocation_class='ACCESS_ENDED'
  AND(e.source_type='COMMAND' OR(e.source_type='MEMBERSHIP' AND m.state='ENDED' AND m.ended_at IS NOT NULL AND m.ended_at<=e.revoked_at))
  AND NOT EXISTS(SELECT 1 FROM public.org_device_enrollments x WHERE x.org_id=e.org_id AND x.key_id=e.key_id AND x.state='REVOKED' AND x.revocation_class='LOST_OR_COMPROMISED')
$$;

-- The window in force at the end instant: the latest settings version effective
-- at or before it (ties by revision), or the 15-day default when none exists.
CREATE FUNCTION org_sync_late_window(org UUID,ended TIMESTAMPTZ)
 RETURNS TABLE(window_days INTEGER,settings_revision BIGINT) LANGUAGE sql STABLE SET search_path=pg_catalog AS $$
 SELECT COALESCE((SELECT(v.settings_values->'policies'->>'intake_window_days')::integer FROM public.org_settings_versions v WHERE v.org_id=org AND v.effective_at<=ended ORDER BY v.effective_at DESC,v.revision DESC LIMIT 1),15),
  (SELECT v.revision FROM public.org_settings_versions v WHERE v.org_id=org AND v.effective_at<=ended ORDER BY v.effective_at DESC,v.revision DESC LIMIT 1)
$$;

CREATE TABLE org_sync_late_intakes(
 org_id UUID NOT NULL,operation_id UUID NOT NULL,membership_id UUID NOT NULL,enrollment_id UUID NOT NULL,
 end_source TEXT NOT NULL CHECK(end_source IN('MEMBERSHIP','DEVICE')),
 ended_at TIMESTAMPTZ NOT NULL CHECK(isfinite(ended_at)),
 window_days SMALLINT NOT NULL CHECK(window_days BETWEEN 0 AND 60),settings_revision BIGINT,
 declared_at TIMESTAMPTZ NOT NULL CHECK(isfinite(declared_at)),declared_submicro SMALLINT NOT NULL CHECK(declared_submicro BETWEEN 0 AND 999),
 received_at TIMESTAMPTZ NOT NULL CHECK(isfinite(received_at) AND received_at>'0001-01-01 00:00:00+00'::timestamptz AND received_at<'10000-01-01 00:00:00+00'::timestamptz),
 disposition TEXT NOT NULL CHECK(disposition IN('HOLD_CLAIM','HOLD_DECLARED','ENDED','CLOSED')),
 result TEXT NOT NULL,code TEXT NOT NULL,
 policy TEXT NOT NULL DEFAULT 'late-intake-v1' CHECK(policy='late-intake-v1'),
 PRIMARY KEY(org_id,operation_id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id),
 FOREIGN KEY(org_id,settings_revision) REFERENCES org_settings_versions(org_id,revision),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_acknowledgements(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((disposition IN('HOLD_CLAIM','HOLD_DECLARED') AND((result='ACCEPTED_FOR_REVIEW' AND code='ACCESS_ENDED_BEFORE_RECEIPT') OR(result='REJECTED' AND code='MALFORMED')))
  OR(disposition='ENDED' AND result='REJECTED' AND((end_source='MEMBERSHIP' AND code='MEMBERSHIP_ENDED') OR(end_source='DEVICE' AND code='DEVICE_REVOKED')))
  OR(disposition='CLOSED' AND result='REJECTED' AND code='INTAKE_WINDOW_CLOSED'))
);
ALTER TABLE org_sync_late_intakes ENABLE ROW LEVEL SECURITY;ALTER TABLE org_sync_late_intakes FORCE ROW LEVEL SECURITY;
CREATE POLICY org_sync_late_intakes_scope ON org_sync_late_intakes USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_sync_late_intakes_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_sync_late_intakes FOR EACH STATEMENT EXECUTE FUNCTION org_sync_keep_history();
REVOKE ALL ON org_sync_late_intakes FROM PUBLIC;

-- The database re-derives the end, the window and the disposition from history.
CREATE FUNCTION org_sync_late_intake_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE o public.org_sync_operations%ROWTYPE;src TEXT;ended TIMESTAMPTZ;w INTEGER;revision BIGINT;decided TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 -- Serialize with device revocations (their guard locks the same root).
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO o FROM public.org_sync_operations WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
 IF NOT FOUND OR o.membership_id<>NEW.membership_id OR o.enrollment_id<>NEW.enrollment_id THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='late intake source unavailable';END IF;
 SELECT l.end_source,l.ended_at INTO src,ended FROM public.org_sync_late_end(NEW.org_id,o.membership_id,o.enrollment_id,o.owner_account_id) l;
 IF NOT FOUND OR src IS DISTINCT FROM NEW.end_source OR ended IS DISTINCT FROM NEW.ended_at THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='late intake source unavailable';END IF;
 SELECT x.window_days,x.settings_revision INTO w,revision FROM public.org_sync_late_window(NEW.org_id,ended) x;
 IF w IS DISTINCT FROM NEW.window_days::integer OR revision IS DISTINCT FROM NEW.settings_revision THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='late intake window unavailable';END IF;
 IF NEW.received_at>clock_timestamp() THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='late intake receipt unavailable';END IF;
 decided=public.org_sync_late_disposition(ended,w,NEW.declared_at,o.claimed_at,NEW.received_at);
 IF decided IS NULL OR decided<>NEW.disposition THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='late intake decision unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_sync_late_intake_source BEFORE INSERT ON org_sync_late_intakes FOR EACH ROW EXECUTE FUNCTION org_sync_late_intake_source();

-- At commit the decision is published with exactly its final acknowledgement,
-- a real review item when held, and never an applied effect.
CREATE FUNCTION org_sync_late_intake_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NOT EXISTS(SELECT 1 FROM public.org_sync_acknowledgements a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.result=NEW.result AND a.code=NEW.code AND a.received_at=NEW.received_at AND a.entity_revision IS NULL)
 OR(NEW.result='ACCEPTED_FOR_REVIEW' AND NOT EXISTS(SELECT 1 FROM public.org_sync_review_items i WHERE i.org_id=NEW.org_id AND i.operation_id=NEW.operation_id AND i.code=NEW.code))
 OR EXISTS(SELECT 1 FROM public.org_sync_effects e WHERE e.org_id=NEW.org_id AND e.operation_id=NEW.operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='late intake publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_sync_late_intake_complete AFTER INSERT ON org_sync_late_intakes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_late_intake_complete();

-- Late codes exist only with their recorded decision. The late table is read
-- only inside this branch, so ordinary acknowledgements need no new privilege.
CREATE FUNCTION org_sync_ack_late_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 IF NEW.code IN('ACCESS_ENDED_BEFORE_RECEIPT','MEMBERSHIP_ENDED','INTAKE_WINDOW_CLOSED','DEVICE_REVOKED') THEN
  prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
  IF NOT EXISTS(SELECT 1 FROM public.org_sync_late_intakes l WHERE l.org_id=NEW.org_id AND l.operation_id=NEW.operation_id AND l.result=NEW.result AND l.code=NEW.code AND l.received_at=NEW.received_at)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='late intake acknowledgement source unavailable';END IF;
  PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_sync_ack_late_source AFTER INSERT ON org_sync_acknowledgements DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_ack_late_source();

-- The 201 held-meter source guard, unchanged for every current-source code,
-- plus one late branch: ACCESS_ENDED_BEFORE_RECEIPT needs the exact late source
-- and a declaration or an earlier claim before its end instant.
CREATE OR REPLACE FUNCTION org_meter_held_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE o public.org_sync_operations%ROWTYPE;i public.org_sync_review_items%ROWTYPE;m public.org_meters%ROWTYPE;s public.org_meter_segments%ROWTYPE;actual_unit UUID;disposal_at TIMESTAMPTZ;disposal_sub SMALLINT;late_end TIMESTAMPTZ;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND OR i.operation_type<>'meter.reading.record' OR i.operation_schema<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held meter intent unavailable';END IF;
 SELECT * INTO o FROM public.org_sync_operations WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
 IF NOT FOUND OR o.entity_type<>'meter_reading' OR o.entity_id<>NEW.reading_id THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held meter source unavailable';END IF;
 IF i.code='ACCESS_ENDED_BEFORE_RECEIPT' THEN
  SELECT l.ended_at INTO late_end FROM public.org_sync_late_end(o.org_id,o.membership_id,o.enrollment_id,o.owner_account_id) l;
  IF NOT FOUND OR late_end IS NULL OR NOT EXISTS(SELECT 1 FROM public.org_memberships p WHERE p.org_id=o.org_id AND p.id=o.membership_id AND p.employee_id=i.author_employee_id)
   OR NOT(o.claimed_at<late_end OR NEW.occurred_at<late_end)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held meter source unavailable';END IF;
 ELSIF NOT EXISTS(SELECT 1 FROM public.org_device_enrollments e JOIN public.org_memberships p ON(p.org_id,p.id)=(e.org_id,e.membership_id) WHERE e.org_id=o.org_id AND e.id=o.enrollment_id AND e.membership_id=o.membership_id AND e.state='ENROLLED' AND p.state='ACTIVE' AND p.account_id=o.owner_account_id AND p.employee_id=i.author_employee_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held meter source unavailable';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 -- The held fact reserves its reading identity; it cannot claim an applied one.
 IF EXISTS(SELECT 1 FROM public.org_meter_readings WHERE org_id=NEW.org_id AND(id=NEW.reading_id OR operation_id=NEW.operation_id)) THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_meter_readings_pkey',MESSAGE='meter reading identity unavailable';END IF;
 SELECT unit_id,disposed_at,disposed_submicro INTO actual_unit,disposal_at,disposal_sub FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
 IF NOT FOUND OR NEW.unit_id IS DISTINCT FROM actual_unit THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held meter context unavailable';END IF;
 IF i.code='ASSET_UNAVAILABLE' AND(disposal_at IS NULL OR(NEW.occurred_at,NEW.occurred_submicro)<(disposal_at,disposal_sub)) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held asset unavailability unproven';END IF;
 SELECT * INTO m FROM public.org_meters WHERE org_id=NEW.org_id AND id=NEW.meter_id AND asset_id=NEW.asset_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private meter not found';END IF;
 SELECT * INTO s FROM public.org_meter_segments WHERE org_id=NEW.org_id AND id=NEW.segment_id AND meter_id=NEW.meter_id;
 IF NOT FOUND OR(NEW.occurred_at,NEW.occurred_submicro)<(m.installed_at,m.installed_submicro) OR(m.removed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(m.removed_at,m.removed_submicro))
 OR(NEW.occurred_at,NEW.occurred_submicro)<(s.opened_at,s.opened_submicro) OR(s.closed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(s.closed_at,s.closed_submicro))
 OR NEW.value>(CASE m.kind WHEN 'ODOMETER' THEN 1000000000000 ELSE 3600000000 END) OR(m.modulus IS NOT NULL AND NEW.value>=m.modulus)
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid held meter observation';END IF;
 RETURN NEW;
END $$;
