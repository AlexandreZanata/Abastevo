-- EMP-SUBS-005 and VAL-35: preserve signed fueling facts after plan loss.
CREATE TABLE org_held_fuelings (
    org_id UUID NOT NULL,
    review_item_id UUID NOT NULL,
    operation_id UUID NOT NULL,
    fueling_id UUID NOT NULL,
    asset_id UUID NOT NULL,
    responsible_employee_id UUID NOT NULL,
    canonical_operation BYTEA NOT NULL CHECK (octet_length(canonical_operation) BETWEEN 1 AND 8192),
    PRIMARY KEY (org_id,review_item_id),
    UNIQUE (org_id,operation_id),
    UNIQUE (org_id,fueling_id),
    FOREIGN KEY (org_id,review_item_id,operation_id) REFERENCES org_sync_review_items(org_id,id,operation_id),
    FOREIGN KEY (org_id,operation_id) REFERENCES org_sync_state_intakes(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (org_id,asset_id) REFERENCES org_assets(org_id,id),
    FOREIGN KEY (org_id,responsible_employee_id) REFERENCES org_employee_identities(org_id,id)
);

ALTER TABLE org_held_fuelings ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_held_fuelings FORCE ROW LEVEL SECURITY;
CREATE POLICY org_held_fuelings_scope ON org_held_fuelings
    USING (org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
    WITH CHECK (org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_held_fuelings_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_held_fuelings
    FOR EACH STATEMENT EXECUTE FUNCTION org_sync_keep_history();
REVOKE ALL ON org_held_fuelings FROM PUBLIC;

CREATE FUNCTION org_fueling_plan_hold_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE envelope JSONB; operation public.org_sync_operations%ROWTYPE;
BEGIN
    PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held fueling unavailable';
    END IF;
    SELECT o.* INTO operation FROM public.org_sync_operations o
    JOIN public.org_sync_review_items i ON (o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=
        (i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
    JOIN public.org_memberships m ON (m.org_id,m.id,m.account_id,m.employee_id)=(o.org_id,o.membership_id,o.owner_account_id,i.author_employee_id)
    JOIN public.org_device_enrollments e ON (e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
    WHERE (i.org_id,i.id,i.operation_id)=(NEW.org_id,NEW.review_item_id,NEW.operation_id) AND i.code='PLAN_CHANGED'
      AND o.operation_type='fueling.submit' AND o.operation_schema=1 AND o.entity_type='fueling' AND o.entity_id=NEW.fueling_id
      AND m.state='ACTIVE' AND e.state='ENROLLED';
    IF NOT FOUND OR EXISTS (
        SELECT 1 FROM public.org_fuelings f WHERE f.org_id=NEW.org_id AND (f.id=NEW.fueling_id OR f.operation_id=NEW.operation_id)
    ) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held fueling source unavailable';
    END IF;
    envelope=pg_catalog.convert_from(NEW.canonical_operation,'UTF8')::jsonb;
    IF (envelope->>'org_id',envelope->>'operation_id',envelope->>'owner_account_id',envelope->>'api_origin',
        envelope->>'membership_id',envelope->>'enrollment_id',envelope->>'type',envelope->>'operation_schema',
        envelope->'entity'->>'type',envelope->'entity'->>'id',envelope->>'payload_digest',
        envelope->'payload'->>'asset_id',envelope->'payload'->>'responsible_employee_id')
       IS DISTINCT FROM (NEW.org_id::text,NEW.operation_id::text,operation.owner_account_id::text,operation.api_origin,
        operation.membership_id::text,operation.enrollment_id::text,'fueling.submit','1','fueling',NEW.fueling_id::text,
        operation.payload_digest,NEW.asset_id::text,NEW.responsible_employee_id::text) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held fueling facts unavailable';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_held_fueling_source BEFORE INSERT ON org_held_fuelings
    FOR EACH ROW EXECUTE FUNCTION org_fueling_plan_hold_source();
CREATE CONSTRAINT TRIGGER org_held_fueling_complete AFTER INSERT ON org_held_fuelings
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_review_complete();

CREATE FUNCTION org_fueling_respect_held_identity() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM public.org_held_fuelings h WHERE h.org_id=NEW.org_id AND h.fueling_id=NEW.id) THEN
        RAISE EXCEPTION USING ERRCODE='23505',MESSAGE='fueling review identity reserved';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_fuelings_retained_hold BEFORE INSERT ON org_fuelings
    FOR EACH ROW EXECUTE FUNCTION org_fueling_respect_held_identity();

-- Preserve every prior owner and require a historical subscription for fueling.
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
 OR(owner_type IN('trip.pause','trip.resume','trip.cancel') AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_trip_held_transitions h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='trip' AND o.entity_id=h.trip_id
 AND owner_type='trip.'||lower(h.command)))
 OR(owner_type='fueling.submit' AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i
 JOIN public.org_held_fuelings h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_state_intakes s ON(s.org_id,s.operation_id)=(i.org_id,i.operation_id)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema
   AND o.entity_type='fueling' AND o.entity_id=h.fueling_id
   AND s.condition='PLAN' AND s.code='PLAN_CHANGED' AND s.subscription_id IS NOT NULL))
 $$;
