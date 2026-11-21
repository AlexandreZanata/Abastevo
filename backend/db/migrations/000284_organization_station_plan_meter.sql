-- EMP-SUBS-009: independently reviewed links from scoped establishments
-- to the station profile's representation and operator sources.
CREATE TABLE org_billing_station_plan_reviews (
    org_id UUID NOT NULL REFERENCES organizations(org_id),
    station_id UUID NOT NULL REFERENCES directory_stations(id),
    revision BIGINT NOT NULL CHECK (revision>0),
    approval_id UUID NOT NULL CHECK (approval_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    establishment_id UUID NOT NULL CHECK (establishment_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    grant_id UUID NOT NULL REFERENCES representation_grants(id),
    grant_version INTEGER NOT NULL CHECK (grant_version>0),
    membership_id UUID NOT NULL CHECK (membership_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    membership_revision BIGINT NOT NULL CHECK (membership_revision>0),
    operator_revision_id UUID NOT NULL REFERENCES station_operator_revisions(id),
    approved_operator UUID NOT NULL CHECK (approved_operator<>'00000000-0000-0000-0000-000000000000'::uuid),
    decision TEXT NOT NULL CHECK (decision IN ('APPROVED','WITHDRAWN')),
    decision_ref TEXT NOT NULL CHECK (decision_ref ~ '^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$'),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(recorded_at)),
    PRIMARY KEY (org_id,station_id,revision),
    UNIQUE (org_id,approval_id)
);
CREATE TABLE org_billing_station_plan_admissions (
    org_id UUID NOT NULL,
    operation_id UUID NOT NULL,
    station_id UUID NOT NULL,
    review_revision BIGINT NOT NULL,
    admitted_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(admitted_at)),
    PRIMARY KEY (org_id,operation_id),
    UNIQUE (org_id,station_id,review_revision),
    FOREIGN KEY (org_id,station_id,review_revision)
        REFERENCES org_billing_station_plan_reviews(org_id,station_id,revision),
    FOREIGN KEY (org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
DO $$
DECLARE relation TEXT;
BEGIN
    FOREACH relation IN ARRAY ARRAY['org_billing_station_plan_reviews','org_billing_station_plan_admissions'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',relation);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',relation);
        EXECUTE format('CREATE POLICY %I ON %I USING (org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK (org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',
            relation||'_scope',relation);
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_billing_subscription_events_immutable()',
            relation||'_retained',relation);
        EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',relation);
    END LOOP;
END $$;

CREATE FUNCTION org_billing_station_plan_reviews_at(target_org UUID, observed_at TIMESTAMPTZ)
RETURNS TABLE(station_id UUID, revision BIGINT, approval_id UUID, approved_operator UUID)
LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
    WITH latest AS (
        SELECT DISTINCT ON (r.station_id) r.*
        FROM public.org_billing_station_plan_reviews r
        WHERE r.org_id=target_org AND r.recorded_at<=observed_at
        ORDER BY r.station_id,r.revision DESC
    )
    SELECT r.station_id,r.revision,r.approval_id,r.approved_operator
    FROM latest r
    JOIN public.organizations company ON company.org_id=r.org_id AND company.state='ACTIVE'
    JOIN public.directory_stations station ON station.id=r.station_id AND station.status='active'
    JOIN public.representation_grants grant_row ON (grant_row.id,grant_row.station_id,grant_row.version)=(r.grant_id,r.station_id,r.grant_version)
    JOIN public.station_operator_revisions operator_row ON
        (operator_row.id,operator_row.station_id,operator_row.cnpj)=(r.operator_revision_id,r.station_id,grant_row.operator_cnpj)
    JOIN public.org_establishments establishment ON
        (establishment.org_id,establishment.id,establishment.cnpj)=(r.org_id,r.establishment_id,operator_row.cnpj)
    JOIN public.org_legal_entities legal ON
        (legal.org_id,legal.id)=(establishment.org_id,establishment.legal_entity_id) AND legal.state='DECLARED'
    JOIN public.accounts account ON account.id=grant_row.account_id AND account.status='active'
      AND (account.enterprise_managed_org_id IS NULL OR account.enterprise_managed_org_id=r.org_id)
    WHERE r.decision='APPROVED' AND grant_row.status='active'
      AND grant_row.valid_from<=observed_at AND (grant_row.valid_to IS NULL OR observed_at<grant_row.valid_to)
      AND operator_row.valid_from<=observed_at AND operator_row.valid_to IS NULL
      AND NOT EXISTS (SELECT 1 FROM public.payment_billing_exits billing_exit
          WHERE billing_exit.account_id=account.id AND billing_exit.reason='ACCOUNT_DELETION')
      AND EXISTS (SELECT 1 FROM public.org_memberships member
       WHERE (member.org_id,member.id,member.account_id,member.revision)=(r.org_id,r.membership_id,account.id,r.membership_revision)
         AND member.state='ACTIVE')
$$;
REVOKE ALL ON FUNCTION org_billing_station_plan_reviews_at(UUID,TIMESTAMPTZ) FROM PUBLIC;

CREATE FUNCTION org_billing_station_plan_admission_proof() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    NEW.admitted_at=clock_timestamp();
    IF NOT EXISTS (
        SELECT 1 FROM public.org_billing_station_plan_reviews_at(NEW.org_id,NEW.admitted_at) r
        JOIN public.org_audit_events a ON (a.org_id,a.operation_id,a.actor_id)=(NEW.org_id,NEW.operation_id,r.approved_operator)
        WHERE (r.station_id,r.revision)=(NEW.station_id,NEW.review_revision)
          AND a.action='subs.station.admitted' AND a.target_type='station' AND a.target_id=NEW.station_id
    ) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='company station admission proof unavailable';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_billing_station_plan_admission_proof BEFORE INSERT ON org_billing_station_plan_admissions
    FOR EACH ROW EXECUTE FUNCTION org_billing_station_plan_admission_proof();
