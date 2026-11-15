-- EMP-TRIP-004. Submission and administrative review of a completed trip
-- (B-BR-EMP-342..347, EDR-220..223). One append-only APPR-001 history per
-- trip: SUBMIT through the signed trip.submit operation of its author, then
-- OPEN and one decision (APPROVE, RETURN, REJECT) per revision by an
-- independent reviewer through the company route. The current review state
-- is the latest entry (DRAFT revision 1 when there is none, EDR-205); it is
-- never stored elsewhere. No key references an asset, unit or cost centre,
-- so no usage source changes.
CREATE TABLE org_trip_review_entries(
 org_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 sequence BIGINT NOT NULL CHECK(sequence BETWEEN 1 AND 1000000),
 kind TEXT NOT NULL CHECK(kind IN('SUBMIT','OPEN','APPROVE','RETURN','REJECT')),
 revision BIGINT NOT NULL CHECK(revision BETWEEN 1 AND 1000000),
 state TEXT NOT NULL,
 reason TEXT NOT NULL,
 actor_membership_id UUID NOT NULL,
 actor_employee_id UUID NOT NULL,
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,trip_id,sequence),
 CONSTRAINT org_trip_review_entries_operation_key UNIQUE(org_id,operation_id,kind),
 CONSTRAINT org_trip_review_entries_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_trip_review_entries_actor_fkey FOREIGN KEY(org_id,actor_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_review_entries_actor_employee_fkey FOREIGN KEY(org_id,actor_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_trip_review_entries_state CHECK((kind='SUBMIT' AND state='SUBMITTED') OR(kind='OPEN' AND state='IN_REVIEW') OR(kind='APPROVE' AND state='APPROVED')
  OR(kind='RETURN' AND state='RETURNED') OR(kind='REJECT' AND state='REJECTED')),
 -- B-BR-EMP-163 / APPR-001 reason policy: none, optional or required.
 CONSTRAINT org_trip_review_entries_reason CHECK(CASE WHEN kind IN('SUBMIT','OPEN') THEN reason=''
  WHEN kind='APPROVE' THEN reason='' OR org_asset_text_valid(reason,280,true) ELSE org_asset_text_valid(reason,280,true) END),
 CHECK(isfinite(recorded_at))
);
-- One submission per trip; one opening and one decision per revision.
CREATE UNIQUE INDEX org_trip_review_entries_one_submit ON org_trip_review_entries(org_id,trip_id) WHERE kind='SUBMIT';
CREATE UNIQUE INDEX org_trip_review_entries_one_open ON org_trip_review_entries(org_id,trip_id,revision) WHERE kind='OPEN';
CREATE UNIQUE INDEX org_trip_review_entries_one_decision ON org_trip_review_entries(org_id,trip_id,revision) WHERE kind IN('APPROVE','RETURN','REJECT');

-- Every entry re-proves the APPR-001 transition from the latest entry under
-- the organization root and trip row locks, a COMPLETED trip, the author of
-- a submission and the separation of a reviewer from the trip's author and
-- driver (BR-REV-003, canonical merge chain).
CREATE FUNCTION org_trip_review_entry_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;last_entry public.org_trip_review_entries%ROWTYPE;actor_person UUID;author_person UUID;driver_person UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF t.state<>'COMPLETED' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip review needs a completed trip';END IF;
 SELECT * INTO last_entry FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id ORDER BY sequence DESC LIMIT 1;
 IF NOT((last_entry.sequence IS NULL AND NEW.sequence=1 AND NEW.kind='SUBMIT' AND NEW.revision=1)
 OR(last_entry.sequence IS NOT NULL AND NEW.sequence=last_entry.sequence+1 AND NEW.revision=last_entry.revision
  AND((last_entry.state='SUBMITTED' AND NEW.kind='OPEN') OR(last_entry.state='IN_REVIEW' AND NEW.kind IN('APPROVE','RETURN','REJECT')))))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid trip review transition';END IF;
 IF NEW.kind='SUBMIT' THEN
  IF NEW.actor_membership_id<>t.author_membership_id OR NEW.actor_employee_id<>t.author_employee_id OR NOT EXISTS(
   SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
   JOIN public.org_device_enrollments e ON(e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
   WHERE o.org_id=NEW.org_id AND o.operation_id=NEW.operation_id AND o.operation_type='trip.submit' AND o.operation_schema=1 AND o.entity_type='trip_review'
   AND o.entity_id=NEW.trip_id AND o.membership_id=t.author_membership_id AND m.employee_id=t.author_employee_id AND m.account_id=o.owner_account_id
   AND m.state='ACTIVE' AND e.state='ENROLLED')
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip submission source unavailable';END IF;
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
 END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_review_entry_source BEFORE INSERT ON org_trip_review_entries FOR EACH ROW EXECUTE FUNCTION org_trip_review_entry_source();

-- At commit a submission is published only with its ACCEPTED receipt and
-- effect at the entry revision; an opening only beside the decision of the
-- same operation; a decision only with its own audit event.
CREATE FUNCTION org_trip_review_published() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE r public.org_trip_review_entries%ROWTYPE;decision public.org_trip_review_entries%ROWTYPE;prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 SELECT * INTO r FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id AND sequence=NEW.sequence;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip review publication unavailable';END IF;
 IF r.kind='SUBMIT' THEN
  IF NOT public.org_trip_published(r.org_id,r.operation_id,r.revision) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip submission publication unavailable';END IF;
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
 END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_trip_review_entry_published AFTER INSERT ON org_trip_review_entries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_trip_review_published();

ALTER TABLE org_trip_review_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_trip_review_entries FORCE ROW LEVEL SECURITY;
CREATE POLICY org_trip_review_entries_scope ON org_trip_review_entries USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_trip_review_entries_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_trip_review_entries FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history();
REVOKE ALL ON org_trip_review_entries FROM PUBLIC;
