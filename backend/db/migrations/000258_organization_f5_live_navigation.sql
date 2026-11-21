-- F5 Live navigation and operations (EMP-GPS-007, EMP-GPS-011, EMP-GPS-013,
-- EMP-GPS-014, EMP-ROUTE-002, EMP-ROUTE-003; B-BR-EMP-563, BR-GPS-030..040,
-- BR-ROUTE-001..010).
-- Avoids SQL FOREIGN KEY constraints referencing org_units, org_cost_centres,
-- org_assets, or org_teams so usage-source pg_constraint audits stay intact;
-- unit_id values are validated in triggers instead.

CREATE SEQUENCE org_tracking_live_revision_seq START WITH 1 INCREMENT BY 1;

-- Per-organization retention and operational alert/freshness thresholds
-- (BR-GPS-031, BR-GPS-035, BR-GPS-038). Absent row uses specification defaults.
CREATE TABLE org_tracking_settings(
 org_id UUID NOT NULL PRIMARY KEY REFERENCES organizations(org_id),
 raw_retention_days INTEGER NOT NULL DEFAULT 90 CHECK(raw_retention_days BETWEEN 30 AND 365),
 live_current_seconds INTEGER NOT NULL DEFAULT 180 CHECK(live_current_seconds BETWEEN 60 AND 600),
 live_stale_seconds INTEGER NOT NULL DEFAULT 900 CHECK(live_stale_seconds BETWEEN 300 AND 3600),
 stale_tracker_seconds INTEGER NOT NULL DEFAULT 900 CHECK(stale_tracker_seconds BETWEEN 300 AND 7200),
 long_stop_seconds INTEGER NOT NULL DEFAULT 14400 CHECK(long_stop_seconds BETWEEN 3600 AND 43200),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 CONSTRAINT org_tracking_settings_live_order CHECK(live_current_seconds < live_stale_seconds)
);

CREATE FUNCTION org_tracking_settings_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 NEW.updated_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_tracking_settings_guard BEFORE INSERT OR UPDATE ON org_tracking_settings FOR EACH ROW EXECUTE FUNCTION org_tracking_settings_guard();

-- EMP-GPS-007: Live position projection per tracked trip. Freshness is never
-- stored; collected_at moves forward monotonically on newer fixes while
-- last_communication_at advances on every accepted batch. expires_at is set
-- to 24h after trip completion/cancellation and rows past expires_at are
-- ineligible for reads and eligible for purge.
CREATE TABLE org_tracking_live_positions(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 trip_id UUID NOT NULL,
 session_id UUID NOT NULL,
 unit_id UUID,
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 collected_at TIMESTAMPTZ CHECK(collected_at IS NULL OR isfinite(collected_at)),
 received_at TIMESTAMPTZ CHECK(received_at IS NULL OR isfinite(received_at)),
 last_communication_at TIMESTAMPTZ CHECK(last_communication_at IS NULL OR isfinite(last_communication_at)),
 lat_e7 INTEGER CHECK(lat_e7 IS NULL OR lat_e7 BETWEEN -900000000 AND 900000000),
 lon_e7 INTEGER CHECK(lon_e7 IS NULL OR lon_e7 BETWEEN -1800000000 AND 1800000000),
 accuracy_m INTEGER CHECK(accuracy_m IS NULL OR accuracy_m BETWEEN 0 AND 10000),
 speed_cm_s INTEGER CHECK(speed_cm_s IS NULL OR speed_cm_s BETWEEN 0 AND 100000),
 bearing_deg SMALLINT CHECK(bearing_deg IS NULL OR bearing_deg BETWEEN 0 AND 359),
 coarse BOOLEAN,
 source TEXT CHECK(source IS NULL OR source='PHONE'),
 operation_id UUID,
 expires_at TIMESTAMPTZ CHECK(expires_at IS NULL OR isfinite(expires_at)),
 PRIMARY KEY(org_id,trip_id),
 CONSTRAINT org_tracking_live_positions_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_tracking_live_positions_session_fkey FOREIGN KEY(org_id,session_id) REFERENCES org_tracking_sessions(org_id,id),
 CONSTRAINT org_tracking_live_positions_fix_complete CHECK((num_nonnulls(collected_at,received_at,lat_e7,lon_e7,accuracy_m,coarse)=0) OR (num_nonnulls(collected_at,received_at,lat_e7,lon_e7,accuracy_m,coarse)=6))
);
CREATE INDEX org_tracking_live_positions_rev ON org_tracking_live_positions(org_id,revision);
CREATE INDEX org_tracking_live_positions_expires ON org_tracking_live_positions(org_id,expires_at) WHERE expires_at IS NOT NULL;

CREATE FUNCTION org_tracking_live_position_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE s public.org_tracking_sessions%ROWTYPE;t public.org_trips%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT * INTO s FROM public.org_tracking_sessions WHERE org_id=NEW.org_id AND id=NEW.session_id;
 IF NOT FOUND OR s.trip_id<>NEW.trip_id THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 NEW.unit_id=t.unit_id;
 IF TG_OP='UPDATE' THEN
  IF OLD.collected_at IS NOT NULL AND NEW.collected_at IS NOT NULL AND NEW.collected_at<=OLD.collected_at THEN
   NEW.collected_at=OLD.collected_at;
   NEW.received_at=OLD.received_at;
   NEW.lat_e7=OLD.lat_e7;
   NEW.lon_e7=OLD.lon_e7;
   NEW.accuracy_m=OLD.accuracy_m;
   NEW.speed_cm_s=OLD.speed_cm_s;
   NEW.bearing_deg=OLD.bearing_deg;
   NEW.coarse=OLD.coarse;
   NEW.source=OLD.source;
   NEW.operation_id=OLD.operation_id;
  END IF;
  IF OLD.last_communication_at IS NOT NULL AND(NEW.last_communication_at IS NULL OR NEW.last_communication_at<OLD.last_communication_at) THEN
   NEW.last_communication_at=OLD.last_communication_at;
  END IF;
 END IF;
 NEW.revision=nextval('public.org_tracking_live_revision_seq');
 RETURN NEW;
END $$;
CREATE TRIGGER org_tracking_live_position_guard BEFORE INSERT OR UPDATE ON org_tracking_live_positions FOR EACH ROW EXECUTE FUNCTION org_tracking_live_position_guard();

-- Keep live position expiration aligned when trips finish or cancel.
CREATE FUNCTION org_tracking_live_position_expire() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE should_expire BOOLEAN=false;
BEGIN
 IF TG_TABLE_NAME='org_trip_finishes' THEN
  should_expire=true;
 ELSIF TG_TABLE_NAME='org_trip_transitions' THEN
  IF NEW.command='CANCEL' THEN
   should_expire=true;
  END IF;
 END IF;
 IF should_expire THEN
  IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
   RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
  END IF;
  UPDATE public.org_tracking_live_positions SET expires_at=NEW.occurred_at+interval '24 hours'
  WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id;
 END IF;
 RETURN NULL;
END $$;
REVOKE ALL ON FUNCTION org_tracking_live_position_expire() FROM PUBLIC;
CREATE TRIGGER org_tracking_live_expire_on_finish AFTER INSERT ON org_trip_finishes FOR EACH ROW EXECUTE FUNCTION org_tracking_live_position_expire();
CREATE TRIGGER org_tracking_live_expire_on_cancel AFTER INSERT ON org_trip_transitions FOR EACH ROW EXECUTE FUNCTION org_tracking_live_position_expire();

-- EMP-GPS-011: Insert-only purge receipts and purge-gated raw segment cleanup.
CREATE TABLE org_tracking_purge_receipts(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 trip_id UUID NOT NULL,
 session_id UUID NOT NULL,
 reason TEXT NOT NULL CHECK(reason IN('RETENTION_EXPIRED','TRIMMED_AFTER_END')),
 deleted_segments INTEGER NOT NULL CHECK(deleted_segments>=0),
 cutoff_at TIMESTAMPTZ NOT NULL CHECK(isfinite(cutoff_at)),
 purged_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(purged_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_tracking_purge_receipts_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_tracking_purge_receipts_session_fkey FOREIGN KEY(org_id,session_id) REFERENCES org_tracking_sessions(org_id,id)
);
CREATE INDEX org_tracking_purge_receipts_session ON org_tracking_purge_receipts(org_id,session_id,reason,purged_at DESC);

CREATE FUNCTION org_tracking_purge_receipt_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE s public.org_tracking_sessions%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT * INTO s FROM public.org_tracking_sessions WHERE org_id=NEW.org_id AND id=NEW.session_id;
 IF NOT FOUND OR s.trip_id<>NEW.trip_id THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 PERFORM 1 FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 NEW.purged_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_tracking_purge_receipt_guard BEFORE INSERT ON org_tracking_purge_receipts FOR EACH ROW EXECUTE FUNCTION org_tracking_purge_receipt_guard();

DROP TRIGGER org_tracking_segments_immutable ON org_tracking_segments;
CREATE TRIGGER org_tracking_segments_no_edit BEFORE UPDATE OR TRUNCATE ON org_tracking_segments FOR EACH STATEMENT EXECUTE FUNCTION org_tracking_keep_history();
CREATE FUNCTION org_tracking_segment_purge_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF OLD.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_tracking_purge_receipts r WHERE r.org_id=OLD.org_id AND r.session_id=OLD.session_id AND r.trip_id=OLD.trip_id)
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization tracking history is insert-only';END IF;
 RETURN OLD;
END $$;
CREATE TRIGGER org_tracking_segments_purge_only BEFORE DELETE ON org_tracking_segments FOR EACH ROW EXECUTE FUNCTION org_tracking_segment_purge_guard();

-- EMP-GPS-013: Server-side geofences (circle or non-self-intersecting polygon).
CREATE TABLE org_tracking_geofences(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 revision BIGINT NOT NULL CHECK(revision>0),
 name TEXT NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 160),
 unit_id UUID,
 kind TEXT NOT NULL CHECK(kind IN('CIRCLE','POLYGON')),
 center_lat_e7 INTEGER CHECK(center_lat_e7 IS NULL OR center_lat_e7 BETWEEN -900000000 AND 900000000),
 center_lon_e7 INTEGER CHECK(center_lon_e7 IS NULL OR center_lon_e7 BETWEEN -1800000000 AND 1800000000),
 radius_m INTEGER CHECK(radius_m IS NULL OR radius_m BETWEEN 50 AND 50000),
 vertices_lat_e7 INTEGER[] NOT NULL DEFAULT '{}',
 vertices_lon_e7 INTEGER[] NOT NULL DEFAULT '{}',
 geometry public.geometry(Geometry,4326) NOT NULL,
 on_enter BOOLEAN NOT NULL,
 on_exit BOOLEAN NOT NULL,
 dwell_seconds INTEGER NOT NULL DEFAULT 0 CHECK(dwell_seconds=0 OR dwell_seconds BETWEEN 60 AND 86400),
 recipients UUID[] NOT NULL DEFAULT '{}' CHECK(cardinality(recipients)<=50),
 enabled BOOLEAN NOT NULL DEFAULT true,
 archived BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_tracking_geofences_rules CHECK(on_enter OR on_exit OR dwell_seconds>0),
 CONSTRAINT org_tracking_geofences_shape CHECK((kind='CIRCLE' AND num_nonnulls(center_lat_e7,center_lon_e7,radius_m)=3 AND cardinality(vertices_lat_e7)=0 AND cardinality(vertices_lon_e7)=0)
  OR(kind='POLYGON' AND cardinality(vertices_lat_e7) BETWEEN 3 AND 100 AND cardinality(vertices_lon_e7)=cardinality(vertices_lat_e7) AND array_position(vertices_lat_e7,NULL) IS NULL AND array_position(vertices_lon_e7,NULL) IS NULL))
);
CREATE INDEX org_tracking_geofences_active ON org_tracking_geofences(org_id,enabled,archived);
CREATE INDEX org_tracking_geofences_geom ON org_tracking_geofences USING GIST(geometry);

CREATE FUNCTION org_tracking_geofence_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE ring public.geometry;poly public.geometry;n INTEGER;i INTEGER;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NEW.unit_id IS NOT NULL THEN
  PERFORM 1 FROM public.org_units u WHERE u.org_id=NEW.org_id AND u.id=NEW.unit_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='geofence revision conflict';END IF;
  NEW.created_at=OLD.created_at;
 ELSE
  IF NEW.revision<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='geofence revision conflict';END IF;
  NEW.created_at=clock_timestamp();
 END IF;
 IF NEW.kind='CIRCLE' THEN
  NEW.geometry=public.ST_Buffer(public.ST_SetSRID(public.ST_MakePoint(NEW.center_lon_e7::double precision/10000000,NEW.center_lat_e7::double precision/10000000),4326)::public.geography,NEW.radius_m)::public.geometry;
 ELSE
  n=cardinality(NEW.vertices_lat_e7);
  FOR i IN 1..n LOOP
   IF NEW.vertices_lat_e7[i] NOT BETWEEN -900000000 AND 900000000 OR NEW.vertices_lon_e7[i] NOT BETWEEN -1800000000 AND 1800000000
   THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid geofence geometry';END IF;
  END LOOP;
  ring=public.ST_MakeLine(ARRAY(
   SELECT public.ST_SetSRID(public.ST_MakePoint(NEW.vertices_lon_e7[k]::double precision/10000000,NEW.vertices_lat_e7[k]::double precision/10000000),4326)
   FROM unnest(array_append(ARRAY(SELECT generate_series(1,n)),1)) AS k));
  poly=public.ST_MakePolygon(ring);
  IF NOT public.ST_IsValid(poly) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid geofence geometry';END IF;
  NEW.geometry=poly;
 END IF;
 NEW.updated_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_tracking_geofence_guard BEFORE INSERT OR UPDATE ON org_tracking_geofences FOR EACH ROW EXECUTE FUNCTION org_tracking_geofence_guard();

CREATE TABLE org_tracking_geofence_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 operation_id UUID NOT NULL,
 geofence_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('CREATE','CHANGE')),
 actor_account_id UUID NOT NULL,
 actor_membership_id UUID NOT NULL,
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 result_revision BIGINT NOT NULL CHECK(result_revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,operation_id),
 CONSTRAINT org_tracking_geofence_commands_fence_fkey FOREIGN KEY(org_id,geofence_id) REFERENCES org_tracking_geofences(org_id,id),
 CONSTRAINT org_tracking_geofence_commands_actor_fkey FOREIGN KEY(org_id,actor_membership_id) REFERENCES org_memberships(org_id,id)
);

CREATE TABLE org_tracking_geofence_states(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 geofence_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 session_id UUID NOT NULL,
 inside BOOLEAN NOT NULL,
 entered_at TIMESTAMPTZ CHECK(entered_at IS NULL OR isfinite(entered_at)),
 dwell_emitted BOOLEAN NOT NULL DEFAULT false,
 last_fix_at TIMESTAMPTZ CHECK(last_fix_at IS NULL OR isfinite(last_fix_at)),
 PRIMARY KEY(org_id,geofence_id,session_id),
 CONSTRAINT org_tracking_geofence_states_fence_fkey FOREIGN KEY(org_id,geofence_id) REFERENCES org_tracking_geofences(org_id,id),
 CONSTRAINT org_tracking_geofence_states_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_tracking_geofence_states_session_fkey FOREIGN KEY(org_id,session_id) REFERENCES org_tracking_sessions(org_id,id)
);

CREATE TABLE org_tracking_geofence_events(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 geofence_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 session_id UUID NOT NULL,
 unit_id UUID,
 event_kind TEXT NOT NULL CHECK(event_kind IN('ENTER','EXIT','DWELL')),
 fix_at TIMESTAMPTZ NOT NULL CHECK(isfinite(fix_at)),
 detected_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(detected_at)),
 late_suppressed BOOLEAN NOT NULL,
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_tracking_geofence_events_dedup UNIQUE(org_id,geofence_id,session_id,event_kind,fix_at),
 CONSTRAINT org_tracking_geofence_events_fence_fkey FOREIGN KEY(org_id,geofence_id) REFERENCES org_tracking_geofences(org_id,id),
 CONSTRAINT org_tracking_geofence_events_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_tracking_geofence_events_session_fkey FOREIGN KEY(org_id,session_id) REFERENCES org_tracking_sessions(org_id,id)
);
CREATE INDEX org_tracking_geofence_events_fence ON org_tracking_geofence_events(org_id,geofence_id,detected_at DESC);

-- EMP-GPS-014: Coordinate-free operational alerts.
CREATE TABLE org_tracking_alerts(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 kind TEXT NOT NULL CHECK(kind IN('STALE_TRACKER','LONG_STOP','GEOFENCE')),
 trip_id UUID NOT NULL,
 session_id UUID NOT NULL,
 unit_id UUID,
 geofence_id UUID,
 event_kind TEXT NOT NULL DEFAULT '',
 anchor_at TIMESTAMPTZ NOT NULL CHECK(isfinite(anchor_at)),
 observed_at TIMESTAMPTZ NOT NULL CHECK(isfinite(observed_at)),
 duration_seconds BIGINT NOT NULL CHECK(duration_seconds>=0),
 threshold_seconds BIGINT NOT NULL CHECK(threshold_seconds>=0),
 radius_m BIGINT NOT NULL DEFAULT 0 CHECK(radius_m>=0),
 late_suppressed BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_tracking_alerts_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_tracking_alerts_session_fkey FOREIGN KEY(org_id,session_id) REFERENCES org_tracking_sessions(org_id,id),
 CONSTRAINT org_tracking_alerts_fence_fkey FOREIGN KEY(org_id,geofence_id) REFERENCES org_tracking_geofences(org_id,id)
);
CREATE UNIQUE INDEX org_tracking_alerts_dedup ON org_tracking_alerts(org_id,kind,session_id,COALESCE(geofence_id,'00000000-0000-0000-0000-000000000000'::uuid),event_kind,anchor_at);
CREATE INDEX org_tracking_alerts_observed ON org_tracking_alerts(org_id,observed_at DESC);

-- EMP-ROUTE-002 & EMP-ROUTE-003: Insert-only route plans and device computations.
CREATE TABLE org_route_plans(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 owner_kind TEXT NOT NULL CHECK(owner_kind IN('MISSION','TRIP')),
 owner_id UUID NOT NULL CHECK(owner_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 unit_id UUID,
 revision BIGINT NOT NULL CHECK(revision>0),
 waypoints JSONB NOT NULL CHECK(jsonb_typeof(waypoints)='array' AND jsonb_array_length(waypoints) BETWEEN 2 AND 102),
 avoid_toll BOOLEAN NOT NULL DEFAULT false,
 avoid_motorway BOOLEAN NOT NULL DEFAULT false,
 avoid_ferry BOOLEAN NOT NULL DEFAULT false,
 avoid_unpaved BOOLEAN NOT NULL DEFAULT false,
 planned_distance_m BIGINT CHECK(planned_distance_m IS NULL OR planned_distance_m BETWEEN 1 AND 100000000),
 planned_distance_source TEXT CHECK(planned_distance_source IS NULL OR planned_distance_source IN('MANUAL','ENGINE')),
 computation_id UUID,
 heavy_asset_notice BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_route_plans_owner_rev UNIQUE(org_id,owner_kind,owner_id,revision),
 CONSTRAINT org_route_plans_dist_pair CHECK((planned_distance_m IS NULL)=(planned_distance_source IS NULL))
);
CREATE INDEX org_route_plans_owner ON org_route_plans(org_id,owner_kind,owner_id,revision DESC);

CREATE FUNCTION org_route_plan_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE m public.org_trip_missions%ROWTYPE;t public.org_trips%ROWTYPE;prev_rev BIGINT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NEW.owner_kind='MISSION' THEN
  SELECT * INTO m FROM public.org_trip_missions WHERE org_id=NEW.org_id AND id=NEW.owner_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  NEW.unit_id=m.unit_id;
 ELSE
  SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.owner_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  NEW.unit_id=t.unit_id;
 END IF;
 SELECT COALESCE(max(p.revision),0) INTO prev_rev FROM public.org_route_plans p WHERE p.org_id=NEW.org_id AND p.owner_kind=NEW.owner_kind AND p.owner_id=NEW.owner_id;
 IF NEW.revision<>prev_rev+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='route plan revision conflict';END IF;
 NEW.created_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_route_plan_guard BEFORE INSERT ON org_route_plans FOR EACH ROW EXECUTE FUNCTION org_route_plan_guard();

CREATE TABLE org_route_plan_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 operation_id UUID NOT NULL,
 owner_kind TEXT NOT NULL CHECK(owner_kind IN('MISSION','TRIP')),
 owner_id UUID NOT NULL CHECK(owner_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 plan_id UUID NOT NULL,
 actor_account_id UUID NOT NULL,
 actor_membership_id UUID NOT NULL,
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 result_revision BIGINT NOT NULL CHECK(result_revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,operation_id),
 CONSTRAINT org_route_plan_commands_plan_fkey FOREIGN KEY(org_id,plan_id) REFERENCES org_route_plans(org_id,id),
 CONSTRAINT org_route_plan_commands_actor_fkey FOREIGN KEY(org_id,actor_membership_id) REFERENCES org_memberships(org_id,id)
);

CREATE TABLE org_route_computations(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 owner_kind TEXT NOT NULL CHECK(owner_kind IN('MISSION','TRIP')),
 owner_id UUID NOT NULL CHECK(owner_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 plan_revision BIGINT NOT NULL CHECK(plan_revision>0),
 engine_id TEXT NOT NULL CHECK(length(btrim(engine_id)) BETWEEN 1 AND 64),
 map_data_version TEXT NOT NULL CHECK(length(btrim(map_data_version)) BETWEEN 1 AND 64),
 profile TEXT NOT NULL CHECK(profile='CAR'),
 kind TEXT NOT NULL CHECK(kind IN('PLANNING','DEPARTURE','RECALCULATION')),
 avoid_toll BOOLEAN NOT NULL DEFAULT false,
 avoid_motorway BOOLEAN NOT NULL DEFAULT false,
 avoid_ferry BOOLEAN NOT NULL DEFAULT false,
 avoid_unpaved BOOLEAN NOT NULL DEFAULT false,
 distance_m BIGINT NOT NULL CHECK(distance_m BETWEEN 0 AND 100000000),
 duration_seconds BIGINT NOT NULL CHECK(duration_seconds BETWEEN 0 AND 10000000),
 line_lat_e7 INTEGER[] NOT NULL,
 line_lon_e7 INTEGER[] NOT NULL,
 geometry public.geometry(LineString,4326),
 attribution TEXT NOT NULL DEFAULT '© OpenStreetMap contributors',
 computed_at TIMESTAMPTZ NOT NULL CHECK(isfinite(computed_at)),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_route_computations_plan_fkey FOREIGN KEY(org_id,owner_kind,owner_id,plan_revision) REFERENCES org_route_plans(org_id,owner_kind,owner_id,revision),
 CONSTRAINT org_route_computations_line_arrays CHECK(cardinality(line_lat_e7)=cardinality(line_lon_e7) AND cardinality(line_lat_e7) BETWEEN 0 AND 1000 AND array_position(line_lat_e7,NULL) IS NULL AND array_position(line_lon_e7,NULL) IS NULL AND(cardinality(line_lat_e7)=0 OR cardinality(line_lat_e7)>=2))
);
CREATE INDEX org_route_computations_owner ON org_route_computations(org_id,owner_kind,owner_id,plan_revision DESC,computed_at DESC);

CREATE FUNCTION org_route_computation_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE k INTEGER;i INTEGER;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 k=cardinality(NEW.line_lat_e7);
 FOR i IN 1..k LOOP
  IF NEW.line_lat_e7[i] NOT BETWEEN -900000000 AND 900000000 OR NEW.line_lon_e7[i] NOT BETWEEN -1800000000 AND 1800000000
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid route computation geometry';END IF;
 END LOOP;
 NEW.geometry=CASE WHEN k<2 THEN NULL ELSE public.ST_SetSRID(public.ST_MakeLine(ARRAY(
  SELECT public.ST_MakePoint(NEW.line_lon_e7[idx]::double precision/10000000,NEW.line_lat_e7[idx]::double precision/10000000)
  FROM generate_series(1,k) AS idx ORDER BY idx)),4326) END;
 NEW.created_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_route_computation_guard BEFORE INSERT ON org_route_computations FOR EACH ROW EXECUTE FUNCTION org_route_computation_guard();

-- Backfill live position projection from any existing tracking sessions and
-- segments so migration upgrade preserves current operational state.
DO $$DECLARE r RECORD;fix RECORD;exp_at TIMESTAMPTZ;
BEGIN
 FOR r IN SELECT DISTINCT s.org_id,s.id AS session_id,s.trip_id,t.unit_id,t.state
  FROM public.org_tracking_sessions s JOIN public.org_trips t ON(t.org_id,t.id)=(s.org_id,s.trip_id) LOOP
  PERFORM set_config('anpfuel.org_id',r.org_id::text,true);
  exp_at=NULL;
  IF r.state='COMPLETED' THEN
   SELECT f.occurred_at+interval '24 hours' INTO exp_at FROM public.org_trip_finishes f WHERE f.org_id=r.org_id AND f.trip_id=r.trip_id;
  ELSIF r.state='CANCELLED' THEN
   SELECT tr.occurred_at+interval '24 hours' INTO exp_at FROM public.org_trip_transitions tr WHERE tr.org_id=r.org_id AND tr.trip_id=r.trip_id AND tr.command='CANCEL' LIMIT 1;
  END IF;
  SELECT u.collected_at,seg.received_at,u.lat_e7,u.lon_e7,u.accuracy_m,u.speed_cm_s,u.bearing_deg,u.coarse,seg.source,seg.operation_id
  INTO fix
  FROM public.org_tracking_segments seg
  CROSS JOIN LATERAL unnest(seg.collected_at,seg.lat_e7,seg.lon_e7,seg.accuracy_m,seg.speed_cm_s,seg.bearing_deg,seg.coarse) AS u(collected_at,lat_e7,lon_e7,accuracy_m,speed_cm_s,bearing_deg,coarse)
  WHERE seg.org_id=r.org_id AND seg.session_id=r.session_id
  ORDER BY u.collected_at DESC LIMIT 1;
  IF fix.collected_at IS NOT NULL THEN
   INSERT INTO public.org_tracking_live_positions(org_id,trip_id,session_id,unit_id,collected_at,received_at,last_communication_at,lat_e7,lon_e7,accuracy_m,speed_cm_s,bearing_deg,coarse,source,operation_id,expires_at)
   VALUES(r.org_id,r.trip_id,r.session_id,r.unit_id,fix.collected_at,fix.received_at,fix.received_at,fix.lat_e7,fix.lon_e7,fix.accuracy_m,fix.speed_cm_s,fix.bearing_deg,fix.coarse,fix.source,fix.operation_id,exp_at)
   ON CONFLICT(org_id,trip_id) DO NOTHING;
  ELSE
   INSERT INTO public.org_tracking_live_positions(org_id,trip_id,session_id,unit_id,expires_at)
   VALUES(r.org_id,r.trip_id,r.session_id,r.unit_id,exp_at)
   ON CONFLICT(org_id,trip_id) DO NOTHING;
  END IF;
 END LOOP;
 PERFORM set_config('anpfuel.org_id','',true);
END $$;

DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY[
  'org_tracking_settings','org_tracking_live_positions','org_tracking_purge_receipts',
  'org_tracking_geofences','org_tracking_geofence_commands','org_tracking_geofence_states',
  'org_tracking_geofence_events','org_tracking_alerts',
  'org_route_plans','org_route_plan_commands','org_route_computations'
 ] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
  EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
  EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
 FOREACH name IN ARRAY ARRAY[
  'org_tracking_purge_receipts','org_tracking_geofence_commands','org_tracking_geofence_events',
  'org_tracking_alerts','org_route_plans','org_route_plan_commands','org_route_computations'
 ] LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_tracking_keep_history()',name||'_immutable',name);
 END LOOP;
END $$;
