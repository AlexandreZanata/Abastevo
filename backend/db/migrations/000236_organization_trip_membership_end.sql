-- EMP-TRIP-018. Open trips when a membership ends (B-BR-EMP-404..406,
-- EDR-260/261). Additive, replacing two function bodies and keeping every
-- earlier line: the 191 offboarding guard admits a TRIP handover flag (a
-- trip in progress that the ending membership started, source revision 1)
-- and refuses an offboarding that leaves such a trip unflagged; offboarding
-- never closes, cancels or deletes the trip. The 220 held-trip guard admits
-- a late finish (ACCESS_ENDED_BEFORE_RECEIPT) of the trip's author from its
-- exact ended period, declared or claimed before the end (EMP-OFFL-009). No
-- table, key or usage source changes.

-- The 191 offboarding guard with the TRIP flag and the open-trip completeness check.
CREATE OR REPLACE FUNCTION org_offboarding_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE item JSONB;k TEXT;parts TEXT[];
BEGIN
 IF NEW.result-ARRAY['membership_id','employee_id','access_model','state','revision','suspension_origin','end_reason','employee_state','employee_revision','handover_digest','items']<>'{}'::jsonb
 OR NOT(NEW.result ?& ARRAY['membership_id','employee_id','access_model','state','revision','suspension_origin','end_reason','employee_state','employee_revision','handover_digest','items'])
 OR jsonb_typeof(NEW.result->'items')<>'array' OR jsonb_array_length(NEW.result->'items')>500
 OR NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
 WHERE m.org_id=NEW.org_id AND m.id=NEW.membership_id AND m.access_model='EXISTING_ACCOUNT' AND m.state='ENDED'
 AND NEW.result->>'membership_id'=m.id::text AND NEW.result->>'employee_id'=m.employee_id::text AND NEW.result->>'access_model'=m.access_model AND NEW.result->>'state'=m.state
 AND NEW.result->>'revision'=m.revision::text AND NEW.result->>'suspension_origin'=m.suspension_origin AND NEW.result->>'end_reason'=m.end_reason
 AND m.end_reason=CASE NEW.kind WHEN 'LEAVE' THEN 'LEFT' ELSE 'OFFBOARDED' END
 AND(NEW.kind<>'LEAVE' OR m.account_id=NEW.actor_account_id)
 AND NEW.result->>'employee_state'=e.state AND NEW.result->>'employee_revision'=e.revision::text)
 OR NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.actor_id=NEW.actor_account_id AND a.target_type='membership' AND a.target_id=NEW.membership_id AND a.request_hash=NEW.request_hash AND a.action='idn.membership.ended' AND a.reason_code=CASE NEW.kind WHEN 'LEAVE' THEN 'left' ELSE 'offboarded' END)
 THEN RAISE EXCEPTION USING ERRCODE='P9002',MESSAGE='offboarding source unavailable';END IF;
 -- Flags reference actual retained source facts after their reductions, not
 -- client-created work or a claim about modules that are not installed.
 FOR item IN SELECT value FROM jsonb_array_elements(NEW.result->'items') LOOP
  k=item->>'key';parts=string_to_array(k,':');
  IF item-ARRAY['key','source_revision']<>'{}'::jsonb OR NOT(item ?& ARRAY['key','source_revision']) OR NOT(
   (parts[1]='TEAM_LINK' AND array_length(parts,1)=2 AND item->>'source_revision'='1' AND EXISTS(SELECT 1 FROM public.org_team_links l JOIN public.org_memberships m ON(m.org_id,m.employee_id)=(l.org_id,l.employee_id) WHERE m.org_id=NEW.org_id AND m.id=NEW.membership_id AND l.id::text=parts[2] AND l.end_reason='MEMBERSHIP_ENDED' AND l.source_membership_id=NEW.membership_id AND l.source_membership_revision::text=NEW.result->>'revision'))
   OR(parts[1]='SETTING' AND array_length(parts,1)=3 AND EXISTS(SELECT 1 FROM public.org_settings_designations d WHERE d.org_id=NEW.org_id AND d.member_id=NEW.membership_id AND d.settings_revision::text=item->>'source_revision' AND d.responsibility=parts[2] AND d.slot=parts[3] AND EXISTS(SELECT 1 FROM public.org_settings_versions v WHERE v.org_id=d.org_id AND v.revision=d.settings_revision+1 AND v.source='MEMBERSHIP_REDUCTION' AND v.source_membership_id=NEW.membership_id AND v.source_membership_revision::text=NEW.result->>'revision')))
   -- EMP-TRIP-018: a trip in progress that the ending membership started.
   OR(parts[1]='TRIP' AND array_length(parts,1)=2 AND item->>'source_revision'='1' AND EXISTS(SELECT 1 FROM public.org_trips t WHERE t.org_id=NEW.org_id AND t.id::text=parts[2] AND t.author_membership_id=NEW.membership_id AND t.state='IN_PROGRESS')))
  THEN RAISE EXCEPTION USING ERRCODE='P9002',MESSAGE='handover source unavailable';END IF;
 END LOOP;
 -- EMP-TRIP-018 (B-BR-EMP-404): no trip in progress of the ending membership
 -- is left without its flag; offboarding never closes or deletes it.
 IF EXISTS(SELECT 1 FROM public.org_trips t WHERE t.org_id=NEW.org_id AND t.author_membership_id=NEW.membership_id AND t.state='IN_PROGRESS'
  AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(NEW.result->'items') x WHERE x.value->>'key'='TRIP:'||t.id::text))
 THEN RAISE EXCEPTION USING ERRCODE='P9002',MESSAGE='handover source unavailable';END IF;
 NEW.occurred_at=clock_timestamp();RETURN NEW;
END $$;

-- The 220 held-trip guard with the late finish branch; every other check unchanged.
CREATE OR REPLACE FUNCTION org_trip_held_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;o public.org_sync_operations%ROWTYPE;late_end TIMESTAMPTZ;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip intent unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid held trip declaration';END IF;
 IF TG_TABLE_NAME='org_trip_held_starts' THEN
  IF i.operation_type<>'trip.start' OR i.operation_schema<>1 OR i.code NOT IN('ASSET_BUSY','ASSET_UNAVAILABLE','PERMISSION_CHANGED')
  OR NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.start',NEW.trip_id,i.payload_digest,NULL,NULL,i.author_employee_id)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip source unavailable';END IF;
  IF EXISTS(SELECT 1 FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id) THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_trips_pkey',MESSAGE='trip identity unavailable';END IF;
  PERFORM 1 FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  -- ASSET_BUSY needs a trip in progress or one holding the declared instant;
  -- ASSET_UNAVAILABLE (lifecycle, block, reservation, maintenance, missing
  -- assignment) and PERMISSION_CHANGED are decided by their owning ports in
  -- this transaction and are not re-derived here.
  IF i.code='ASSET_BUSY' AND NOT EXISTS(SELECT 1 FROM public.org_trips t2 LEFT JOIN public.org_trip_finishes f2 ON(f2.org_id,f2.trip_id)=(t2.org_id,t2.id)
   WHERE t2.org_id=NEW.org_id AND t2.asset_id=NEW.asset_id AND(t2.state='IN_PROGRESS' OR((t2.occurred_at,t2.occurred_submicro)<=(NEW.occurred_at,NEW.occurred_submicro)
   AND(f2.occurred_at,f2.occurred_submicro)>(NEW.occurred_at,NEW.occurred_submicro))))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip condition unavailable';END IF;
 ELSE
  SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  -- EMP-TRIP-018 (OFFL-009): a finish of the trip's author declared, or
  -- claimed, before the end of its exact ended period, received late.
  IF i.code='ACCESS_ENDED_BEFORE_RECEIPT' THEN
   SELECT * INTO o FROM public.org_sync_operations WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
   IF FOUND THEN
    SELECT l.ended_at INTO late_end FROM public.org_sync_late_end(o.org_id,o.membership_id,o.enrollment_id,o.owner_account_id) l;
   END IF;
   IF i.operation_type<>'trip.finish' OR i.operation_schema<>1 OR o.operation_id IS NULL OR late_end IS NULL
   OR o.operation_type<>'trip.finish' OR o.operation_schema<>1 OR o.entity_type<>'trip' OR o.entity_id<>NEW.trip_id OR o.payload_digest<>i.payload_digest
   OR o.membership_id<>t.author_membership_id OR i.author_employee_id<>t.author_employee_id
   OR NOT(o.claimed_at<late_end OR NEW.occurred_at<late_end)
   OR EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=NEW.org_id AND f.operation_id=NEW.operation_id)
   OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
   OR(NEW.meter_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_meters m WHERE m.org_id=NEW.org_id AND m.id=NEW.meter_id AND m.asset_id=t.asset_id))
   THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip finish source unavailable';END IF;
   RETURN NEW;
  END IF;
  IF i.operation_type<>'trip.finish' OR i.operation_schema<>1 OR i.code<>'TRIP_ALREADY_CLOSED' OR t.state<>'COMPLETED'
  OR NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.finish',NEW.trip_id,i.payload_digest,t.author_membership_id,NULL,t.author_employee_id)
  OR EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=NEW.org_id AND f.operation_id=NEW.operation_id)
  OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
  OR(NEW.meter_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_meters m WHERE m.org_id=NEW.org_id AND m.id=NEW.meter_id AND m.asset_id=t.asset_id))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip finish source unavailable';END IF;
 END IF;
 RETURN NEW;
END $$;
