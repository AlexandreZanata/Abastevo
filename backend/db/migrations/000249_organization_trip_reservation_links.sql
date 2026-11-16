-- EMP-TRIP-023. Reservation-to-trip integration and no-show release
-- (B-BR-EMP-412..414, EDR-265, EDR-266). Migration 000249 (the reserved
-- 000238 stays unused: it would run before the 000243 bodies extended here).
-- Additive: a trip start may name the mission it executes through a soft
-- reference decided at receipt; an eligible start stores one insert-only
-- trip-mission link, moves the mission to IN_PROGRESS and consumes the
-- mission's confirmed reservation once (FULFILLED); the transaction that
-- completes the linked trip completes the mission. A planner requests the
-- mission's reservation through a RESERVE mission revision. Every redefined
-- function starts from its latest body and keeps every line (line log in
-- the task evidence); no existing row is written.

-- 1. Reservations name their mission and, once fulfilled, the trip.
ALTER TABLE org_asset_reservations ADD COLUMN mission_id UUID,ADD COLUMN trip_id UUID,ADD COLUMN fulfilled_at TIMESTAMPTZ,
 ADD CONSTRAINT org_asset_reservations_mission_fkey FOREIGN KEY(org_id,mission_id) REFERENCES org_trip_missions(org_id,id),
 ADD CONSTRAINT org_asset_reservations_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 ADD CONSTRAINT org_asset_reservations_trip_key UNIQUE(org_id,trip_id),
 ADD CONSTRAINT org_asset_reservations_fulfilled_check CHECK((state='FULFILLED')=(trip_id IS NOT NULL) AND(trip_id IS NULL)=(fulfilled_at IS NULL)
  AND(trip_id IS NULL OR mission_id IS NOT NULL) AND(mission_id IS NULL OR employee_id IS NULL) AND(fulfilled_at IS NULL OR isfinite(fulfilled_at)));
CREATE INDEX org_asset_reservations_mission ON org_asset_reservations(org_id,mission_id,starts_at,id) WHERE mission_id IS NOT NULL;

-- 2. Mission history gains the linked trip's START and COMPLETE (source TRIP)
-- and the planner's RESERVE; the 232 kind/source CHECKs widened by name.
ALTER TABLE org_trip_mission_transitions ADD COLUMN trip_id UUID,
 DROP CONSTRAINT org_trip_mission_transitions_kind_check,
 ADD CONSTRAINT org_trip_mission_transitions_kind_check CHECK(kind IN('CREATE','PLAN','ASSIGN','ACCEPT','DECLINE','CANCEL','VEHICLE','RESERVE','START','COMPLETE')),
 DROP CONSTRAINT org_trip_mission_transitions_source_check,
 ADD CONSTRAINT org_trip_mission_transitions_source_check CHECK(source IN('COMPANY','DEVICE','TRIP')),
 ADD CONSTRAINT org_trip_mission_transitions_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 ADD CONSTRAINT org_trip_mission_transitions_trip_check CHECK((source='TRIP')=(kind IN('START','COMPLETE')) AND(source='TRIP')=(trip_id IS NOT NULL));
ALTER TABLE org_trip_mission_commands DROP CONSTRAINT org_trip_mission_commands_kind_check,
 ADD CONSTRAINT org_trip_mission_commands_kind_check CHECK(kind IN('CREATE','PLAN','ASSIGN','CANCEL','VEHICLE','RESERVE'));

-- 3. One insert-only link per executing trip and per mission (B-BR-EMP-412).
CREATE TABLE org_trip_mission_links(
 org_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 mission_id UUID NOT NULL,
 operation_id UUID NOT NULL,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,trip_id),
 CONSTRAINT org_trip_mission_links_mission_key UNIQUE(org_id,mission_id),
 CONSTRAINT org_trip_mission_links_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_mission_links_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_trip_mission_links_mission_fkey FOREIGN KEY(org_id,mission_id) REFERENCES org_trip_missions(org_id,id),
 CONSTRAINT org_trip_mission_links_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CHECK(isfinite(recorded_at))
);

-- The link of the trip's own start operation, under the organization root,
-- trip and mission locks: the trip is IN_PROGRESS; the mission is ACCEPTED
-- by the trip's author membership for the trip's driver and names no other
-- vehicle.
CREATE FUNCTION org_trip_mission_link_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;m public.org_trip_missions%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 SELECT * INTO m FROM public.org_trip_missions WHERE org_id=NEW.org_id AND id=NEW.mission_id FOR UPDATE;
 IF t.id IS NULL OR m.id IS NULL THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF t.state<>'IN_PROGRESS' OR t.operation_id<>NEW.operation_id OR m.state<>'ACCEPTED' OR m.assignee_membership_id IS DISTINCT FROM t.author_membership_id
 OR m.assignee_employee_id IS DISTINCT FROM t.driver_employee_id OR m.asset_id IS NOT NULL AND m.asset_id<>t.asset_id
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission link source unavailable';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_mission_link_source BEFORE INSERT ON org_trip_mission_links FOR EACH ROW EXECUTE FUNCTION org_trip_mission_link_source();

-- At commit a link exists only with the mission's START revision of the
-- same trip and operation.
CREATE FUNCTION org_trip_mission_link_published() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NOT EXISTS(SELECT 1 FROM public.org_trip_mission_transitions x WHERE x.org_id=NEW.org_id AND x.mission_id=NEW.mission_id AND x.kind='START'
  AND x.trip_id=NEW.trip_id AND x.operation_id=NEW.operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission link publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_trip_mission_link_published AFTER INSERT ON org_trip_mission_links DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_trip_mission_link_published();

ALTER TABLE org_trip_mission_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_trip_mission_links FORCE ROW LEVEL SECURITY;
CREATE POLICY org_trip_mission_links_scope ON org_trip_mission_links USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_trip_mission_links_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_trip_mission_links FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history();
REVOKE ALL ON org_trip_mission_links FROM PUBLIC;

-- 4. B-BR-EMP-413: the transaction that completes a linked trip (the
-- driver's finish or a supervisor closure) completes its IN_PROGRESS mission
-- with that fact's operation and member; a mission no longer IN_PROGRESS
-- (cancelled by its planner) is left unchanged.
CREATE FUNCTION org_trip_mission_follow() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE l public.org_trip_mission_links%ROWTYPE;m public.org_trip_missions%ROWTYPE;op UUID;member UUID;account UUID;
BEGIN
 SELECT * INTO l FROM public.org_trip_mission_links WHERE org_id=NEW.org_id AND trip_id=NEW.id;
 IF NOT FOUND THEN RETURN NULL;END IF;
 SELECT * INTO m FROM public.org_trip_missions WHERE org_id=NEW.org_id AND id=l.mission_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF m.state<>'IN_PROGRESS' THEN RETURN NULL;END IF;
 SELECT f.operation_id,f.author_membership_id INTO op,member FROM public.org_trip_finishes f WHERE f.org_id=NEW.org_id AND f.trip_id=NEW.id;
 IF NOT FOUND THEN
  SELECT c.operation_id,c.closer_membership_id INTO op,member FROM public.org_trip_closures c WHERE c.org_id=NEW.org_id AND c.trip_id=NEW.id;
 END IF;
 SELECT p.account_id INTO account FROM public.org_memberships p WHERE p.org_id=NEW.org_id AND p.id=member;
 IF op IS NULL OR member IS NULL OR account IS NULL THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission completion source unavailable';END IF;
 INSERT INTO public.org_trip_mission_transitions(org_id,mission_id,revision,kind,from_state,to_state,source,actor_account_id,actor_membership_id,operation_id,
  assignee_employee_id,assignee_membership_id,trip_id)
 VALUES(NEW.org_id,m.id,m.revision+1,'COMPLETE','IN_PROGRESS','COMPLETED','TRIP',account,member,op,m.assignee_employee_id,m.assignee_membership_id,NEW.id);
 UPDATE public.org_trip_missions SET state='COMPLETED',revision=revision+1 WHERE org_id=NEW.org_id AND id=m.id AND revision=m.revision;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission completion source unavailable';END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER org_trip_mission_follow AFTER UPDATE OF state ON org_trips FOR EACH ROW
 WHEN(NEW.state='COMPLETED' AND OLD.state<>'COMPLETED') EXECUTE FUNCTION org_trip_mission_follow();

-- 5. The 217 reservation guard with the mission request and the FULFILLED
-- source (EDR-266).
CREATE OR REPLACE FUNCTION org_asset_reservation_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE asset_state TEXT;asset_revision BIGINT;employee_state TEXT;now_at TIMESTAMPTZ:=transaction_timestamp();
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept reservation changes';END IF;
  SELECT state INTO asset_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_asset_reservations_asset_fkey',MESSAGE='private reference not found';END IF;
  IF NEW.employee_id IS NOT NULL THEN
   SELECT state INTO employee_state FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.employee_id;
   IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_asset_reservations_employee_fkey',MESSAGE='private reference not found';END IF;
   IF employee_state<>'ACTIVE' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='employee does not accept reservation';END IF;
  END IF;
  IF asset_state<>'ACTIVE' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset does not accept reservation';END IF;
  IF NEW.state NOT IN('REQUESTED','CONFIRMED') OR NEW.revision<>1 OR NEW.starts_at<=now_at
   OR NEW.end_cause<>'' OR NEW.ended_at IS NOT NULL OR NEW.end_operation_id IS NOT NULL OR NEW.ended_by IS NOT NULL OR NEW.end_reason<>'' OR NEW.source_revision IS NOT NULL
   OR(NEW.state='REQUESTED' AND(NEW.confirm_operation_id IS NOT NULL OR NEW.confirmed_by IS NOT NULL))
   OR(NEW.state='CONFIRMED' AND(NEW.confirm_operation_id IS DISTINCT FROM NEW.operation_id OR NEW.confirmed_by IS DISTINCT FROM NEW.requested_by)) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial reservation';
  END IF;
  IF NEW.state='CONFIRMED' AND public.org_asset_reservation_blocked(NEW.org_id,NEW.asset_id,NEW.starts_at,NEW.ends_at) THEN
   RAISE EXCEPTION USING ERRCODE='23P01',CONSTRAINT='org_asset_reservations_block_overlap',MESSAGE='reservation window conflicts';
  END IF;
  -- EMP-TRIP-023 (EDR-266): nothing is created fulfilled; a mission's
  -- reservation comes only from the mission's RESERVE revision of the same
  -- operation, REQUESTED, without a reserved employee, for the mission's
  -- vehicle and planned window.
  IF NEW.trip_id IS NOT NULL OR NEW.fulfilled_at IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial reservation';END IF;
  IF NEW.mission_id IS NOT NULL THEN
   IF NEW.state<>'REQUESTED' OR NEW.employee_id IS NOT NULL OR NOT EXISTS(SELECT 1 FROM public.org_trip_missions m
    JOIN public.org_trip_mission_transitions x ON(x.org_id,x.mission_id)=(m.org_id,m.id)
    WHERE m.org_id=NEW.org_id AND m.id=NEW.mission_id AND m.asset_id=NEW.asset_id AND m.starts_at=NEW.starts_at AND m.ends_at=NEW.ends_at
    AND x.kind='RESERVE' AND x.operation_id=NEW.operation_id AND x.revision=m.revision+1) THEN
    RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission reservation source unavailable';
   END IF;
  END IF;
  NEW.requested_at=clock_timestamp();
  NEW.confirmed_at=CASE WHEN NEW.state='CONFIRMED' THEN NEW.requested_at END;
  RETURN NEW;
 END IF;
 IF ROW(NEW.org_id,NEW.id,NEW.asset_id,NEW.employee_id,NEW.purpose,NEW.starts_at,NEW.ends_at,NEW.no_show_tolerance,NEW.operation_id,NEW.requested_by,NEW.requested_at)
   IS DISTINCT FROM ROW(OLD.org_id,OLD.id,OLD.asset_id,OLD.employee_id,OLD.purpose,OLD.starts_at,OLD.ends_at,OLD.no_show_tolerance,OLD.operation_id,OLD.requested_by,OLD.requested_at)
  OR NEW.revision IS DISTINCT FROM OLD.revision+1 THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='reservation is immutable';
 END IF;
 -- EMP-TRIP-023: the mission link never changes; the trip and its instant
 -- are written once, by the FULFILLED transition below.
 IF NEW.mission_id IS DISTINCT FROM OLD.mission_id OR OLD.trip_id IS NOT NULL OR OLD.fulfilled_at IS NOT NULL
  OR NEW.state<>'FULFILLED' AND(NEW.trip_id IS NOT NULL OR NEW.fulfilled_at IS NOT NULL) THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='reservation is immutable';
 END IF;
 IF OLD.state='REQUESTED' AND NEW.state='CONFIRMED' THEN
  PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept reservation changes';END IF;
  IF now_at>=OLD.starts_at OR NEW.confirm_operation_id IS NULL OR NEW.confirmed_by IS NULL OR NEW.end_cause<>'' OR NEW.ended_at IS NOT NULL
   OR NEW.end_operation_id IS NOT NULL OR NEW.ended_by IS NOT NULL OR NEW.end_reason<>'' OR NEW.source_revision IS NOT NULL THEN
   RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='reservation transition unavailable';
  END IF;
  SELECT state INTO asset_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id;
  IF asset_state IS DISTINCT FROM 'ACTIVE' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset does not accept reservation';END IF;
  IF public.org_asset_reservation_blocked(NEW.org_id,NEW.asset_id,NEW.starts_at,NEW.ends_at) THEN
   RAISE EXCEPTION USING ERRCODE='23P01',CONSTRAINT='org_asset_reservations_block_overlap',MESSAGE='reservation window conflicts';
  END IF;
  NEW.confirmed_at=clock_timestamp();
  RETURN NEW;
 END IF;
 IF ROW(NEW.confirm_operation_id,NEW.confirmed_by,NEW.confirmed_at) IS DISTINCT FROM ROW(OLD.confirm_operation_id,OLD.confirmed_by,OLD.confirmed_at) THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='reservation is immutable';
 END IF;
 IF OLD.state IN('REQUESTED','CONFIRMED') AND NEW.state='CANCELLED' THEN
  IF OLD.state='REQUESTED' AND now_at>=OLD.starts_at OR OLD.state='CONFIRMED' AND now_at>=public.org_asset_reservation_deadline(OLD.starts_at,OLD.ends_at,OLD.no_show_tolerance) THEN
   RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='reservation already ended';
  END IF;
  IF NEW.end_cause='CANCELLED' THEN
   PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
   IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept reservation changes';END IF;
   IF NEW.end_operation_id IS NULL OR NEW.ended_by IS NULL OR NEW.source_revision IS NOT NULL THEN
    RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reservation end source unavailable';
   END IF;
  ELSIF NEW.end_cause='ASSET_LEFT_SERVICE' THEN
   SELECT state,revision INTO asset_state,asset_revision FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id;
   IF asset_state NOT IN('INACTIVE','DISPOSED','ARCHIVED') OR asset_revision IS DISTINCT FROM NEW.source_revision
    OR NEW.end_operation_id IS NOT NULL OR NEW.ended_by IS NOT NULL OR NEW.end_reason<>'' THEN
    RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reservation end source unavailable';
   END IF;
  ELSE
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reservation end source unavailable';
  END IF;
  NEW.ended_at=clock_timestamp();
  RETURN NEW;
 END IF;
 IF(OLD.state='REQUESTED' AND NEW.state='EXPIRED' AND now_at>=OLD.starts_at)
  OR(OLD.state='CONFIRMED' AND NEW.state='NO_SHOW' AND now_at>=public.org_asset_reservation_deadline(OLD.starts_at,OLD.ends_at,OLD.no_show_tolerance)) THEN
  IF NEW.end_cause<>'TIME' OR NEW.end_operation_id IS NOT NULL OR NEW.ended_by IS NOT NULL OR NEW.end_reason<>'' OR NEW.source_revision IS NOT NULL THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reservation end source unavailable';
  END IF;
  NEW.ended_at=clock_timestamp();
  RETURN NEW;
 END IF;
 -- EMP-TRIP-023 (B-BR-EMP-414): CONFIRMED -> FULFILLED before the no-show
 -- deadline, by the IN_PROGRESS trip linked to the reservation's mission on
 -- its asset whose declared start lies in [start - 30 minutes, end).
 IF OLD.state='CONFIRMED' AND NEW.state='FULFILLED' THEN
  IF now_at>=public.org_asset_reservation_deadline(OLD.starts_at,OLD.ends_at,OLD.no_show_tolerance) OR OLD.mission_id IS NULL OR NEW.trip_id IS NULL
   OR NEW.end_cause<>'' OR NEW.ended_at IS NOT NULL OR NEW.end_operation_id IS NOT NULL OR NEW.ended_by IS NOT NULL OR NEW.end_reason<>'' OR NEW.source_revision IS NOT NULL THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reservation fulfilment source unavailable';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM public.org_trip_mission_links l JOIN public.org_trips t ON(t.org_id,t.id)=(l.org_id,l.trip_id)
   WHERE l.org_id=NEW.org_id AND l.trip_id=NEW.trip_id AND l.mission_id=OLD.mission_id AND t.asset_id=OLD.asset_id AND t.state='IN_PROGRESS'
   AND t.occurred_at>=OLD.starts_at-interval '30 minutes' AND t.occurred_at<OLD.ends_at) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reservation fulfilment source unavailable';
  END IF;
  NEW.fulfilled_at=clock_timestamp();
  RETURN NEW;
 END IF;
 RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='reservation transition unavailable';
END $$;

-- 6. The 232 mission table with the EDR-265 rows.
CREATE OR REPLACE FUNCTION org_trip_mission_next(from_state TEXT,command TEXT) RETURNS TEXT
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT CASE
  -- EMP-TRIP-023 (EDR-265): the linked trip completes a mission in progress
  -- and its planner may cancel it; nothing else changes it.
  WHEN from_state='IN_PROGRESS' AND command='COMPLETE' THEN 'COMPLETED'
  WHEN from_state='IN_PROGRESS' AND command='CANCEL' THEN 'CANCELLED'
  WHEN from_state IN('COMPLETED','CANCELLED','IN_PROGRESS') THEN NULL
  WHEN command='PLAN' AND from_state IN('DRAFT','PLANNED','ASSIGNED','DECLINED') THEN 'PLANNED'
  WHEN command='ASSIGN' AND from_state IN('DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED') THEN 'ASSIGNED'
  WHEN command='CANCEL' AND from_state IN('DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED') THEN 'CANCELLED'
  WHEN command='ACCEPT' AND from_state='ASSIGNED' THEN 'ACCEPTED'
  WHEN command='DECLINE' AND from_state='ASSIGNED' THEN 'DECLINED'
  WHEN command='VEHICLE' AND from_state IN('PLANNED','ASSIGNED','ACCEPTED','DECLINED') THEN from_state
  -- EMP-TRIP-023: the linked start; the reservation request keeps the state.
  WHEN command='START' AND from_state='ACCEPTED' THEN 'IN_PROGRESS'
  WHEN command='RESERVE' AND from_state IN('PLANNED','ASSIGNED','ACCEPTED','DECLINED') THEN from_state
 END
 $$;

-- 7. The 231 transition source with the trip-driven kinds.
CREATE OR REPLACE FUNCTION org_trip_mission_transition_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE m public.org_trip_missions%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO m FROM public.org_trip_missions WHERE org_id=NEW.org_id AND id=NEW.mission_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NEW.kind='CREATE' THEN
  IF NEW.revision<>1 OR m.revision<>1 OR m.state<>'DRAFT' OR NEW.operation_id<>m.create_operation_id OR NEW.actor_account_id<>m.created_by
  OR NEW.actor_membership_id<>m.created_by_membership_id OR NEW.source<>'COMPANY'
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission transition unavailable';END IF;
 ELSE
  IF NEW.revision<>m.revision+1 OR NEW.from_state IS DISTINCT FROM m.state OR NEW.to_state IS DISTINCT FROM public.org_trip_mission_next(m.state,NEW.kind)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission transition unavailable';END IF;
  IF NEW.kind='ASSIGN' AND(NOT public.org_trip_mission_assignee_valid(NEW.org_id,NEW.assignee_employee_id,NEW.assignee_membership_id)
   OR m.state IN('ASSIGNED','ACCEPTED') AND NEW.assignee_membership_id=m.assignee_membership_id)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission assignee unavailable';END IF;
  IF NEW.kind='PLAN' AND(NEW.assignee_employee_id IS NOT NULL OR NEW.assignee_membership_id IS NOT NULL)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission transition unavailable';END IF;
  IF NEW.kind NOT IN('ASSIGN','PLAN') AND(NEW.assignee_employee_id IS DISTINCT FROM m.assignee_employee_id OR NEW.assignee_membership_id IS DISTINCT FROM m.assignee_membership_id)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission transition unavailable';END IF;
  IF NEW.kind IN('ACCEPT','DECLINE') AND(NEW.actor_membership_id IS DISTINCT FROM m.assignee_membership_id OR NOT EXISTS(
   SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships mm ON(mm.org_id,mm.id)=(o.org_id,o.membership_id)
   JOIN public.org_device_enrollments d ON(d.org_id,d.id,d.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
   WHERE o.org_id=NEW.org_id AND o.operation_id=NEW.operation_id AND o.operation_type='trip.mission.respond' AND o.operation_schema=1
   AND o.entity_type='mission' AND o.entity_id=NEW.mission_id AND o.membership_id=NEW.actor_membership_id AND o.enrollment_id=NEW.enrollment_id
   AND o.owner_account_id=NEW.actor_account_id AND mm.account_id=o.owner_account_id AND mm.state='ACTIVE' AND d.state='ENROLLED'))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission response source unavailable';END IF;
 END IF;
 -- EMP-TRIP-023 (EDR-265): START and COMPLETE come only from the linked trip
 -- (source TRIP): the start's link, trip operation and author; the finish or
 -- supervisor closure that completed the trip and its member.
 IF(NEW.source='TRIP')<>(NEW.kind IN('START','COMPLETE')) OR(NEW.source='TRIP')<>(NEW.trip_id IS NOT NULL)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission trip source unavailable';END IF;
 IF NEW.kind='START' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_trip_mission_links l JOIN public.org_trips t ON(t.org_id,t.id)=(l.org_id,l.trip_id)
   JOIN public.org_memberships mm ON(mm.org_id,mm.id)=(t.org_id,t.author_membership_id)
   WHERE l.org_id=NEW.org_id AND l.mission_id=NEW.mission_id AND l.trip_id=NEW.trip_id AND l.operation_id=NEW.operation_id AND t.operation_id=NEW.operation_id
   AND t.state='IN_PROGRESS' AND NEW.actor_membership_id=t.author_membership_id AND NEW.actor_account_id=mm.account_id)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission trip source unavailable';END IF;
 ELSIF NEW.kind='COMPLETE' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_trip_mission_links l JOIN public.org_trips t ON(t.org_id,t.id)=(l.org_id,l.trip_id)
   JOIN public.org_memberships mm ON(mm.org_id,mm.id)=(l.org_id,NEW.actor_membership_id)
   WHERE l.org_id=NEW.org_id AND l.mission_id=NEW.mission_id AND l.trip_id=NEW.trip_id AND t.state='COMPLETED' AND mm.account_id=NEW.actor_account_id
   AND(EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=t.org_id AND f.trip_id=t.id AND f.operation_id=NEW.operation_id AND f.author_membership_id=NEW.actor_membership_id)
   OR EXISTS(SELECT 1 FROM public.org_trip_closures c WHERE c.org_id=t.org_id AND c.trip_id=t.id AND c.operation_id=NEW.operation_id AND c.closer_membership_id=NEW.actor_membership_id)))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission trip source unavailable';END IF;
 END IF;
 IF NEW.source='COMPANY' AND NOT public.org_trip_mission_actor_valid(NEW.org_id,NEW.actor_membership_id,NEW.actor_account_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission transition unavailable';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;

-- 8. The 232 head guard with START, COMPLETE and RESERVE.
CREATE OR REPLACE FUNCTION org_trip_mission_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trip_mission_transitions%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM OLD.org_id OR NEW.id IS DISTINCT FROM OLD.id OR NEW.unit_id IS DISTINCT FROM OLD.unit_id
 OR NEW.asset_id IS DISTINCT FROM OLD.asset_id AND NOT EXISTS(SELECT 1 FROM public.org_trip_mission_vehicles v WHERE v.org_id=NEW.org_id AND v.mission_id=NEW.id
  AND v.revision=NEW.revision AND v.previous_asset_id IS NOT DISTINCT FROM OLD.asset_id AND v.asset_id=NEW.asset_id)
 OR NEW.title IS DISTINCT FROM OLD.title OR NEW.starts_at IS DISTINCT FROM OLD.starts_at OR NEW.ends_at IS DISTINCT FROM OLD.ends_at
 OR NEW.created_by IS DISTINCT FROM OLD.created_by OR NEW.created_by_membership_id IS DISTINCT FROM OLD.created_by_membership_id
 OR NEW.create_operation_id IS DISTINCT FROM OLD.create_operation_id OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.revision<>OLD.revision+1
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='mission history is append-only';END IF;
 SELECT * INTO t FROM public.org_trip_mission_transitions WHERE org_id=NEW.org_id AND mission_id=NEW.id AND revision=NEW.revision;
 IF NOT FOUND OR t.from_state IS DISTINCT FROM OLD.state OR t.to_state IS DISTINCT FROM NEW.state
 OR t.assignee_employee_id IS DISTINCT FROM NEW.assignee_employee_id OR t.assignee_membership_id IS DISTINCT FROM NEW.assignee_membership_id
 OR t.kind='ASSIGN' AND(NEW.answer IS NOT NULL OR NEW.answer_reason IS NOT NULL)
 OR t.kind IN('ACCEPT','DECLINE') AND(NEW.answer IS DISTINCT FROM t.kind OR NEW.answer_reason IS DISTINCT FROM t.reason)
 OR t.kind='CANCEL' AND(NEW.answer IS DISTINCT FROM OLD.answer OR NEW.answer_reason IS DISTINCT FROM OLD.answer_reason)
 OR t.kind='PLAN' AND(NEW.answer IS NOT NULL OR NEW.answer_reason IS NOT NULL OR NOT EXISTS(SELECT 1 FROM public.org_trip_mission_plans p
  WHERE p.org_id=t.org_id AND p.mission_id=t.mission_id AND p.revision=t.revision AND p.operation_id=t.operation_id))
 OR t.kind='VEHICLE' AND(NEW.answer IS DISTINCT FROM OLD.answer OR NEW.answer_reason IS DISTINCT FROM OLD.answer_reason OR NOT EXISTS(SELECT 1 FROM public.org_trip_mission_vehicles v
  WHERE v.org_id=t.org_id AND v.mission_id=t.mission_id AND v.revision=t.revision AND v.operation_id=t.operation_id AND v.asset_id=NEW.asset_id))
 -- EMP-TRIP-023: START, COMPLETE and RESERVE keep the answer.
 OR t.kind IN('START','COMPLETE','RESERVE') AND(NEW.answer IS DISTINCT FROM OLD.answer OR NEW.answer_reason IS DISTINCT FROM OLD.answer_reason)
 OR t.kind='CREATE'
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='mission history is append-only';END IF;
 -- EMP-TRIP-023: a RESERVE revision exists only with its reservation.
 IF t.kind='RESERVE' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_asset_reservations r WHERE r.org_id=t.org_id AND r.mission_id=t.mission_id AND r.operation_id=t.operation_id AND r.state='REQUESTED')
  THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='mission history is append-only';END IF;
 END IF;
 NEW.updated_at=clock_timestamp();RETURN NEW;
END $$;

-- 9. The 232 publication check with RESERVE and the trip-driven kinds.
CREATE OR REPLACE FUNCTION org_trip_mission_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF TG_TABLE_NAME='org_trip_missions' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_trip_mission_transitions t WHERE t.org_id=NEW.org_id AND t.mission_id=NEW.id AND t.revision=1 AND t.kind='CREATE')
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission publication unavailable';END IF;
 ELSIF NOT EXISTS(SELECT 1 FROM public.org_trip_missions m WHERE m.org_id=NEW.org_id AND m.id=NEW.mission_id AND m.revision>=NEW.revision)
 OR NEW.source='COMPANY' AND NOT EXISTS(SELECT 1 FROM public.org_trip_mission_commands c WHERE c.org_id=NEW.org_id AND c.operation_id=NEW.operation_id
  AND c.mission_id=NEW.mission_id AND c.result_revision=NEW.revision AND c.actor_id=NEW.actor_account_id)
 OR NEW.source='DEVICE' AND NOT public.org_trip_published(NEW.org_id,NEW.operation_id,NEW.revision)
 OR NEW.kind='PLAN' AND NOT EXISTS(SELECT 1 FROM public.org_trip_mission_plans p WHERE p.org_id=NEW.org_id AND p.mission_id=NEW.mission_id
  AND p.revision=NEW.revision AND p.operation_id=NEW.operation_id)
 OR NEW.kind='VEHICLE' AND NOT EXISTS(SELECT 1 FROM public.org_trip_mission_vehicles v WHERE v.org_id=NEW.org_id AND v.mission_id=NEW.mission_id
  AND v.revision=NEW.revision AND v.operation_id=NEW.operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission publication unavailable';END IF;
 -- EMP-TRIP-023: a RESERVE revision is published with its reservation, a
 -- trip-driven one with the trip's link to the mission.
 IF TG_TABLE_NAME='org_trip_mission_transitions' THEN
  IF NEW.kind='RESERVE' THEN
   IF NOT EXISTS(SELECT 1 FROM public.org_asset_reservations r WHERE r.org_id=NEW.org_id AND r.mission_id=NEW.mission_id AND r.operation_id=NEW.operation_id)
   THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission publication unavailable';END IF;
  ELSIF NEW.source='TRIP' THEN
   IF NOT EXISTS(SELECT 1 FROM public.org_trip_mission_links l WHERE l.org_id=NEW.org_id AND l.mission_id=NEW.mission_id AND l.trip_id=NEW.trip_id)
   THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission publication unavailable';END IF;
  END IF;
 END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;

-- 10. The 243 held source admitting ORPHAN_REFERENCE for a held start.
CREATE OR REPLACE FUNCTION org_trip_held_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;o public.org_sync_operations%ROWTYPE;late_end TIMESTAMPTZ;rm UUID;re UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip intent unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid held trip declaration';END IF;
 IF TG_TABLE_NAME='org_trip_held_starts' THEN
  IF i.operation_type<>'trip.start' OR i.operation_schema<>1 OR i.code NOT IN('ASSET_BUSY','ASSET_UNAVAILABLE','PERMISSION_CHANGED','ORPHAN_REFERENCE')
  OR NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.start',NEW.trip_id,i.payload_digest,NULL,NULL,i.author_employee_id)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip source unavailable';END IF;
  IF EXISTS(SELECT 1 FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id) THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_trips_pkey',MESSAGE='trip identity unavailable';END IF;
  PERFORM 1 FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  -- EMP-TRIP-023 (EDR-265): ORPHAN_REFERENCE is a start whose mission
  -- reference was missing, foreign or ineligible at receipt; the operation's
  -- soft references are not stored, so it is decided by the owner here.
  -- ASSET_BUSY needs a trip in progress or one holding the declared instant;
  -- ASSET_UNAVAILABLE (lifecycle, block, reservation, maintenance, missing
  -- assignment) and PERMISSION_CHANGED are decided by their owning ports in
  -- this transaction and are not re-derived here.
  -- EMP-TRIP-011: a paused trip holds the asset; a cancelled one until its cancellation.
  IF i.code='ASSET_BUSY' AND NOT EXISTS(SELECT 1 FROM public.org_trips t2 LEFT JOIN public.org_trip_finishes f2 ON(f2.org_id,f2.trip_id)=(t2.org_id,t2.id)
   LEFT JOIN public.org_trip_transitions c2 ON(c2.org_id,c2.trip_id,c2.command)=(t2.org_id,t2.id,'CANCEL')
   WHERE t2.org_id=NEW.org_id AND t2.asset_id=NEW.asset_id AND(t2.state IN('IN_PROGRESS','PAUSED') OR((t2.occurred_at,t2.occurred_submicro)<=(NEW.occurred_at,NEW.occurred_submicro)
   AND(COALESCE(f2.occurred_at,c2.occurred_at),COALESCE(f2.occurred_submicro,c2.occurred_submicro))>(NEW.occurred_at,NEW.occurred_submicro))))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip condition unavailable';END IF;
 ELSE
  SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  -- EMP-TRIP-018 (OFFL-009): a finish of the trip's author declared, or
  -- claimed, before the end of its exact ended period, received late.
  IF i.code='ACCESS_ENDED_BEFORE_RECEIPT' THEN
   -- EMP-TRIP-012: a finish comes from the current responsible member.
   SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NULL,NULL) r;
   SELECT * INTO o FROM public.org_sync_operations WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
   IF FOUND THEN
    SELECT l.ended_at INTO late_end FROM public.org_sync_late_end(o.org_id,o.membership_id,o.enrollment_id,o.owner_account_id) l;
   END IF;
   IF i.operation_type<>'trip.finish' OR i.operation_schema<>1 OR o.operation_id IS NULL OR late_end IS NULL
   OR o.operation_type<>'trip.finish' OR o.operation_schema<>1 OR o.entity_type<>'trip' OR o.entity_id<>NEW.trip_id OR o.payload_digest<>i.payload_digest
   OR o.membership_id IS DISTINCT FROM rm OR i.author_employee_id IS DISTINCT FROM re
   OR NOT(o.claimed_at<late_end OR NEW.occurred_at<late_end)
   OR EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=NEW.org_id AND f.operation_id=NEW.operation_id)
   OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
   OR(NEW.meter_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_meters m WHERE m.org_id=NEW.org_id AND m.id=NEW.meter_id AND m.asset_id=t.asset_id))
   THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip finish source unavailable';END IF;
   RETURN NEW;
  END IF;
  -- EMP-TRIP-012: the current responsible member and that member's employee.
  SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NULL,NULL) r;
  IF i.operation_type<>'trip.finish' OR i.operation_schema<>1 OR i.code<>'TRIP_ALREADY_CLOSED' OR t.state NOT IN('COMPLETED','CANCELLED')
  OR NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.finish',NEW.trip_id,i.payload_digest,rm,NULL,re)
  OR EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=NEW.org_id AND f.operation_id=NEW.operation_id)
  OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
  OR(NEW.meter_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_meters m WHERE m.org_id=NEW.org_id AND m.id=NEW.meter_id AND m.asset_id=t.asset_id))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip finish source unavailable';END IF;
 END IF;
 RETURN NEW;
END $$;
