-- EMP-TRIP-015. Late and duplicate trip facts and amendments after closure
-- (B-BR-EMP-363..368, EDR-234..237). Additive: the review history admits the
-- author's RESUBMIT of a returned field revision and a company AMEND that
-- opens a reviewed amendment revision; amendment content (corrected start or
-- end readings) is an insert-only record; a stale or duplicate trip.submit
-- and a logbook event that arrives after the trip's submission are kept as
-- OFFL-008 held facts (STALE_REVISION), never applied to a reviewed or
-- approved revision. An approved amendment supersedes exactly its readings
-- in the decision transaction. No key references an asset, unit or cost
-- centre, so no usage source changes; amendment readings reference
-- org_meter_readings, which no usage source inspects.
ALTER TABLE org_trip_review_entries DROP CONSTRAINT org_trip_review_entries_kind_check,
 ADD CONSTRAINT org_trip_review_entries_kind_check CHECK(kind IN('SUBMIT','RESUBMIT','AMEND','OPEN','APPROVE','RETURN','REJECT')),
 DROP CONSTRAINT org_trip_review_entries_state,
 ADD CONSTRAINT org_trip_review_entries_state CHECK((kind IN('SUBMIT','RESUBMIT','AMEND') AND state='SUBMITTED') OR(kind='OPEN' AND state='IN_REVIEW')
  OR(kind='APPROVE' AND state='APPROVED') OR(kind='RETURN' AND state='RETURNED') OR(kind='REJECT' AND state='REJECTED')),
 DROP CONSTRAINT org_trip_review_entries_reason,
 -- B-BR-EMP-163 reason policy; an amendment always states its reason (BR-AMD-001).
 ADD CONSTRAINT org_trip_review_entries_reason CHECK(CASE WHEN kind IN('SUBMIT','RESUBMIT','OPEN') THEN reason=''
  WHEN kind='APPROVE' THEN reason='' OR org_asset_text_valid(reason,280,true) ELSE org_asset_text_valid(reason,280,true) END);
-- One submission-producing entry per revision.
CREATE UNIQUE INDEX org_trip_review_entries_one_revision ON org_trip_review_entries(org_id,trip_id,revision) WHERE kind IN('SUBMIT','RESUBMIT','AMEND');

-- The content of one amendment revision: each corrected reading of the
-- effective revision, its value there, the corrected value and the reading
-- that supersedes it once the amendment is approved.
CREATE TABLE org_trip_amendments(
 org_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 revision BIGINT NOT NULL CHECK(revision BETWEEN 2 AND 1000000),
 base_revision BIGINT NOT NULL CHECK(base_revision BETWEEN 1 AND 999999),
 intent TEXT NOT NULL CHECK(intent IN('AMEND','EDIT')),
 start_meter_id UUID,start_reading_id UUID,start_from BIGINT CHECK(start_from>=0),start_to BIGINT CHECK(start_to>=0),start_new_reading_id UUID,
 end_meter_id UUID,end_reading_id UUID,end_from BIGINT CHECK(end_from>=0),end_to BIGINT CHECK(end_to>=0),end_new_reading_id UUID,
 actor_membership_id UUID NOT NULL,
 actor_employee_id UUID NOT NULL,
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,trip_id,revision),
 CONSTRAINT org_trip_amendments_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_amendments_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_trip_amendments_actor_fkey FOREIGN KEY(org_id,actor_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_amendments_actor_employee_fkey FOREIGN KEY(org_id,actor_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_trip_amendments_start_fkey FOREIGN KEY(org_id,start_meter_id,start_reading_id) REFERENCES org_meter_readings(org_id,meter_id,id),
 CONSTRAINT org_trip_amendments_end_fkey FOREIGN KEY(org_id,end_meter_id,end_reading_id) REFERENCES org_meter_readings(org_id,meter_id,id),
 CONSTRAINT org_trip_amendments_points CHECK(num_nonnulls(start_meter_id,start_reading_id,start_from,start_to,start_new_reading_id) IN(0,5)
  AND num_nonnulls(end_meter_id,end_reading_id,end_from,end_to,end_new_reading_id) IN(0,5)
  AND(start_reading_id IS NOT NULL OR end_reading_id IS NOT NULL)
  AND(start_reading_id IS NULL OR start_to<>start_from) AND(end_reading_id IS NULL OR end_to<>end_from)
  AND start_new_reading_id IS DISTINCT FROM end_new_reading_id
  AND start_new_reading_id IS DISTINCT FROM '00000000-0000-0000-0000-000000000000'::uuid AND end_new_reading_id IS DISTINCT FROM '00000000-0000-0000-0000-000000000000'::uuid),
 CONSTRAINT org_trip_amendments_lineage CHECK(base_revision<revision AND(intent='AMEND' OR base_revision=revision-1)),
 CHECK(isfinite(recorded_at))
);

-- The current reading of a trip point in its effective revision: the reading
-- that the latest approved amendment of that point created, else the
-- reading the start or finish recorded.
CREATE FUNCTION org_trip_point_reading(p_org UUID,p_trip UUID,p_point TEXT) RETURNS UUID LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT COALESCE((SELECT CASE p_point WHEN 'START' THEN a.start_new_reading_id ELSE a.end_new_reading_id END FROM public.org_trip_amendments a
  WHERE a.org_id=p_org AND a.trip_id=p_trip AND(CASE p_point WHEN 'START' THEN a.start_new_reading_id ELSE a.end_new_reading_id END) IS NOT NULL
  AND EXISTS(SELECT 1 FROM public.org_trip_review_entries e WHERE e.org_id=a.org_id AND e.trip_id=a.trip_id AND e.revision=a.revision AND e.kind='APPROVE')
  ORDER BY a.revision DESC LIMIT 1),
  CASE p_point WHEN 'START' THEN(SELECT t.reading_id FROM public.org_trips t WHERE t.org_id=p_org AND t.id=p_trip)
  ELSE(SELECT f.reading_id FROM public.org_trip_finishes f WHERE f.org_id=p_org AND f.trip_id=p_trip) END)
 $$;

-- True when the trip's latest revision is under review or decided: a field
-- facts change applies only while the case is DRAFT or a returned field
-- revision (BR-AMD-006).
CREATE FUNCTION org_trip_revision_closed(p_org UUID,p_trip UUID) RETURNS BOOLEAN LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_trip_review_entries e WHERE e.org_id=p_org AND e.trip_id=p_trip)
 AND(COALESCE((SELECT e.state FROM public.org_trip_review_entries e WHERE e.org_id=p_org AND e.trip_id=p_trip ORDER BY e.sequence DESC LIMIT 1),'')<>'RETURNED'
  OR EXISTS(SELECT 1 FROM public.org_trip_review_entries e WHERE e.org_id=p_org AND e.trip_id=p_trip AND e.kind='AMEND'))
 $$;

-- An approved amendment's reading: the original voided by supersession to
-- the new reading of the same origin, segment and declaration with the
-- corrected value, all in the decision operation (METER-004 path).
CREATE FUNCTION org_trip_superseded(p_org UUID,p_meter UUID,p_reading UUID,p_new UUID,p_value BIGINT,p_op UUID) RETURNS BOOLEAN LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_meter_reading_states s WHERE s.org_id=p_org AND s.meter_id=p_meter AND s.reading_id=p_reading
  AND s.state='VOIDED' AND s.cause='SUPERSEDED' AND s.superseded_by=p_new AND s.operation_id=p_op)
 AND EXISTS(SELECT 1 FROM public.org_meter_readings n JOIN public.org_meter_readings o ON(o.org_id,o.meter_id,o.id)=(n.org_id,n.meter_id,p_reading)
  WHERE n.org_id=p_org AND n.meter_id=p_meter AND n.id=p_new AND n.value=p_value AND n.operation_id=p_op AND n.origin=o.origin AND n.segment_id=o.segment_id
  AND(n.occurred_at,n.occurred_submicro)=(o.occurred_at,o.occurred_submicro))
 $$;

-- B-BR-EMP-365/368: an amendment of a COMPLETED trip with an approved
-- revision, under the organization root and trip row locks: AMEND from the
-- approved or a rejected amendment (base = effective), EDIT of a returned
-- amendment (base = that revision); the next revision; an active author;
-- each corrected reading is the point's current reading of the effective
-- revision with its stored value; the month is not LOCKED.
CREATE FUNCTION org_trip_amendment_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;last_entry public.org_trip_review_entries%ROWTYPE;effective BIGINT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF t.state<>'COMPLETED' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip amendment needs a completed trip';END IF;
 SELECT * INTO last_entry FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id ORDER BY sequence DESC LIMIT 1;
 SELECT max(revision) INTO effective FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id AND kind='APPROVE';
 IF last_entry.sequence IS NULL OR effective IS NULL OR NEW.revision<>last_entry.revision+1
 OR NOT((NEW.intent='AMEND' AND NEW.base_revision=effective AND(last_entry.state='APPROVED' OR(last_entry.state='REJECTED' AND last_entry.revision>effective)))
  OR(NEW.intent='EDIT' AND NEW.base_revision=last_entry.revision AND last_entry.state='RETURNED' AND last_entry.revision>effective))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid trip amendment lineage';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
  WHERE m.org_id=NEW.org_id AND m.id=NEW.actor_membership_id AND m.employee_id=NEW.actor_employee_id AND m.state='ACTIVE' AND e.state='ACTIVE' AND e.merged_into IS NULL)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip amendment author unavailable';END IF;
 IF(NEW.start_reading_id IS NOT NULL AND(NEW.start_reading_id IS DISTINCT FROM public.org_trip_point_reading(NEW.org_id,NEW.trip_id,'START')
  OR NOT EXISTS(SELECT 1 FROM public.org_meter_readings r WHERE r.org_id=NEW.org_id AND r.meter_id=NEW.start_meter_id AND r.id=NEW.start_reading_id AND r.value=NEW.start_from)
  OR EXISTS(SELECT 1 FROM public.org_meter_reading_states s WHERE s.org_id=NEW.org_id AND s.meter_id=NEW.start_meter_id AND s.reading_id=NEW.start_reading_id AND s.state='VOIDED')
  OR EXISTS(SELECT 1 FROM public.org_meter_readings r WHERE r.org_id=NEW.org_id AND r.id=NEW.start_new_reading_id)))
 OR(NEW.end_reading_id IS NOT NULL AND(NEW.end_reading_id IS DISTINCT FROM public.org_trip_point_reading(NEW.org_id,NEW.trip_id,'END')
  OR NOT EXISTS(SELECT 1 FROM public.org_meter_readings r WHERE r.org_id=NEW.org_id AND r.meter_id=NEW.end_meter_id AND r.id=NEW.end_reading_id AND r.value=NEW.end_from)
  OR EXISTS(SELECT 1 FROM public.org_meter_reading_states s WHERE s.org_id=NEW.org_id AND s.meter_id=NEW.end_meter_id AND s.reading_id=NEW.end_reading_id AND s.state='VOIDED')
  OR EXISTS(SELECT 1 FROM public.org_meter_readings r WHERE r.org_id=NEW.org_id AND r.id=NEW.end_new_reading_id)))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip amendment reading unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_finance_periods p WHERE p.org_id=NEW.org_id AND p.period_month=date_trunc('month',t.reporting_day)::date AND p.state='LOCKED')
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='trip reporting period locked';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_amendment_source BEFORE INSERT ON org_trip_amendments FOR EACH ROW EXECUTE FUNCTION org_trip_amendment_source();

-- At commit an amendment is published only beside its AMEND entry of the
-- same revision, operation and author.
CREATE FUNCTION org_trip_amendment_published() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NOT EXISTS(SELECT 1 FROM public.org_trip_review_entries e WHERE e.org_id=NEW.org_id AND e.trip_id=NEW.trip_id AND e.revision=NEW.revision AND e.kind='AMEND'
  AND e.operation_id=NEW.operation_id AND e.actor_membership_id=NEW.actor_membership_id AND e.actor_employee_id=NEW.actor_employee_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip amendment publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_trip_amendment_published AFTER INSERT ON org_trip_amendments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_trip_amendment_published();

-- OFFL-008 held submissions (STALE_REVISION): the author's trip.submit that
-- names a revision which is no longer the current draft or returned field
-- revision. The whole intent is kept; the review case is never changed.
CREATE TABLE org_trip_held_submissions(
 org_id UUID NOT NULL,review_item_id UUID NOT NULL,operation_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 revision BIGINT NOT NULL CHECK(revision BETWEEN 1 AND 1000000),
 occurred_at TIMESTAMPTZ NOT NULL,occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 PRIMARY KEY(org_id,review_item_id),UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_held_submissions_review_fkey FOREIGN KEY(org_id,review_item_id,operation_id) REFERENCES org_sync_review_items(org_id,id,operation_id),
 CONSTRAINT org_trip_held_submissions_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz)
);
CREATE FUNCTION org_trip_held_submission_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;last_entry public.org_trip_review_entries%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip submission intent unavailable';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF i.operation_type<>'trip.submit' OR i.operation_schema<>1 OR i.code<>'STALE_REVISION' OR t.state<>'COMPLETED' OR i.author_employee_id<>t.author_employee_id
 OR NOT EXISTS(SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
  JOIN public.org_device_enrollments e ON(e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
  WHERE o.org_id=NEW.org_id AND o.operation_id=NEW.operation_id AND o.operation_type='trip.submit' AND o.operation_schema=1 AND o.entity_type='trip_review'
  AND o.entity_id=NEW.trip_id AND o.payload_digest=i.payload_digest AND o.membership_id=t.author_membership_id AND m.employee_id=t.author_employee_id
  AND m.account_id=o.owner_account_id AND m.state='ACTIVE' AND e.state='ENROLLED')
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 OR EXISTS(SELECT 1 FROM public.org_trip_review_entries x WHERE x.org_id=NEW.org_id AND x.operation_id=NEW.operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip submission source unavailable';END IF;
 SELECT * INTO last_entry FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id ORDER BY sequence DESC LIMIT 1;
 IF(last_entry.sequence IS NULL AND NEW.revision=1)
 OR(last_entry.state='RETURNED' AND NEW.revision=last_entry.revision AND NOT EXISTS(SELECT 1 FROM public.org_trip_review_entries x WHERE x.org_id=NEW.org_id AND x.trip_id=NEW.trip_id AND x.kind IN('AMEND','APPROVE')))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip submission condition unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_trip_held_submission_source BEFORE INSERT ON org_trip_held_submissions FOR EACH ROW EXECUTE FUNCTION org_trip_held_submission_source();
CREATE CONSTRAINT TRIGGER org_trip_held_submission_complete AFTER INSERT ON org_trip_held_submissions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_review_complete();

-- The 228 review-entry source with the EMP-TRIP-015 transitions (RESUBMIT of
-- a returned never-approved field revision by its author; AMEND beside its
-- content row), the reviewer's separation from an amendment's author and
-- the period lock on AMEND; every other check unchanged.
CREATE OR REPLACE FUNCTION org_trip_review_entry_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;last_entry public.org_trip_review_entries%ROWTYPE;actor_person UUID;author_person UUID;driver_person UUID;
 amended BOOLEAN;effective BIGINT;amendment public.org_trip_review_entries%ROWTYPE;amendment_person UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF t.state<>'COMPLETED' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip review needs a completed trip';END IF;
 SELECT * INTO last_entry FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id ORDER BY sequence DESC LIMIT 1;
 amended=EXISTS(SELECT 1 FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id AND kind='AMEND');
 SELECT max(revision) INTO effective FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id AND kind='APPROVE';
 -- EMP-TRIP-015: the author resubmits a returned field revision (revision
 -- + 1); an amendment opens the next revision after an approval, a
 -- rejected amendment or a returned amendment, with its content row.
 IF NOT((last_entry.sequence IS NULL AND NEW.sequence=1 AND NEW.kind='SUBMIT' AND NEW.revision=1)
 OR(last_entry.sequence IS NOT NULL AND NEW.sequence=last_entry.sequence+1 AND(
  (NEW.revision=last_entry.revision AND((last_entry.state='SUBMITTED' AND NEW.kind='OPEN') OR(last_entry.state='IN_REVIEW' AND NEW.kind IN('APPROVE','RETURN','REJECT'))))
  OR(NEW.kind='RESUBMIT' AND NEW.revision=last_entry.revision+1 AND last_entry.state='RETURNED' AND NOT amended AND effective IS NULL)
  OR(NEW.kind='AMEND' AND NEW.revision=last_entry.revision+1 AND effective IS NOT NULL
   AND(last_entry.state='APPROVED' OR(last_entry.state IN('REJECTED','RETURNED') AND last_entry.revision>effective))
   AND EXISTS(SELECT 1 FROM public.org_trip_amendments a WHERE a.org_id=NEW.org_id AND a.trip_id=NEW.trip_id AND a.revision=NEW.revision
    AND a.operation_id=NEW.operation_id AND a.actor_membership_id=NEW.actor_membership_id AND a.actor_employee_id=NEW.actor_employee_id)))))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid trip review transition';END IF;
 IF NEW.kind IN('SUBMIT','RESUBMIT') THEN
  IF NEW.actor_membership_id<>t.author_membership_id OR NEW.actor_employee_id<>t.author_employee_id OR NOT EXISTS(
   SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
   JOIN public.org_device_enrollments e ON(e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
   WHERE o.org_id=NEW.org_id AND o.operation_id=NEW.operation_id AND o.operation_type='trip.submit' AND o.operation_schema=1 AND o.entity_type='trip_review'
   AND o.entity_id=NEW.trip_id AND o.membership_id=t.author_membership_id AND m.employee_id=t.author_employee_id AND m.account_id=o.owner_account_id
   AND m.state='ACTIVE' AND e.state='ENROLLED')
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip submission source unavailable';END IF;
 ELSIF NEW.kind='AMEND' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
   WHERE m.org_id=NEW.org_id AND m.id=NEW.actor_membership_id AND m.employee_id=NEW.actor_employee_id AND m.state='ACTIVE' AND e.state='ACTIVE' AND e.merged_into IS NULL)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip amendment author unavailable';END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
   WHERE m.org_id=NEW.org_id AND m.id=NEW.actor_membership_id AND m.employee_id=NEW.actor_employee_id AND m.state='ACTIVE' AND e.state='ACTIVE' AND e.merged_into IS NULL)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip reviewer unavailable';END IF;
  actor_person=public.org_sync_review_employee_canonical(NEW.org_id,NEW.actor_employee_id);
  author_person=public.org_sync_review_employee_canonical(t.org_id,t.author_employee_id);
  driver_person=public.org_sync_review_employee_canonical(t.org_id,t.driver_employee_id);
  IF actor_person IS NULL OR author_person IS NULL OR driver_person IS NULL OR actor_person IN(author_person,driver_person)
  OR NEW.actor_membership_id=t.author_membership_id OR EXISTS(SELECT 1 FROM public.org_memberships a JOIN public.org_memberships r ON(r.org_id,r.account_id)=(a.org_id,a.account_id)
   WHERE a.org_id=NEW.org_id AND a.id=t.author_membership_id AND r.id=NEW.actor_membership_id)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip self approval refused';END IF;
  -- BR-REV-003 for an amendment revision: its author never reviews it.
  SELECT * INTO amendment FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id AND kind='AMEND' AND revision=NEW.revision;
  IF FOUND THEN
   amendment_person=public.org_sync_review_employee_canonical(NEW.org_id,amendment.actor_employee_id);
   IF amendment_person IS NULL OR actor_person=amendment_person OR NEW.actor_membership_id=amendment.actor_membership_id
   OR EXISTS(SELECT 1 FROM public.org_memberships a JOIN public.org_memberships r ON(r.org_id,r.account_id)=(a.org_id,a.account_id)
    WHERE a.org_id=NEW.org_id AND a.id=amendment.actor_membership_id AND r.id=NEW.actor_membership_id)
   THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip self approval refused';END IF;
  END IF;
 END IF;
 IF NEW.kind IN('OPEN','APPROVE','RETURN','REJECT','AMEND') AND EXISTS(SELECT 1 FROM public.org_finance_periods p
  WHERE p.org_id=NEW.org_id AND p.period_month=date_trunc('month',t.reporting_day)::date AND p.state='LOCKED')
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='trip reporting period locked';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;

-- The 226 publication with RESUBMIT (its ACK at the entry revision), AMEND
-- (its content row and trip.trip.amended audit event) and, for an approved
-- amendment, the supersession of exactly its readings in the decision.
CREATE OR REPLACE FUNCTION org_trip_review_published() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE r public.org_trip_review_entries%ROWTYPE;decision public.org_trip_review_entries%ROWTYPE;prior_org TEXT;am public.org_trip_amendments%ROWTYPE;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 SELECT * INTO r FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id AND sequence=NEW.sequence;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip review publication unavailable';END IF;
 IF r.kind IN('SUBMIT','RESUBMIT') THEN
  IF NOT public.org_trip_published(r.org_id,r.operation_id,r.revision) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip submission publication unavailable';END IF;
 ELSIF r.kind='AMEND' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_trip_amendments a WHERE a.org_id=r.org_id AND a.trip_id=r.trip_id AND a.revision=r.revision AND a.operation_id=r.operation_id)
  OR NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=r.org_id AND a.operation_id=r.operation_id AND a.target_type='trip' AND a.target_id=r.trip_id
   AND a.actor_id=(SELECT account_id FROM public.org_memberships m WHERE m.org_id=r.org_id AND m.id=r.actor_membership_id) AND a.action='trip.trip.amended')
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip amendment publication unavailable';END IF;
 ELSE
  decision=r;
  IF r.kind='OPEN' THEN
   SELECT * INTO decision FROM public.org_trip_review_entries WHERE org_id=r.org_id AND trip_id=r.trip_id AND sequence=r.sequence+1 AND operation_id=r.operation_id AND kind IN('APPROVE','RETURN','REJECT');
   IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip review opening without decision';END IF;
  END IF;
  IF NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=decision.org_id AND a.operation_id=decision.operation_id AND a.target_type='trip' AND a.target_id=decision.trip_id
   AND a.actor_id=(SELECT account_id FROM public.org_memberships m WHERE m.org_id=decision.org_id AND m.id=decision.actor_membership_id)
   AND a.action=CASE decision.kind WHEN 'APPROVE' THEN 'trip.trip.approved' WHEN 'RETURN' THEN 'trip.trip.returned' ELSE 'trip.trip.rejected' END)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip decision publication unavailable';END IF;
  -- An approved amendment superseded exactly its readings in this operation.
  IF decision.kind='APPROVE' THEN
   SELECT * INTO am FROM public.org_trip_amendments WHERE org_id=decision.org_id AND trip_id=decision.trip_id AND revision=decision.revision;
   IF FOUND AND((am.start_reading_id IS NOT NULL AND NOT public.org_trip_superseded(am.org_id,am.start_meter_id,am.start_reading_id,am.start_new_reading_id,am.start_to,decision.operation_id))
   OR(am.end_reading_id IS NOT NULL AND NOT public.org_trip_superseded(am.org_id,am.end_meter_id,am.end_reading_id,am.end_new_reading_id,am.end_to,decision.operation_id)))
   THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip amendment readings unavailable';END IF;
  END IF;
 END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;

-- The 225 logbook source unchanged except one clause: a submitted, decided
-- or amended revision takes no direct fact (B-BR-EMP-364).
CREATE OR REPLACE FUNCTION org_trip_event_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;f public.org_trip_finishes%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NOT public.org_trip_event_operation_valid(NEW.org_id,NEW.operation_id,NEW.id,NEW.payload_digest,NEW.author_membership_id,NEW.enrollment_id,t.author_employee_id)
 OR NEW.author_membership_id<>t.author_membership_id OR NEW.author_employee_id<>t.author_employee_id
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip event source unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_trip_held_events h WHERE h.org_id=NEW.org_id AND h.id=NEW.id)
 THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_trip_events_pkey',MESSAGE='trip event identity unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid trip event';END IF;
 SELECT * INTO f FROM public.org_trip_finishes WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id;
 IF FOUND AND(NEW.occurred_at,NEW.occurred_submicro)>(f.occurred_at,f.occurred_submicro)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip already closed';END IF;
 -- EMP-TRIP-015: a submitted, decided or amended revision takes no direct fact.
 IF public.org_trip_revision_closed(NEW.org_id,NEW.trip_id) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip revision closed';END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;

-- The 225 held-event source with STALE_REVISION (an event inside the trip
-- whose revision is submitted, decided or amended); TRIP_ALREADY_CLOSED
-- unchanged.
CREATE OR REPLACE FUNCTION org_trip_held_event_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;f public.org_trip_finishes%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip event intent unavailable';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO f FROM public.org_trip_finishes WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id;
 IF i.operation_type<>'trip.event.record' OR i.operation_schema<>1 OR i.code NOT IN('TRIP_ALREADY_CLOSED','STALE_REVISION') OR t.state<>'COMPLETED' OR NOT FOUND
 OR NOT public.org_trip_event_operation_valid(NEW.org_id,NEW.operation_id,NEW.id,i.payload_digest,t.author_membership_id,NULL,t.author_employee_id)
 OR i.author_employee_id<>t.author_employee_id
 OR(i.code='TRIP_ALREADY_CLOSED' AND(NEW.occurred_at,NEW.occurred_submicro)<=(f.occurred_at,f.occurred_submicro))
 -- EMP-TRIP-015: within the trip, but its revision is submitted, decided or amended.
 OR(i.code='STALE_REVISION' AND((NEW.occurred_at,NEW.occurred_submicro)>(f.occurred_at,f.occurred_submicro)
  OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro) OR NOT public.org_trip_revision_closed(NEW.org_id,NEW.trip_id)))
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip event source unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_trip_events e WHERE e.org_id=NEW.org_id AND(e.id=NEW.id OR e.operation_id=NEW.operation_id))
 THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_trip_events_pkey',MESSAGE='trip event identity unavailable';END IF;
 RETURN NEW;
END $$;

-- The 225 review-source hook with the trip.submit owner (held submissions);
-- the meter, trip.start, trip.finish and trip.event.record branches are
-- unchanged and unknown owners still fail closed.
CREATE OR REPLACE FUNCTION org_sync_review_source_valid(org UUID,review UUID,owner_type TEXT,owner_schema BIGINT) RETURNS BOOLEAN
 LANGUAGE SQL VOLATILE SET search_path=pg_catalog AS $$
 SELECT(owner_type='meter.reading.record' AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_meter_held_readings h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='meter_reading' AND o.entity_id=h.reading_id))
 OR(owner_type='trip.start' AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_trip_held_starts h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='trip' AND o.entity_id=h.trip_id))
 OR(owner_type='trip.finish' AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_trip_held_finishes h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='trip' AND o.entity_id=h.trip_id))
 OR(owner_type='trip.event.record' AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_trip_held_events h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='trip_event' AND o.entity_id=h.id))
 OR(owner_type='trip.submit' AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_trip_held_submissions h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='trip_review' AND o.entity_id=h.trip_id))
 $$;

DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_trip_amendments','org_trip_held_submissions'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history()',name||'_immutable',name);
 EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;
