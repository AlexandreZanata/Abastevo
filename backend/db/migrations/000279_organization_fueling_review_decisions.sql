-- EMP-FUEL-003 and EMP-INTEG-003. Preserve the original signed fueling and
-- its expense. Record a reviewer opening and one same-revision decision.
ALTER TABLE org_fueling_review_entries
    ADD COLUMN warning_notes JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN fuel_entered_tank BOOLEAN,
    ADD CONSTRAINT org_fueling_review_actor_operation UNIQUE (org_id,operation_id),
    ADD CONSTRAINT org_fueling_review_audit_fkey FOREIGN KEY (org_id,operation_id)
        REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
    DROP CONSTRAINT org_fueling_review_entries_submit,
    ADD CONSTRAINT org_fueling_review_shape CHECK (
        jsonb_typeof(warning_notes)='object'
        AND (kind='APPROVE' OR warning_notes='{}')
        AND (kind='REJECT')=(fuel_entered_tank IS NOT NULL)
        AND revision=1
        AND ((sequence=1 AND kind='SUBMIT' AND state='SUBMITTED' AND reason='')
          OR (sequence=2 AND kind='OPEN' AND state='IN_REVIEW' AND reason='')
          OR (sequence=3 AND kind='APPROVE' AND state='APPROVED' AND (reason='' OR org_asset_text_valid(reason,280,true)))
          OR (sequence=3 AND (kind,state) IN (('RETURN','RETURNED'),('REJECT','REJECTED')) AND org_asset_text_valid(reason,280,true)))
    );

CREATE FUNCTION org_fueling_duplicate_pending(org UUID, fueling UUID) RETURNS BOOLEAN LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
    SELECT EXISTS (
        SELECT 1 FROM public.org_fuelings f
        JOIN public.organizations root ON root.org_id=f.org_id
        JOIN public.org_fuelings other ON (other.org_id,other.asset_id,other.product)=(f.org_id,f.asset_id,f.product) AND other.id<>f.id
        LEFT JOIN public.org_finding_settings cfg ON cfg.org_id=f.org_id
        WHERE (f.org_id,f.id)=(org,fueling)
        AND (SELECT r.state FROM public.org_fueling_review_entries r WHERE (r.org_id,r.fueling_id)=(other.org_id,other.id)
             ORDER BY r.sequence DESC LIMIT 1) NOT IN ('REJECTED','CANCELLED')
        AND (((other.occurred_at BETWEEN f.occurred_at-make_interval(mins=>COALESCE(cfg.duplicate_window_minutes,30))
                                      AND f.occurred_at+make_interval(mins=>COALESCE(cfg.duplicate_window_minutes,30)))
                AND (other.quantity=f.quantity OR other.paid_centavos=f.paid_centavos))
            OR (timezone(root.time_zone,other.occurred_at)::date=timezone(root.time_zone,f.occurred_at)::date
                AND other.quantity=f.quantity AND other.paid_centavos=f.paid_centavos))
    )
$$;

CREATE FUNCTION org_fueling_review_decision_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE f public.org_fuelings%ROWTYPE; actor public.org_memberships%ROWTYPE; person UUID; last_sequence BIGINT; last_state TEXT;
BEGIN
    IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
    END IF;
    SELECT * INTO f FROM public.org_fuelings WHERE (org_id,id)=(NEW.org_id,NEW.fueling_id);
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='fueling review unavailable';
    END IF;
    IF NEW.kind='SUBMIT' THEN
        IF (NEW.operation_id,NEW.actor_membership_id)<>(f.operation_id,f.author_membership_id) THEN
            RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fueling review source unavailable';
        END IF;
        NEW.recorded_at=clock_timestamp();
        RETURN NEW;
    END IF;
    PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot AND state='ACTIVE' FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fueling review unavailable';
    END IF;
    SELECT * INTO actor FROM public.org_memberships WHERE (org_id,id)=(NEW.org_id,NEW.actor_membership_id)
        AND state='ACTIVE' AND account_id IS NOT NULL;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fueling reviewer unavailable';
    END IF;
    person=public.org_sync_review_employee_canonical(NEW.org_id,actor.employee_id);
    IF person IS NULL OR person<>actor.employee_id OR person IS NOT DISTINCT FROM public.org_sync_review_employee_canonical(NEW.org_id,f.author_employee_id)
        OR person IS NOT DISTINCT FROM public.org_sync_review_employee_canonical(NEW.org_id,f.responsible_employee_id)
        OR EXISTS (SELECT 1 FROM public.org_memberships m WHERE (m.org_id,m.id,m.account_id)=(f.org_id,f.author_membership_id,actor.account_id)) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fueling reviewer unavailable';
    END IF;
    SELECT r.sequence,r.state INTO last_sequence,last_state FROM public.org_fueling_review_entries r
        WHERE (r.org_id,r.fueling_id)=(NEW.org_id,NEW.fueling_id) ORDER BY r.sequence DESC LIMIT 1;
    IF NOT FOUND OR NEW.sequence<>last_sequence+1 OR (last_state,NEW.kind) NOT IN (
        ('SUBMITTED','OPEN'),('IN_REVIEW','APPROVE'),('IN_REVIEW','RETURN'),('IN_REVIEW','REJECT')
    ) OR NOT EXISTS (
        SELECT 1 FROM public.org_audit_events a WHERE (a.org_id,a.operation_id,a.actor_id,a.target_id)=(NEW.org_id,NEW.operation_id,actor.account_id,NEW.fueling_id)
          AND a.target_type='fueling.review' AND a.action='fueling.review.'||lower(NEW.kind)
    ) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fueling review decision unavailable';
    END IF;
    IF NEW.kind='APPROVE' AND (public.org_fueling_duplicate_pending(NEW.org_id,NEW.fueling_id)
        OR EXISTS (
            SELECT 1 FROM public.org_fueling_warnings w WHERE (w.org_id,w.fueling_id,w.revision)=(NEW.org_id,NEW.fueling_id,NEW.revision)
            AND NOT (NEW.warning_notes ? w.code))
        OR EXISTS (
            SELECT 1 FROM jsonb_each(NEW.warning_notes) n WHERE jsonb_typeof(n.value)<>'string'
                OR NOT public.org_asset_text_valid(n.value#>>'{}',280,true)
                OR NOT EXISTS (SELECT 1 FROM public.org_fueling_warnings w
                               WHERE (w.org_id,w.fueling_id,w.revision,w.code)=(NEW.org_id,NEW.fueling_id,NEW.revision,n.key)))
    ) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fueling review evidence unavailable';
    END IF;
    NEW.recorded_at=clock_timestamp();
    RETURN NEW;
END $$;
DROP TRIGGER org_fueling_review_source ON org_fueling_review_entries;
CREATE TRIGGER org_fueling_review_source BEFORE INSERT ON org_fueling_review_entries
    FOR EACH ROW EXECUTE FUNCTION org_fueling_review_decision_source();
