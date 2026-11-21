-- Apply as the migration owner after 000297 to a dedicated ordinary login.
BEGIN;
SELECT pg_catalog.set_config('abastevo.billing_suspension_role', :'billing_suspension_role', true);

DO $billing_suspension_role$
DECLARE
    role_name text := pg_catalog.current_setting('abastevo.billing_suspension_role');
    role_oid oid;
    policy_name text := 'billing_suspension_' || role_name;
BEGIN
    IF role_name !~ '^[a-z_][a-z0-9_]{0,43}$' OR role_name IN (current_user, session_user) THEN
        RAISE EXCEPTION 'a distinct lowercase suspension login is required';
    END IF;
    SELECT oid INTO role_oid FROM pg_catalog.pg_roles WHERE rolname = role_name AND rolcanlogin
        AND NOT (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication);
    IF role_oid IS NULL OR EXISTS (SELECT 1 FROM pg_catalog.pg_auth_members WHERE member = role_oid OR roleid = role_oid)
       OR EXISTS (SELECT 1 FROM pg_catalog.pg_class WHERE relowner = role_oid)
       OR pg_catalog.has_schema_privilege(role_oid, 'public', 'CREATE')
       OR pg_catalog.has_table_privilege(role_oid, 'public.org_billing_settlement_receipts', 'SELECT,INSERT') THEN
        RAISE EXCEPTION 'billing suspension requires a dedicated ordinary login';
    END IF;
    EXECUTE pg_catalog.format('DROP POLICY IF EXISTS %I ON public.org_audit_events', policy_name);
    EXECUTE pg_catalog.format($policy$
        CREATE POLICY %I ON public.org_audit_events AS RESTRICTIVE TO %I
        USING (action = 'subs.billing.suspended')
        WITH CHECK (action = 'subs.billing.suspended' AND target_type = 'subscription')
    $policy$, policy_name, role_name);
    EXECUTE pg_catalog.format('DROP POLICY IF EXISTS %I ON public.org_billing_subscription_events', policy_name);
    EXECUTE pg_catalog.format($policy$
        CREATE POLICY %I ON public.org_billing_subscription_events AS RESTRICTIVE FOR INSERT TO %I
        WITH CHECK (event_kind = 'SUSPEND_ACCESS' AND event_source = 'REVIEWED_OPERATOR'
            AND from_state = 'PAST_DUE' AND state = 'SUSPENDED'
            AND EXISTS (
                SELECT 1 FROM public.org_billing_suspension_approvals a
                JOIN public.org_audit_events e ON (e.org_id,e.target_id,e.actor_id) =
                    (a.org_id,a.subscription_id,a.operator_id)
                WHERE a.org_id = org_billing_subscription_events.org_id
                  AND a.subscription_id = org_billing_subscription_events.subscription_id
                  AND a.id::text = evidence_reference AND a.expected_revision = revision - 1
                  AND occurred_at >= a.valid_from AND occurred_at < a.valid_until
                  AND e.action = 'subs.billing.suspended' AND e.target_type = 'subscription'
                  AND e.request_hash = pg_catalog.encode(pg_catalog.sha256(pg_catalog.convert_to(
                      'subs.billing.suspended/v1/' || a.org_id::text || '/' || a.id::text, 'UTF8')), 'hex')
            ))
    $policy$, policy_name, role_name);
    EXECUTE pg_catalog.format('GRANT CONNECT ON DATABASE %I TO %I', pg_catalog.current_database(), role_name);
END
$billing_suspension_role$;

GRANT USAGE ON SCHEMA public TO :"billing_suspension_role";
GRANT SELECT ON TABLE public.org_billing_suspension_approvals, public.enterprise_plan_catalog
    TO :"billing_suspension_role";
GRANT SELECT, INSERT ON TABLE public.org_audit_events, public.org_billing_subscription_events
    TO :"billing_suspension_role";
COMMIT;
