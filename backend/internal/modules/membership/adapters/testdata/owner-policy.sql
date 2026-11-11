-- Disposable PostGIS fixture only. This is not the production AUTH catalog.
CREATE TABLE org_test_role_assignments(
 org_id UUID NOT NULL,member_id UUID NOT NULL,role TEXT NOT NULL CHECK(role IN ('OWNER','MEMBER')),
 PRIMARY KEY(org_id,member_id),FOREIGN KEY(org_id,member_id) REFERENCES org_memberships(org_id,id)
);
ALTER TABLE org_test_role_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_test_role_assignments FORCE ROW LEVEL SECURITY;
CREATE POLICY test_roles_scope ON org_test_role_assignments
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE OR REPLACE FUNCTION org_membership_access_policy(org UUID,member UUID,action TEXT) RETURNS TEXT
 LANGUAGE plpgsql VOLATILE SET search_path=pg_catalog AS $$
DECLARE assigned TEXT;
BEGIN
 IF org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RETURN 'UNAVAILABLE'; END IF;
 SELECT role INTO assigned FROM public.org_test_role_assignments WHERE org_id=org AND member_id=member;
 IF assigned IS NULL THEN RETURN 'UNAVAILABLE'; END IF;
 IF action='ACTIVATE' THEN RETURN 'ALLOW'; END IF;
 IF action<>'REDUCE' THEN RETURN 'UNAVAILABLE'; END IF;
 IF assigned<>'OWNER' THEN RETURN 'ALLOW'; END IF;
 IF EXISTS(SELECT 1 FROM public.org_test_role_assignments r
  JOIN public.org_memberships m ON (m.org_id,m.id)=(r.org_id,r.member_id)
  JOIN public.accounts a ON a.id=m.account_id AND a.status='active'
  WHERE r.org_id=org AND r.member_id<>member AND r.role='OWNER' AND m.state='ACTIVE'
   AND NOT EXISTS(SELECT 1 FROM public.payment_billing_exits e WHERE e.account_id=m.account_id AND e.reason='ACCOUNT_DELETION')) THEN
  RETURN 'ALLOW';
 END IF;
 RETURN 'LAST_OWNER';
END $$;
