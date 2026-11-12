-- EMP-IDN-010. Actual membership reductions and immutable reviewed handover.
CREATE TABLE org_offboarding_commands(
 org_id UUID NOT NULL,operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_account_id UUID NOT NULL REFERENCES accounts(id),membership_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('OFFBOARD','LEAVE')),request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 reason TEXT NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 280 AND reason !~ '[[:cntrl:]]'),
 result JSONB NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=131072),
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(occurred_at)),
 PRIMARY KEY(org_id,operation_id),UNIQUE(org_id,membership_id),FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE org_offboarding_notices(
 org_id UUID NOT NULL,operation_id UUID NOT NULL,
 delivery_state TEXT NOT NULL DEFAULT 'PENDING_NOTIFICATION_CORE' CHECK(delivery_state='PENDING_NOTIFICATION_CORE'),
 PRIMARY KEY(org_id,operation_id),FOREIGN KEY(org_id,operation_id) REFERENCES org_offboarding_commands(org_id,operation_id)
);
ALTER TABLE org_offboarding_commands ENABLE ROW LEVEL SECURITY;ALTER TABLE org_offboarding_commands FORCE ROW LEVEL SECURITY;
ALTER TABLE org_offboarding_notices ENABLE ROW LEVEL SECURITY;ALTER TABLE org_offboarding_notices FORCE ROW LEVEL SECURITY;
CREATE POLICY org_offboarding_scope ON org_offboarding_commands USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_offboarding_notice_scope ON org_offboarding_notices USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_offboarding_commands,org_offboarding_notices FROM PUBLIC;
CREATE TRIGGER org_offboarding_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_offboarding_commands FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_offboarding_notice_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_offboarding_notices FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE FUNCTION org_offboarding_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE item JSONB;k TEXT;parts TEXT[];
BEGIN
 IF NEW.result-ARRAY['membership_id','employee_id','access_model','state','revision','suspension_origin','end_reason','employee_state','employee_revision','handover_digest','items']<>'{}'::jsonb
 OR NOT(NEW.result ?& ARRAY['membership_id','employee_id','access_model','state','revision','suspension_origin','end_reason','employee_state','employee_revision','handover_digest','items'])
 OR jsonb_typeof(NEW.result->'items')<>'array' OR jsonb_array_length(NEW.result->'items')>500
 OR NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
 WHERE m.org_id=NEW.org_id AND m.id=NEW.membership_id AND m.access_model='EXISTING_ACCOUNT' AND m.state='ENDED'
 AND NEW.result->>'membership_id'=m.id::text AND NEW.result->>'employee_id'=m.employee_id::text AND NEW.result->>'access_model'=m.access_model AND NEW.result->>'state'=m.state
 AND NEW.result->>'revision'=m.revision::text AND NEW.result->>'suspension_origin'=m.suspension_origin AND NEW.result->>'end_reason'=m.end_reason
 AND m.end_reason=CASE NEW.kind WHEN 'LEAVE' THEN 'LEFT' ELSE 'OFFBOARDED' END
 AND(NEW.kind<>'LEAVE' OR m.account_id=NEW.actor_account_id)
 AND NEW.result->>'employee_state'=e.state AND NEW.result->>'employee_revision'=e.revision::text)
 OR NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.actor_id=NEW.actor_account_id AND a.target_type='membership' AND a.target_id=NEW.membership_id AND a.request_hash=NEW.request_hash AND a.action='idn.membership.ended' AND a.reason_code=CASE NEW.kind WHEN 'LEAVE' THEN 'left' ELSE 'offboarded' END)
 THEN RAISE EXCEPTION USING ERRCODE='P9002',MESSAGE='offboarding source unavailable';END IF;
 -- Flags reference actual retained source facts after their reductions, not
 -- client-created work or a claim about modules that are not installed.
 FOR item IN SELECT value FROM jsonb_array_elements(NEW.result->'items') LOOP
  k=item->>'key';parts=string_to_array(k,':');
  IF item-ARRAY['key','source_revision']<>'{}'::jsonb OR NOT(item ?& ARRAY['key','source_revision']) OR NOT(
   (parts[1]='TEAM_LINK' AND array_length(parts,1)=2 AND item->>'source_revision'='1' AND EXISTS(SELECT 1 FROM public.org_team_links l JOIN public.org_memberships m ON(m.org_id,m.employee_id)=(l.org_id,l.employee_id) WHERE m.org_id=NEW.org_id AND m.id=NEW.membership_id AND l.id::text=parts[2] AND l.end_reason='MEMBERSHIP_ENDED' AND l.source_membership_id=NEW.membership_id AND l.source_membership_revision::text=NEW.result->>'revision'))
   OR(parts[1]='SETTING' AND array_length(parts,1)=3 AND EXISTS(SELECT 1 FROM public.org_settings_designations d WHERE d.org_id=NEW.org_id AND d.member_id=NEW.membership_id AND d.settings_revision::text=item->>'source_revision' AND d.responsibility=parts[2] AND d.slot=parts[3] AND EXISTS(SELECT 1 FROM public.org_settings_versions v WHERE v.org_id=d.org_id AND v.revision=d.settings_revision+1 AND v.source='MEMBERSHIP_REDUCTION' AND v.source_membership_id=NEW.membership_id AND v.source_membership_revision::text=NEW.result->>'revision'))))
  THEN RAISE EXCEPTION USING ERRCODE='P9002',MESSAGE='handover source unavailable';END IF;
 END LOOP;
 NEW.occurred_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_offboarding_guard BEFORE INSERT ON org_offboarding_commands FOR EACH ROW EXECUTE FUNCTION org_offboarding_guard();
