-- EMP-METER-006 (B-BR-EMP-487/488, EDR-315/317). The current value of every
-- meter as a maintained projection: written by the database in the
-- transaction of every reading fact, never the source of truth, rebuilt from
-- the readings per organization by the operator rebuild (forced RLS lets no
-- migration enumerate organizations, so existing meters are filled by it).
CREATE TABLE org_meter_current_values(
 org_id UUID NOT NULL,meter_id UUID NOT NULL,segment_id UUID,reading_id UUID,value BIGINT CHECK(value>=0),
 occurred_at TIMESTAMPTZ,occurred_submicro SMALLINT CHECK(occurred_submicro BETWEEN 0 AND 999),
 revision BIGINT NOT NULL CHECK(revision>=1),
 PRIMARY KEY(org_id,meter_id),
 CONSTRAINT org_meter_current_values_meter_fkey FOREIGN KEY(org_id,meter_id) REFERENCES org_meters(org_id,id),
 CONSTRAINT org_meter_current_values_segment_fkey FOREIGN KEY(org_id,meter_id,segment_id) REFERENCES org_meter_segments(org_id,meter_id,id),
 CONSTRAINT org_meter_current_values_reading_fkey FOREIGN KEY(org_id,meter_id,reading_id) REFERENCES org_meter_readings(org_id,meter_id,id),
 CONSTRAINT org_meter_current_values_shape CHECK((reading_id IS NULL)=(value IS NULL) AND(reading_id IS NULL)=(occurred_at IS NULL) AND(reading_id IS NULL)=(occurred_submicro IS NULL)
 AND(reading_id IS NULL OR segment_id IS NOT NULL))
);
ALTER TABLE org_meter_current_values ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_meter_current_values FORCE ROW LEVEL SECURITY;
CREATE POLICY org_meter_current_values_scope ON org_meter_current_values USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_meter_current_values FROM PUBLIC;

-- The revision counts the meter's source facts: readings, history revisions,
-- segments, closures and the removal. Incremental and rebuilt rows agree.
CREATE FUNCTION org_meter_current_revision(selected_org UUID,selected_meter UUID) RETURNS BIGINT
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT(SELECT count(*) FROM public.org_meter_readings WHERE org_id=selected_org AND meter_id=selected_meter)
 +(SELECT count(*) FROM public.org_meter_reading_states WHERE org_id=selected_org AND meter_id=selected_meter)
 +(SELECT count(*)+count(closed_at) FROM public.org_meter_segments WHERE org_id=selected_org AND meter_id=selected_meter)
 +(SELECT count(removed_at) FROM public.org_meters WHERE org_id=selected_org AND id=selected_meter)
$$;

CREATE FUNCTION org_meter_current_refresh(selected_org UUID,selected_meter UUID) RETURNS VOID
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE open_segment UUID;latest public.org_meter_readings%ROWTYPE;facts BIGINT;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT s.id INTO open_segment FROM public.org_meter_segments s JOIN public.org_meters m ON(m.org_id,m.id)=(s.org_id,s.meter_id)
 WHERE s.org_id=selected_org AND s.meter_id=selected_meter AND s.closed_at IS NULL AND m.removed_at IS NULL;
 IF open_segment IS NOT NULL THEN
  SELECT r.* INTO latest FROM public.org_meter_readings r WHERE r.org_id=selected_org AND r.meter_id=selected_meter
  AND public.org_meter_reading_segment(r.org_id,r.meter_id,r.id)=open_segment AND public.org_meter_reading_state(r.org_id,r.meter_id,r.id)='VALID'
  ORDER BY r.occurred_at DESC,r.occurred_submicro DESC,r.received_at DESC,r.id DESC LIMIT 1;
 END IF;
 facts=public.org_meter_current_revision(selected_org,selected_meter);
 IF facts<1 THEN RETURN;END IF;
 INSERT INTO public.org_meter_current_values(org_id,meter_id,segment_id,reading_id,value,occurred_at,occurred_submicro,revision)
 VALUES(selected_org,selected_meter,open_segment,latest.id,latest.value,latest.occurred_at,latest.occurred_submicro,facts)
 ON CONFLICT(org_id,meter_id) DO UPDATE SET segment_id=EXCLUDED.segment_id,reading_id=EXCLUDED.reading_id,value=EXCLUDED.value,
 occurred_at=EXCLUDED.occurred_at,occurred_submicro=EXCLUDED.occurred_submicro,revision=EXCLUDED.revision;
END $$;
REVOKE ALL ON FUNCTION org_meter_current_refresh(UUID,UUID) FROM PUBLIC;

CREATE FUNCTION org_meter_current_source() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
BEGIN
 IF TG_TABLE_NAME='org_meters' THEN PERFORM public.org_meter_current_refresh(NEW.org_id,NEW.id);
 ELSE PERFORM public.org_meter_current_refresh(NEW.org_id,NEW.meter_id);END IF;
 RETURN NULL;
END $$;
REVOKE ALL ON FUNCTION org_meter_current_source() FROM PUBLIC;
CREATE TRIGGER org_meter_current_reading AFTER INSERT ON org_meter_readings FOR EACH ROW EXECUTE FUNCTION org_meter_current_source();
CREATE TRIGGER org_meter_current_state AFTER INSERT ON org_meter_reading_states FOR EACH ROW EXECUTE FUNCTION org_meter_current_source();
CREATE TRIGGER org_meter_current_segment AFTER INSERT OR UPDATE ON org_meter_segments FOR EACH ROW EXECUTE FUNCTION org_meter_current_source();
CREATE TRIGGER org_meter_current_removal AFTER UPDATE ON org_meters FOR EACH ROW EXECUTE FUNCTION org_meter_current_source();

-- The operator rebuild (EDR-317): one organization, serialised with every
-- writer on the organization root, rows deleted and recomputed.
CREATE FUNCTION org_meter_current_rebuild(selected_org UUID) RETURNS BIGINT
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE selected_meter UUID;n BIGINT=0;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 DELETE FROM public.org_meter_current_values WHERE org_id=selected_org;
 FOR selected_meter IN SELECT id FROM public.org_meters WHERE org_id=selected_org ORDER BY id LOOP
  PERFORM public.org_meter_current_refresh(selected_org,selected_meter);n=n+1;
 END LOOP;
 RETURN n;
END $$;
REVOKE ALL ON FUNCTION org_meter_current_rebuild(UUID) FROM PUBLIC;
