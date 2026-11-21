-- Apply as the migration owner after 000289, to a dedicated existing login.
-- psql "<owner DSN>" -v ON_ERROR_STOP=1 -v billing_binding_role=abastevo_billing_bind -f infra/db/billing-binding-role.sql
BEGIN;
SELECT pg_catalog.set_config('abastevo.billing_binding_role', :'billing_binding_role', true);

DO $billing_binding_role$
DECLARE
    role_name text := pg_catalog.current_setting('abastevo.billing_binding_role');
    policy_name text := 'billing_binding_' || role_name;
    role_oid oid;
BEGIN
    IF role_name !~ '^[a-z_][a-z0-9_]{0,46}$' OR role_name IN (current_user, session_user) THEN
        RAISE EXCEPTION 'a distinct lowercase billing binding login is required';
    END IF;
    SELECT oid INTO role_oid FROM pg_catalog.pg_roles WHERE rolname = role_name AND rolcanlogin
        AND NOT (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication);
    IF role_oid IS NULL OR EXISTS (SELECT 1 FROM pg_catalog.pg_auth_members WHERE member = role_oid OR roleid = role_oid)
       OR EXISTS (SELECT 1 FROM pg_catalog.pg_class WHERE relowner = role_oid)
       OR pg_catalog.has_schema_privilege(role_oid, 'public', 'CREATE')
       OR pg_catalog.has_table_privilege(role_oid, 'public.org_billing_settlement_receipts', 'SELECT,INSERT') THEN
        RAISE EXCEPTION 'billing binding requires an ordinary dedicated login without existing billing or schema authority';
    END IF;
    EXECUTE pg_catalog.format('DROP POLICY IF EXISTS %I ON public.org_audit_events', policy_name);
    EXECUTE pg_catalog.format($policy$
        CREATE POLICY %I ON public.org_audit_events AS RESTRICTIVE TO %I
        USING (action = 'subs.billing.charge.bound')
        WITH CHECK (action = 'subs.billing.charge.bound' AND target_type = 'subscription')
    $policy$, policy_name, role_name);
    EXECUTE pg_catalog.format('GRANT CONNECT ON DATABASE %I TO %I', pg_catalog.current_database(), role_name);
END
$billing_binding_role$;

GRANT USAGE ON SCHEMA public TO :"billing_binding_role";
GRANT SELECT ON TABLE public.organizations, public.enterprise_billing_offers,
    public.org_billing_charge_approvals TO :"billing_binding_role";
GRANT SELECT, INSERT ON TABLE public.org_audit_events,
    public.org_billing_charge_bindings TO :"billing_binding_role";
COMMIT;
