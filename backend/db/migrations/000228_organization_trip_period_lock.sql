-- EMP-TRIP-016. Period lock of trip review decisions (B-BR-EMP-359..362,
-- EDR-232/233). The 226 review-entry source unchanged except one clause: an
-- opening or a decision whose trip's start reporting month (BR-TIME-004,
-- EDR-182: the day attributed at the start receipt in the stored zone) has a
-- LOCKED APPR-006 head is refused. The trigger already holds the organization
-- root FOR UPDATE, which every period command also takes, so the head read is
-- current. A submission (field intake) is never gated (EDR-232). No table,
-- column, index or usage source changes.
CREATE OR REPLACE FUNCTION org_trip_review_entry_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
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
 IF NEW.kind IN('OPEN','APPROVE','RETURN','REJECT') AND EXISTS(SELECT 1 FROM public.org_finance_periods p
  WHERE p.org_id=NEW.org_id AND p.period_month=date_trunc('month',t.reporting_day)::date AND p.state='LOCKED')
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='trip reporting period locked';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
