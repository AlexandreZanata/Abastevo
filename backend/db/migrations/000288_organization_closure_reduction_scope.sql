-- EMP-ORG-013: preserve the ordinary erasure projection for open companies.
-- Only a closed organization can consult the terminal closure exception.
CREATE OR REPLACE FUNCTION org_membership_access_policy(org UUID,member UUID,action TEXT) RETURNS TEXT
 LANGUAGE plpgsql VOLATILE SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;
BEGIN
 IF org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RETURN 'UNAVAILABLE'; END IF;
 SELECT o.state INTO root_state FROM public.organizations o WHERE o.org_id=org FOR UPDATE;
 IF NOT FOUND THEN RETURN 'UNAVAILABLE'; END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=org AND id=member) THEN RETURN 'UNAVAILABLE'; END IF;
 IF action='ACTIVATE' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_role_assignments r WHERE r.org_id=org AND r.membership_id=member
   AND r.valid_from<=clock_timestamp() AND (r.valid_to IS NULL OR r.valid_to>clock_timestamp())
   AND (r.ended_at IS NULL OR r.ended_at>clock_timestamp())) THEN RETURN 'UNAVAILABLE'; END IF;
  IF EXISTS(SELECT 1 FROM public.org_role_assignments WHERE org_id=org AND membership_id=member AND role='OWNER')
   AND NOT public.org_access_owner_coverage(org,NULL,NULL,member) THEN RETURN 'LAST_OWNER'; END IF;
  RETURN 'ALLOW';
 ELSIF action='REDUCE' THEN
  IF root_state='CLOSED' THEN
   IF EXISTS(SELECT 1 FROM public.org_closure_requests c WHERE c.org_id=org AND c.state='COMPLETED') THEN
    RETURN 'ALLOW';
   END IF;
  END IF;
  IF EXISTS(SELECT 1 FROM public.org_role_assignments WHERE org_id=org AND membership_id=member AND role='OWNER')
   AND NOT public.org_access_owner_coverage(org,member,NULL,NULL) THEN RETURN 'LAST_OWNER'; END IF;
  RETURN 'ALLOW';
 END IF;
 RETURN 'UNAVAILABLE';
END $$;
