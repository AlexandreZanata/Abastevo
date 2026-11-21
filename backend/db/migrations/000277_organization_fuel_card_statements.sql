-- EMP-INTEG-003: one retained reconciliation per approved source transaction.
CREATE TABLE org_card_statement_lines (
    org_id UUID NOT NULL,
    connection_id UUID NOT NULL,
    provider_code TEXT NOT NULL CHECK (octet_length(provider_code) BETWEEN 1 AND 64 AND provider_code ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
    source_account_id UUID NOT NULL CHECK (source_account_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    transaction_hash TEXT NOT NULL CHECK (transaction_hash ~ '^[a-f0-9]{64}$'),
    fact_hash TEXT NOT NULL CHECK (fact_hash ~ '^[a-f0-9]{64}$'),
    occurred_at TIMESTAMPTZ NOT NULL CHECK (isfinite(occurred_at)),
    occurred_submicro SMALLINT NOT NULL CHECK (occurred_submicro BETWEEN 0 AND 999),
    paid_centavos BIGINT NOT NULL CHECK (paid_centavos BETWEEN 1 AND 1000000000000),
    fueling_id UUID,
    confirmed_by UUID NOT NULL,
    operation_id UUID NOT NULL,
    confirmed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(confirmed_at)),
    PRIMARY KEY (org_id,provider_code,source_account_id,transaction_hash),
    UNIQUE (org_id,fueling_id),
    FOREIGN KEY (org_id,connection_id) REFERENCES org_connections(org_id,id),
    FOREIGN KEY (org_id,fueling_id) REFERENCES org_fuelings(org_id,id),
    FOREIGN KEY (org_id,confirmed_by) REFERENCES org_memberships(org_id,id),
    FOREIGN KEY (org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);

CREATE FUNCTION org_card_statement_proof() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND state='ACTIVE' AND has_snapshot FOR UPDATE;
    IF NOT FOUND OR NOT EXISTS (
        SELECT 1 FROM public.org_connections c
        JOIN public.org_connector_approvals a ON (a.org_id,a.id,a.revision)=(c.org_id,c.approval_id,c.approval_revision)
        WHERE (c.org_id,c.id)=(NEW.org_id,NEW.connection_id) AND c.state='ACTIVE'
          AND a.kind='FUEL_CARD_STATEMENT' AND a.decision='APPROVED' AND a.ready_at<=clock_timestamp()
          AND (a.provider_code,a.resource_id)=(NEW.provider_code,NEW.source_account_id)
          AND NOT EXISTS (SELECT 1 FROM public.org_connector_approvals n
                          WHERE (n.org_id,n.id)=(a.org_id,a.id) AND n.revision>a.revision)
    ) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='card statement connection unavailable';
    END IF;
    IF NEW.fueling_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM public.org_fuelings f
        WHERE (f.org_id,f.id)=(NEW.org_id,NEW.fueling_id) AND f.payer='COMPANY' AND f.payment_method='FUEL_CARD'
          AND (f.occurred_at,f.occurred_submicro,f.paid_centavos)=(NEW.occurred_at,NEW.occurred_submicro,NEW.paid_centavos)
          AND (SELECT e.state FROM public.org_fueling_review_entries e WHERE (e.org_id,e.fueling_id)=(f.org_id,f.id)
               ORDER BY e.sequence DESC LIMIT 1)='APPROVED'
    ) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='card statement match unavailable';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_card_statement_proof BEFORE INSERT ON org_card_statement_lines
    FOR EACH ROW EXECUTE FUNCTION org_card_statement_proof();
ALTER TABLE org_card_statement_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_card_statement_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY org_card_statement_lines_scope ON org_card_statement_lines
    USING (org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
    WITH CHECK (org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_card_statement_lines_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_card_statement_lines
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_plan_catalog_immutable();
REVOKE ALL ON org_card_statement_lines FROM PUBLIC;
