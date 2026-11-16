-- EMP-GPS-004. Track intake: the organization tracking policy, one tracking
-- session per trip with its insert-only transitions, and one insert-only
-- segment per accepted track batch (B-BR-EMP-428..434, EDR-277..280).
-- Session operations and batches arrive through the OFFL-002 signed intake;
-- every write locks the trip row (the trip finish locks the same row), so the
-- recording periods, the trip's running intervals (start, EMP-TRIP-011 pauses,
-- cancellation, finish) and the per-session instant uniqueness
-- are decided serially. Rows reference only the trip, the session, the
-- operation, the author and the enrollment: no asset, unit, cost-centre or
-- employee key, so no usage source changes. Positions are 10^-7 degree
-- integers; the MultiPoint geometry is derived by the database.

-- B-BR-EMP-428: insert-only policy versions; no version means OFF.
CREATE TABLE org_tracking_policies(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 revision BIGINT NOT NULL CHECK(revision>0),
 policy TEXT NOT NULL CHECK(policy IN('OFF','OPTIONAL','EXPECTED')),
 notice_version TEXT CHECK(notice_version~'^[a-z0-9][a-z0-9._-]{0,63}$'),
 notice_in_force_at TIMESTAMPTZ CHECK(isfinite(notice_in_force_at)),
 effective_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(effective_at)),
 PRIMARY KEY(org_id,revision),
 CONSTRAINT org_tracking_policies_notice CHECK((policy='OFF' AND notice_version IS NULL AND notice_in_force_at IS NULL)
  OR(policy<>'OFF' AND notice_version IS NOT NULL AND notice_in_force_at IS NOT NULL))
);

-- The policy in force at an instant: the latest version effective then,
-- OPTIONAL or EXPECTED only once its notice is in force; no row is OFF.
CREATE FUNCTION org_tracking_policy_in_force(p_org UUID,p_at TIMESTAMPTZ) RETURNS TABLE(policy TEXT,revision BIGINT)
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT CASE WHEN p.policy<>'OFF' AND p.notice_in_force_at<=p_at THEN p.policy ELSE 'OFF' END,p.revision
 FROM public.org_tracking_policies p WHERE p.org_id=p_org AND p.effective_at<=p_at ORDER BY p.revision DESC LIMIT 1
 $$;

CREATE FUNCTION org_tracking_policy_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NEW.revision<>COALESCE((SELECT max(p.revision) FROM public.org_tracking_policies p WHERE p.org_id=NEW.org_id),0)+1
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking policy revision unavailable';END IF;
 NEW.effective_at=GREATEST(NEW.effective_at,clock_timestamp());RETURN NEW;
END $$;
CREATE TRIGGER org_tracking_policy_guard BEFORE INSERT ON org_tracking_policies FOR EACH ROW EXECUTE FUNCTION org_tracking_policy_guard();

-- B-BR-EMP-430: one session per trip, bound to the trip's driver membership
-- and to the policy version it opened under.
CREATE TABLE org_tracking_sessions(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 trip_id UUID NOT NULL,
 owner_membership_id UUID NOT NULL,
 policy TEXT NOT NULL CHECK(policy IN('OPTIONAL','EXPECTED')),
 policy_revision BIGINT NOT NULL,
 operation_id UUID NOT NULL,
 opened_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_tracking_sessions_trip_key UNIQUE(org_id,trip_id),
 CONSTRAINT org_tracking_sessions_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_tracking_sessions_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_tracking_sessions_owner_fkey FOREIGN KEY(org_id,owner_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_tracking_sessions_policy_fkey FOREIGN KEY(org_id,policy_revision) REFERENCES org_tracking_policies(org_id,revision),
 CONSTRAINT org_tracking_sessions_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CHECK(isfinite(opened_at) AND opened_at>='0001-01-01 00:00:00+00'::timestamptz AND opened_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(created_at))
);

CREATE TABLE org_tracking_transitions(
 org_id UUID NOT NULL,
 session_id UUID NOT NULL,
 seq INTEGER NOT NULL CHECK(seq BETWEEN 1 AND 100000),
 state TEXT NOT NULL CHECK(state IN('NOT_STARTED','RECORDING','SUSPENDED','INTERRUPTED','ENDED')),
 cause TEXT CHECK(cause IN('NOT_STARTED','PERMISSION_DENIED','NOTIFICATION_BLOCKED','TRIP_PAUSED','LONG_STATIONARY','PERMISSION_REVOKED','PROCESS_STOPPED',
  'DEVICE_RESTARTED','STOPPED_BY_DRIVER','OWNER_CHANGED','PRECISION_REDUCED','TRIP_COMPLETED','TRIP_CANCELLED','MEMBERSHIP_ENDED','DEVICE_REVOKED')),
 actor TEXT NOT NULL CHECK(actor IN('DRIVER','DEVICE','TRIP','SERVER')),
 permission TEXT CHECK(permission IN('PRECISE','APPROXIMATE','NONE')),
 occurred_at TIMESTAMPTZ NOT NULL,
 operation_id UUID NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,session_id,seq),
 CONSTRAINT org_tracking_transitions_session_fkey FOREIGN KEY(org_id,session_id) REFERENCES org_tracking_sessions(org_id,id),
 CONSTRAINT org_tracking_transitions_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CONSTRAINT org_tracking_transitions_recording CHECK(state<>'RECORDING' OR permission IN('PRECISE','APPROXIMATE')),
 CONSTRAINT org_tracking_transitions_opening CHECK((seq=1)=(cause IS NOT DISTINCT FROM 'NOT_STARTED')
  AND(seq<>1 OR(state='NOT_STARTED' AND actor='DRIVER' AND permission IS NULL))),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz)
);
CREATE INDEX org_tracking_transitions_operation ON org_tracking_transitions(org_id,operation_id,seq);

-- The device operation of a tracking row: exact type family, entity and an
-- active enrolled owner.
CREATE FUNCTION org_tracking_operation_valid(p_org UUID,p_op UUID,p_session UUID,p_batch BOOLEAN,p_digest TEXT,p_member UUID,p_enrollment UUID) RETURNS BOOLEAN
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
  JOIN public.org_device_enrollments e ON(e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
  WHERE o.org_id=p_org AND o.operation_id=p_op AND o.operation_schema=1 AND o.entity_type='tracking_session' AND o.entity_id=p_session
  AND(CASE WHEN p_batch THEN o.operation_type='gps.track.batch'
   ELSE o.operation_type IN('gps.session.start','gps.session.resume','gps.session.suspend','gps.session.interrupt','gps.session.reduce','gps.session.end') END)
  AND(p_digest IS NULL OR o.payload_digest=p_digest) AND o.membership_id=p_member AND(p_enrollment IS NULL OR o.enrollment_id=p_enrollment)
  AND m.account_id=o.owner_account_id AND m.state='ACTIVE' AND e.state='ENROLLED')
 $$;

-- B-BR-EMP-432: a trip runs from its start until its finish or cancellation,
-- except while its latest lifecycle row at or before the instant (EMP-TRIP-011)
-- leaves it PAUSED or CANCELLED.
CREATE FUNCTION org_tracking_trip_running_at(p_org UUID,p_trip UUID,p_at TIMESTAMPTZ) RETURNS BOOLEAN
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_trips t WHERE t.org_id=p_org AND t.id=p_trip AND(p_at,0::smallint)>=(t.occurred_at,t.occurred_submicro))
 AND NOT EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=p_org AND f.trip_id=p_trip AND(p_at,0::smallint)>=(f.occurred_at,f.occurred_submicro))
 AND COALESCE((SELECT x.to_state FROM public.org_trip_transitions x WHERE x.org_id=p_org AND x.trip_id=p_trip
  AND(x.occurred_at,x.occurred_submicro)<=(p_at,0::smallint) ORDER BY x.revision DESC LIMIT 1),'IN_PROGRESS')='IN_PROGRESS'
 $$;

-- B-BR-EMP-428/430: a session opens under the policy in force, for a stored
-- trip run by its owner, at an instant while the trip runs.
CREATE FUNCTION org_tracking_session_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;p RECORD;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 NEW.created_at=clock_timestamp();
 SELECT * INTO p FROM public.org_tracking_policy_in_force(NEW.org_id,NEW.created_at);
 IF NOT FOUND OR p.policy IS DISTINCT FROM NEW.policy OR p.revision IS DISTINCT FROM NEW.policy_revision
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking policy unavailable';END IF;
 IF NEW.owner_membership_id<>t.author_membership_id
 OR NOT public.org_tracking_operation_valid(NEW.org_id,NEW.operation_id,NEW.id,false,NULL,NEW.owner_membership_id,NULL)
 OR NOT EXISTS(SELECT 1 FROM public.org_sync_operations o WHERE o.org_id=NEW.org_id AND o.operation_id=NEW.operation_id AND o.operation_type='gps.session.start')
 OR NOT public.org_tracking_trip_running_at(NEW.org_id,NEW.trip_id,NEW.opened_at)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking session source unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_tracking_session_guard BEFORE INSERT ON org_tracking_sessions FOR EACH ROW EXECUTE FUNCTION org_tracking_session_guard();

-- B-BR-EMP-430: contiguous, ordered, terminal ENDED, the owner's device.
CREATE FUNCTION org_tracking_transition_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE s public.org_tracking_sessions%ROWTYPE;previous public.org_tracking_transitions%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT * INTO s FROM public.org_tracking_sessions WHERE org_id=NEW.org_id AND id=NEW.session_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 PERFORM 1 FROM public.org_trips WHERE org_id=NEW.org_id AND id=s.trip_id FOR UPDATE;
 IF NOT public.org_tracking_operation_valid(NEW.org_id,NEW.operation_id,NEW.session_id,false,NULL,s.owner_membership_id,NULL)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking transition source unavailable';END IF;
 SELECT * INTO previous FROM public.org_tracking_transitions WHERE org_id=NEW.org_id AND session_id=NEW.session_id ORDER BY seq DESC LIMIT 1;
 IF NOT FOUND THEN
  IF NEW.seq<>1 OR NEW.operation_id<>s.operation_id OR NEW.occurred_at<>s.opened_at
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking transition order unavailable';END IF;
 ELSIF NEW.seq<>previous.seq+1 OR NEW.occurred_at<previous.occurred_at OR previous.state='ENDED'
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking transition order unavailable';END IF;
 NEW.created_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_tracking_transition_guard BEFORE INSERT ON org_tracking_transitions FOR EACH ROW EXECUTE FUNCTION org_tracking_transition_guard();

-- At commit a session holds its opening row and every row its ACCEPTED
-- receipt, whose entity revision is the operation's last sequence.
CREATE FUNCTION org_tracking_transition_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF TG_TABLE_NAME='org_tracking_sessions' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_tracking_transitions t WHERE t.org_id=NEW.org_id AND t.session_id=NEW.id AND t.seq=1 AND t.operation_id=NEW.operation_id)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking session publication unavailable';END IF;
 ELSIF NOT public.org_trip_published(NEW.org_id,NEW.operation_id,
  (SELECT max(t.seq) FROM public.org_tracking_transitions t WHERE t.org_id=NEW.org_id AND t.session_id=NEW.session_id AND t.operation_id=NEW.operation_id)::bigint)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking transition publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_tracking_session_complete AFTER INSERT ON org_tracking_sessions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_tracking_transition_complete();
CREATE CONSTRAINT TRIGGER org_tracking_transition_complete AFTER INSERT ON org_tracking_transitions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_tracking_transition_complete();

-- B-BR-EMP-432: the precision of the recording period holding an instant
-- (the latest transition at or before it is RECORDING), or NULL; a trip
-- pause at or after the session's last stored transition and not after the
-- instant suspends it first (the GPS-001 FOLLOW rule, as the intake applies
-- it to the stored history).
CREATE FUNCTION org_tracking_precision_at(p_org UUID,p_session UUID,p_at TIMESTAMPTZ) RETURNS TEXT
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT CASE WHEN t.state='RECORDING' AND NOT EXISTS(SELECT 1 FROM public.org_tracking_sessions s JOIN public.org_trip_transitions x ON(x.org_id,x.trip_id)=(s.org_id,s.trip_id)
  WHERE s.org_id=p_org AND s.id=p_session AND x.command='PAUSE'
  AND(x.occurred_at,x.occurred_submicro)>=((SELECT max(l.occurred_at) FROM public.org_tracking_transitions l WHERE l.org_id=p_org AND l.session_id=p_session),0::smallint)
  AND(x.occurred_at,x.occurred_submicro)<=(p_at,0::smallint)) THEN t.permission END
 FROM public.org_tracking_transitions t
 WHERE t.org_id=p_org AND t.session_id=p_session AND t.occurred_at<=p_at ORDER BY t.seq DESC LIMIT 1
 $$;

-- B-BR-EMP-434: one accepted batch, exact integer arrays and the derived
-- MultiPoint; counters of what was not stored.
CREATE TABLE org_tracking_segments(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 operation_id UUID NOT NULL,
 payload_digest TEXT NOT NULL CHECK(payload_digest~'^sha256:[a-f0-9]{64}$'),
 session_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 seq INTEGER NOT NULL CHECK(seq>0),
 author_membership_id UUID NOT NULL,
 enrollment_id UUID NOT NULL,
 source TEXT NOT NULL CHECK(source='PHONE'),
 boot_id TEXT CHECK(boot_id~'^[A-Za-z0-9._-]{1,64}$'),
 base_elapsed_millis BIGINT CHECK(base_elapsed_millis>=0),
 base_at TIMESTAMPTZ NOT NULL,
 first_at TIMESTAMPTZ,
 last_at TIMESTAMPTZ,
 collected_at TIMESTAMPTZ[] NOT NULL,
 lat_e7 INTEGER[] NOT NULL,
 lon_e7 INTEGER[] NOT NULL,
 accuracy_m INTEGER[] NOT NULL,
 speed_cm_s INTEGER[] NOT NULL,
 bearing_deg SMALLINT[] NOT NULL,
 coarse BOOLEAN[] NOT NULL,
 points public.geometry(MultiPoint,4326),
 received_count INTEGER NOT NULL CHECK(received_count BETWEEN 1 AND 200),
 stored_count INTEGER NOT NULL CHECK(stored_count>=0),
 duplicate_count INTEGER NOT NULL CHECK(duplicate_count>=0),
 outside_session_count INTEGER NOT NULL CHECK(outside_session_count>=0),
 mock_count INTEGER NOT NULL CHECK(mock_count>=0),
 dropped_accuracy INTEGER NOT NULL CHECK(dropped_accuracy BETWEEN 0 AND 1000000),
 dropped_stale INTEGER NOT NULL CHECK(dropped_stale BETWEEN 0 AND 1000000),
 dropped_jump INTEGER NOT NULL CHECK(dropped_jump BETWEEN 0 AND 1000000),
 dropped_mock INTEGER NOT NULL CHECK(dropped_mock BETWEEN 0 AND 1000000),
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,operation_id),
 CONSTRAINT org_tracking_segments_sequence_key UNIQUE(org_id,session_id,seq),
 CONSTRAINT org_tracking_segments_session_fkey FOREIGN KEY(org_id,session_id) REFERENCES org_tracking_sessions(org_id,id),
 CONSTRAINT org_tracking_segments_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_tracking_segments_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CONSTRAINT org_tracking_segments_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_tracking_segments_enrollment_fkey FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id),
 CONSTRAINT org_tracking_segments_counts CHECK(received_count=stored_count+duplicate_count+outside_session_count+mock_count),
 CONSTRAINT org_tracking_segments_arrays CHECK(cardinality(collected_at)=stored_count AND cardinality(lat_e7)=stored_count AND cardinality(lon_e7)=stored_count
  AND cardinality(accuracy_m)=stored_count AND cardinality(speed_cm_s)=stored_count AND cardinality(bearing_deg)=stored_count AND cardinality(coarse)=stored_count
  AND(stored_count=0 OR(array_ndims(collected_at)=1 AND array_ndims(lat_e7)=1 AND array_ndims(lon_e7)=1 AND array_ndims(accuracy_m)=1
   AND array_ndims(speed_cm_s)=1 AND array_ndims(bearing_deg)=1 AND array_ndims(coarse)=1))
  AND array_position(collected_at,NULL) IS NULL AND array_position(lat_e7,NULL) IS NULL AND array_position(lon_e7,NULL) IS NULL
  AND array_position(accuracy_m,NULL) IS NULL AND array_position(coarse,NULL) IS NULL),
 CONSTRAINT org_tracking_segments_bounds CHECK((stored_count=0)=(first_at IS NULL) AND(first_at IS NULL)=(last_at IS NULL) AND(stored_count=0)=(points IS NULL)),
 CONSTRAINT org_tracking_segments_geometry CHECK(points IS NULL OR(public.ST_SRID(points)=4326 AND public.ST_NumGeometries(points)=stored_count)),
 CONSTRAINT org_tracking_segments_run_clock CHECK((boot_id IS NULL)=(base_elapsed_millis IS NULL)),
 CHECK(isfinite(base_at) AND base_at>='0001-01-01 00:00:00+00'::timestamptz AND base_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(received_at))
);
CREATE INDEX org_tracking_segments_span ON org_tracking_segments(org_id,session_id,first_at,last_at);
CREATE INDEX org_tracking_segments_trip ON org_tracking_segments(org_id,trip_id,seq);

-- B-BR-EMP-432/433: under the trip row lock, every stored fix is inside a
-- recording period with its precision, while the trip runs and new for the
-- session; the geometry is derived here.
CREATE FUNCTION org_tracking_segment_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE s public.org_tracking_sessions%ROWTYPE;t public.org_trips%ROWTYPE;i INTEGER;n INTEGER;p TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT * INTO s FROM public.org_tracking_sessions WHERE org_id=NEW.org_id AND id=NEW.session_id;
 IF NOT FOUND OR s.trip_id<>NEW.trip_id THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NEW.author_membership_id<>s.owner_membership_id
 OR NOT public.org_tracking_operation_valid(NEW.org_id,NEW.operation_id,NEW.session_id,true,NEW.payload_digest,NEW.author_membership_id,NEW.enrollment_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking segment source unavailable';END IF;
 n=NEW.stored_count;
 FOR i IN 1..n LOOP
  IF NEW.lat_e7[i] NOT BETWEEN -900000000 AND 900000000 OR NEW.lon_e7[i] NOT BETWEEN -1800000000 AND 1800000000 OR NEW.accuracy_m[i] NOT BETWEEN 0 AND 10000
  OR NEW.speed_cm_s[i] NOT BETWEEN 0 AND 100000 OR NEW.bearing_deg[i] NOT BETWEEN 0 AND 359
  OR(i>1 AND NEW.collected_at[i]<=NEW.collected_at[i-1]) OR NOT isfinite(NEW.collected_at[i])
  OR NOT public.org_tracking_trip_running_at(NEW.org_id,NEW.trip_id,NEW.collected_at[i])
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking fix outside the session';END IF;
  p=public.org_tracking_precision_at(NEW.org_id,NEW.session_id,NEW.collected_at[i]);
  IF p IS NULL OR NEW.coarse[i]<>(p='APPROXIMATE') THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking fix outside the session';END IF;
 END LOOP;
 IF n>0 AND(NEW.first_at<>NEW.collected_at[1] OR NEW.last_at<>NEW.collected_at[n]) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking segment bounds unavailable';END IF;
 IF n>0 AND EXISTS(SELECT 1 FROM public.org_tracking_segments x WHERE x.org_id=NEW.org_id AND x.session_id=NEW.session_id
  AND x.first_at<=NEW.last_at AND x.last_at>=NEW.first_at AND x.collected_at&&NEW.collected_at)
 THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_tracking_segments_fix_key',MESSAGE='tracking fix already stored';END IF;
 NEW.points=CASE WHEN n=0 THEN NULL ELSE public.ST_SetSRID(public.ST_Collect(ARRAY(
  SELECT public.ST_MakePoint(u.lon::double precision/10000000,u.lat::double precision/10000000) FROM unnest(NEW.lon_e7,NEW.lat_e7) WITH ORDINALITY AS u(lon,lat,k) ORDER BY u.k)),4326) END;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_tracking_segment_guard BEFORE INSERT ON org_tracking_segments FOR EACH ROW EXECUTE FUNCTION org_tracking_segment_guard();

CREATE FUNCTION org_tracking_segment_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NOT public.org_trip_published(NEW.org_id,NEW.operation_id,NEW.seq::bigint)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking segment publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_tracking_segment_complete AFTER INSERT ON org_tracking_segments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_tracking_segment_complete();

CREATE FUNCTION org_tracking_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization tracking history is insert-only';END $$;
DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_tracking_policies','org_tracking_sessions','org_tracking_transitions','org_tracking_segments'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_tracking_keep_history()',name||'_immutable',name);
 EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;
