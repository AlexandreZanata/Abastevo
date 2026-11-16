-- EMP-GPS-005. Track processing: one insert-only summary revision per
-- processed input of an ended tracking session (B-BR-EMP-461..466,
-- EDR-298..301). Raw fixes are never edited; the summary records its method,
-- the exact input it was derived from (the session's segment and transition
-- counts, both insert-only), the per-fix filter counters, the gaps with their
-- reasons, coverage and quality, the GPS distance with its method and the
-- simplified geometry with its tolerance. Under the trip row lock (the intake
-- and the trip finish lock the same row) the database re-proves that the
-- summary names exactly the stored input of this session, that it is newer
-- than the current revision and that its counters are the session's own; no
-- asset, unit, cost-centre or employee key, so no usage source changes.
CREATE TABLE org_tracking_summaries(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 session_id UUID NOT NULL,
 revision INTEGER NOT NULL CHECK(revision BETWEEN 1 AND 1000000),
 trip_id UUID NOT NULL,
 method TEXT NOT NULL CHECK(method='gps-track-v1'),
 input_segments INTEGER NOT NULL CHECK(input_segments>=0),
 input_transitions INTEGER NOT NULL CHECK(input_transitions>=2),
 job_id UUID NOT NULL,
 lease_token BIGINT NOT NULL CHECK(lease_token>0),
 expected_seconds BIGINT NOT NULL CHECK(expected_seconds BETWEEN 0 AND 1000000000000),
 gap_seconds BIGINT NOT NULL CHECK(gap_seconds>=0),
 coverage_permille INTEGER CHECK(coverage_permille BETWEEN 0 AND 1000),
 coarse BOOLEAN NOT NULL,
 quality TEXT NOT NULL CHECK(quality IN('PRECISE','APPROXIMATE','INSUFFICIENT')),
 accepted_fixes INTEGER NOT NULL CHECK(accepted_fixes>=0),
 coarse_fixes INTEGER NOT NULL CHECK(coarse_fixes>=0),
 rejected_accuracy INTEGER NOT NULL CHECK(rejected_accuracy>=0),
 rejected_speed INTEGER NOT NULL CHECK(rejected_speed>=0),
 rejected_outside INTEGER NOT NULL CHECK(rejected_outside>=0),
 received_fixes BIGINT NOT NULL CHECK(received_fixes>=0),
 duplicate_fixes BIGINT NOT NULL CHECK(duplicate_fixes>=0),
 outside_session_fixes BIGINT NOT NULL CHECK(outside_session_fixes>=0),
 mock_fixes BIGINT NOT NULL CHECK(mock_fixes>=0),
 device_dropped_accuracy BIGINT NOT NULL CHECK(device_dropped_accuracy>=0),
 device_dropped_stale BIGINT NOT NULL CHECK(device_dropped_stale>=0),
 device_dropped_jump BIGINT NOT NULL CHECK(device_dropped_jump>=0),
 device_dropped_mock BIGINT NOT NULL CHECK(device_dropped_mock>=0),
 clock_uncertain_segments INTEGER NOT NULL CHECK(clock_uncertain_segments>=0),
 distance_m BIGINT CHECK(distance_m>=0),
 distance_method TEXT CHECK(distance_method='haversine-r6371008.8-step10-v1'),
 gap_from TIMESTAMPTZ[] NOT NULL,
 gap_to TIMESTAMPTZ[] NOT NULL,
 gap_reason TEXT[] NOT NULL,
 gap_detected_by TEXT[] NOT NULL,
 line_parts INTEGER[] NOT NULL,
 line_lat_e7 INTEGER[] NOT NULL,
 line_lon_e7 INTEGER[] NOT NULL,
 tolerance_m INTEGER CHECK(tolerance_m BETWEEN 20 AND 20971520),
 geometry public.geometry(MultiLineString,4326),
 processed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(processed_at)),
 PRIMARY KEY(org_id,session_id,revision),
 CONSTRAINT org_tracking_summaries_session_fkey FOREIGN KEY(org_id,session_id) REFERENCES org_tracking_sessions(org_id,id),
 CONSTRAINT org_tracking_summaries_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_tracking_summaries_counts CHECK(gap_seconds<=expected_seconds AND coarse_fixes<=accepted_fixes AND clock_uncertain_segments<=input_segments),
 -- B-BR-EMP-463: coverage and quality are the BR-GPS-026 function of the counts.
 CONSTRAINT org_tracking_summaries_quality CHECK((expected_seconds=0)=(coverage_permille IS NULL)
  AND(expected_seconds=0 OR coverage_permille=((expected_seconds-gap_seconds)*1000)/expected_seconds)
  AND quality=CASE WHEN expected_seconds=0 OR accepted_fixes<2 OR coverage_permille<500 THEN 'INSUFFICIENT'
   WHEN coarse OR coverage_permille<900 THEN 'APPROXIMATE' ELSE 'PRECISE' END),
 -- B-BR-EMP-464/465: nothing is derived from an insufficient track.
 CONSTRAINT org_tracking_summaries_derived CHECK((distance_m IS NULL)=(distance_method IS NULL)
  AND(quality<>'INSUFFICIENT' OR(distance_m IS NULL AND cardinality(line_parts)=0))
  AND(cardinality(line_parts)=0)=(tolerance_m IS NULL) AND(cardinality(line_parts)=0)=(geometry IS NULL)),
 CONSTRAINT org_tracking_summaries_arrays CHECK(cardinality(gap_from)=cardinality(gap_to) AND cardinality(gap_from)=cardinality(gap_reason)
  AND cardinality(gap_from)=cardinality(gap_detected_by) AND cardinality(line_lat_e7)=cardinality(line_lon_e7)
  AND array_position(gap_from,NULL) IS NULL AND array_position(gap_to,NULL) IS NULL AND array_position(gap_reason,NULL) IS NULL
  AND array_position(gap_detected_by,NULL) IS NULL AND array_position(line_parts,NULL) IS NULL AND array_position(line_lat_e7,NULL) IS NULL
  AND array_position(line_lon_e7,NULL) IS NULL AND cardinality(line_lat_e7)<=1000
  AND(cardinality(gap_from)=0 OR array_ndims(gap_from)=1) AND(cardinality(line_parts)=0 OR array_ndims(line_parts)=1)),
 CONSTRAINT org_tracking_summaries_geometry CHECK(geometry IS NULL OR(public.ST_SRID(geometry)=4326 AND public.ST_NumGeometries(geometry)=cardinality(line_parts)))
);
CREATE INDEX org_tracking_summaries_trip ON org_tracking_summaries(org_id,trip_id,revision);

-- B-BR-EMP-463/465/466: under the trip row lock the summary names exactly
-- the stored input of an ended session, is newer than the current revision,
-- carries the session's own totals, ordered disjoint gaps with closed
-- reasons and bounded lines; the geometry is derived here.
CREATE FUNCTION org_tracking_summary_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE s public.org_tracking_sessions%ROWTYPE;previous public.org_tracking_summaries%ROWTYPE;i INTEGER;k INTEGER;n INTEGER;gaps NUMERIC;
 t RECORD;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT * INTO s FROM public.org_tracking_sessions WHERE org_id=NEW.org_id AND id=NEW.session_id;
 IF NOT FOUND OR s.trip_id<>NEW.trip_id THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 PERFORM 1 FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_tracking_transitions x WHERE x.org_id=NEW.org_id AND x.session_id=NEW.session_id AND x.state='ENDED')
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking summary source unavailable';END IF;
 SELECT count(*) AS segments,COALESCE(sum(stored_count),0) AS stored,COALESCE(sum(received_count),0) AS received,COALESCE(sum(duplicate_count),0) AS duplicates,
  COALESCE(sum(outside_session_count),0) AS outside,COALESCE(sum(mock_count),0) AS mock,COALESCE(sum(dropped_accuracy),0) AS d_accuracy,
  COALESCE(sum(dropped_stale),0) AS d_stale,COALESCE(sum(dropped_jump),0) AS d_jump,COALESCE(sum(dropped_mock),0) AS d_mock
  INTO t FROM public.org_tracking_segments WHERE org_id=NEW.org_id AND session_id=NEW.session_id;
 IF NEW.input_segments<>t.segments OR NEW.input_transitions<>(SELECT count(*) FROM public.org_tracking_transitions x WHERE x.org_id=NEW.org_id AND x.session_id=NEW.session_id)
 OR NEW.accepted_fixes+NEW.rejected_accuracy+NEW.rejected_speed+NEW.rejected_outside<>t.stored OR NEW.received_fixes<>t.received
 OR NEW.duplicate_fixes<>t.duplicates OR NEW.outside_session_fixes<>t.outside OR NEW.mock_fixes<>t.mock OR NEW.device_dropped_accuracy<>t.d_accuracy
 OR NEW.device_dropped_stale<>t.d_stale OR NEW.device_dropped_jump<>t.d_jump OR NEW.device_dropped_mock<>t.d_mock
 OR NEW.coarse_fixes>(SELECT count(*) FROM public.org_tracking_segments g CROSS JOIN LATERAL unnest(g.coarse) AS c(v) WHERE g.org_id=NEW.org_id AND g.session_id=NEW.session_id AND c.v)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking summary input unavailable';END IF;
 SELECT * INTO previous FROM public.org_tracking_summaries WHERE org_id=NEW.org_id AND session_id=NEW.session_id ORDER BY revision DESC LIMIT 1;
 IF NEW.revision<>COALESCE(previous.revision,0)+1 OR(FOUND AND NOT(NEW.input_segments>=previous.input_segments AND NEW.input_transitions>=previous.input_transitions
  AND(NEW.input_segments,NEW.input_transitions)<>(previous.input_segments,previous.input_transitions)))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking summary superseded';END IF;
 n=cardinality(NEW.gap_from);gaps=0;
 FOR i IN 1..n LOOP
  IF NOT(NEW.gap_from[i]<NEW.gap_to[i]) OR(i>1 AND NEW.gap_from[i]<NEW.gap_to[i-1]) OR NOT isfinite(NEW.gap_from[i]) OR NOT isfinite(NEW.gap_to[i])
  OR NEW.gap_detected_by[i] NOT IN('DEVICE','SERVER') OR NEW.gap_reason[i] NOT IN('SIGNAL_LOST','NOT_STARTED','PERMISSION_DENIED','NOTIFICATION_BLOCKED','TRIP_PAUSED',
   'LONG_STATIONARY','PERMISSION_REVOKED','PROCESS_STOPPED','DEVICE_RESTARTED','STOPPED_BY_DRIVER','OWNER_CHANGED','TRIP_COMPLETED','TRIP_CANCELLED','MEMBERSHIP_ENDED','DEVICE_REVOKED')
  OR(NEW.gap_reason[i]='SIGNAL_LOST' AND NEW.gap_detected_by[i]<>'SERVER')
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking summary gaps unavailable';END IF;
  gaps=gaps+extract(epoch FROM NEW.gap_to[i]-NEW.gap_from[i]);
 END LOOP;
 IF NEW.gap_seconds<>LEAST(NEW.expected_seconds,ceil(gaps)::bigint) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking summary gaps unavailable';END IF;
 k=0;
 FOR i IN 1..cardinality(NEW.line_parts) LOOP
  IF NEW.line_parts[i]<2 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking summary geometry unavailable';END IF;
  k=k+NEW.line_parts[i];
 END LOOP;
 IF k<>cardinality(NEW.line_lat_e7) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking summary geometry unavailable';END IF;
 FOR i IN 1..k LOOP
  IF NEW.line_lat_e7[i] NOT BETWEEN -900000000 AND 900000000 OR NEW.line_lon_e7[i] NOT BETWEEN -1800000000 AND 1800000000
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tracking summary geometry unavailable';END IF;
 END LOOP;
 NEW.geometry=CASE WHEN k=0 THEN NULL ELSE public.ST_SetSRID(public.ST_Multi(public.ST_Collect(ARRAY(
  SELECT public.ST_MakeLine(ARRAY(SELECT public.ST_MakePoint(NEW.line_lon_e7[j]::double precision/10000000,NEW.line_lat_e7[j]::double precision/10000000)
   FROM generate_series(p.first,p.first+p.parts-1) AS j ORDER BY j))
  FROM(SELECT u.parts,1+COALESCE(sum(u.parts) OVER(ORDER BY u.k ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING),0)::integer AS first,u.k
   FROM unnest(NEW.line_parts) WITH ORDINALITY AS u(parts,k)) p ORDER BY p.k))),4326) END;
 NEW.processed_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_tracking_summary_guard BEFORE INSERT ON org_tracking_summaries FOR EACH ROW EXECUTE FUNCTION org_tracking_summary_guard();

ALTER TABLE org_tracking_summaries ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_tracking_summaries FORCE ROW LEVEL SECURITY;
CREATE POLICY org_tracking_summaries_scope ON org_tracking_summaries USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_tracking_summaries_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_tracking_summaries FOR EACH STATEMENT EXECUTE FUNCTION org_tracking_keep_history();
REVOKE ALL ON org_tracking_summaries FROM PUBLIC;
