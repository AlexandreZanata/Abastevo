-- Apply after 000294 as migration owner to a dedicated existing ordinary login.
-- psql "<owner DSN>" -v ON_ERROR_STOP=1 -v data_rights_role=abastevo_data_rights -f infra/db/data-rights-role.sql
BEGIN;
SELECT pg_catalog.set_config('abastevo.data_rights_role', :'data_rights_role', true);

DO $data_rights_role$
DECLARE
    role_name text := pg_catalog.current_setting('abastevo.data_rights_role');
    role_oid oid;
BEGIN
    IF role_name !~ '^[a-z_][a-z0-9_]{0,46}$' OR role_name IN (current_user, session_user) THEN
        RAISE EXCEPTION 'a distinct lowercase Data rights login is required';
    END IF;
    SELECT oid INTO role_oid FROM pg_catalog.pg_roles WHERE rolname = role_name AND rolcanlogin
        AND NOT (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication);
    IF role_oid IS NULL OR EXISTS (SELECT 1 FROM pg_catalog.pg_auth_members WHERE member = role_oid OR roleid = role_oid)
       OR EXISTS (SELECT 1 FROM pg_catalog.pg_class WHERE relowner = role_oid)
       OR pg_catalog.has_schema_privilege(role_oid, 'public', 'CREATE')
       OR pg_catalog.has_table_privilege(role_oid, 'public.enterprise_data_source_reviews', 'INSERT')
       OR pg_catalog.has_table_privilege(role_oid, 'public.organizations', 'SELECT,INSERT,UPDATE,DELETE') THEN
        RAISE EXCEPTION 'Data rights recording requires a dedicated ordinary login';
    END IF;
    EXECUTE pg_catalog.format('GRANT CONNECT ON DATABASE %I TO %I', pg_catalog.current_database(), role_name);
END
$data_rights_role$;

GRANT USAGE ON SCHEMA public TO :"data_rights_role";
GRANT SELECT ON TABLE public.enterprise_data_source_reviews TO :"data_rights_role";
GRANT INSERT (operation_id,source_code,source_digest,revision,status,decision_ref,license_ref,attribution,snapshot_digest,official_revision)
    ON TABLE public.enterprise_data_source_reviews TO :"data_rights_role";
COMMIT;
