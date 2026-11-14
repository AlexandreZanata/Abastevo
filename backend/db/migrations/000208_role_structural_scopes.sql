-- EMP-AUTH-006 (EDR-160). Unit, cost-centre, team and asset role scopes.
-- Additive: existing ORG/SELF rows keep their values and their new generated
-- structural columns stay NULL; the previous binary writes only ORG/SELF. A
-- structural target is one same-organization unit, cost centre, team or asset
-- (composite foreign key per kind) and is admitted only while live. Ending an
-- assignment never re-checks its target. Assignments are not structure use:
-- archiving neither ends nor is blocked by them.
ALTER TABLE org_role_assignments
 DROP CONSTRAINT org_role_assignments_scope_type_check,
 DROP CONSTRAINT org_role_assignments_check1,
 ADD CONSTRAINT org_role_assignments_scope_type_check CHECK(scope_type IN('ORG','SELF','UNIT','COST_CENTRE','TEAM','ASSET')),
 ADD CONSTRAINT org_role_assignments_scope_target_check CHECK((scope_type='ORG' AND scope_target=org_id) OR(scope_type='SELF' AND scope_target=membership_id)
  OR(scope_type IN('UNIT','COST_CENTRE','TEAM','ASSET') AND scope_target<>org_id AND scope_target<>membership_id)),
 ADD COLUMN scope_unit_id UUID GENERATED ALWAYS AS(CASE WHEN scope_type='UNIT' THEN scope_target END) STORED,
 ADD COLUMN scope_cost_centre_id UUID GENERATED ALWAYS AS(CASE WHEN scope_type='COST_CENTRE' THEN scope_target END) STORED,
 ADD COLUMN scope_team_id UUID GENERATED ALWAYS AS(CASE WHEN scope_type='TEAM' THEN scope_target END) STORED,
 ADD COLUMN scope_asset_id UUID GENERATED ALWAYS AS(CASE WHEN scope_type='ASSET' THEN scope_target END) STORED;
ALTER TABLE org_role_assignments
 ADD CONSTRAINT org_role_assignments_scope_unit_fkey FOREIGN KEY(org_id,scope_unit_id) REFERENCES org_units(org_id,id),
 ADD CONSTRAINT org_role_assignments_scope_cost_centre_fkey FOREIGN KEY(org_id,scope_cost_centre_id) REFERENCES org_cost_centres(org_id,id),
 ADD CONSTRAINT org_role_assignments_scope_team_fkey FOREIGN KEY(org_id,scope_team_id) REFERENCES org_teams(org_id,id),
 ADD CONSTRAINT org_role_assignments_scope_asset_fkey FOREIGN KEY(org_id,scope_asset_id) REFERENCES org_assets(org_id,id);

-- Fires after org_role_assignment_revision (trigger name order), which already
-- holds the account and organization root locks. Every unit, cost-centre, team
-- and asset state change takes the same root lock first, so a target cannot be
-- archived between this read and the insert. A target hidden by RLS (another
-- organization) or of another kind is not found; a non-live one is refused.
CREATE FUNCTION org_role_assignment_structure_guard() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE target_state TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 -- Shape first (as the check constraint): never the organization or the
 -- assignment's own membership, without reading any structure.
 IF NEW.scope_target=NEW.org_id OR NEW.scope_target=NEW.membership_id THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid structural scope target';
 END IF;
 IF NEW.scope_type='UNIT' THEN
  SELECT state INTO target_state FROM public.org_units WHERE org_id=NEW.org_id AND id=NEW.scope_target;
 ELSIF NEW.scope_type='COST_CENTRE' THEN
  SELECT state INTO target_state FROM public.org_cost_centres WHERE org_id=NEW.org_id AND id=NEW.scope_target;
 ELSIF NEW.scope_type='TEAM' THEN
  SELECT state INTO target_state FROM public.org_teams WHERE org_id=NEW.org_id AND id=NEW.scope_target;
 ELSIF NEW.scope_type='ASSET' THEN
  SELECT state INTO target_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.scope_target;
 ELSE
  RETURN NEW;
 END IF;
 IF target_state IS NULL THEN
  RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_role_assignments_scope_target_fkey',MESSAGE='private reference not found';
 END IF;
 -- BR-ORG-008: archived structure accepts no new assignment. A DRAFT asset is
 -- still discardable (deleted), so it cannot be named either.
 IF NOT(target_state='ACTIVE' OR(NEW.scope_type='ASSET' AND target_state IN('INACTIVE','DISPOSED'))) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='scope target not accepting assignment';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_role_assignment_structure BEFORE INSERT ON org_role_assignments
 FOR EACH ROW WHEN(NEW.scope_type IN('UNIT','COST_CENTRE','TEAM','ASSET')) EXECUTE FUNCTION org_role_assignment_structure_guard();

-- The 206 asset-use source; structural role assignments are rights, not use.
CREATE OR REPLACE FUNCTION org_asset_current_usage(selected_org UUID,selected_asset UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_assets WHERE org_id=selected_org AND id=selected_asset) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_assets'::regclass
 AND NOT((c.conrelid='public.org_asset_tanks'::regclass AND c.conname='org_asset_tanks_asset_fkey')
 OR(c.conrelid='public.org_asset_expected'::regclass AND c.conname='org_asset_expected_asset_fkey')
 OR(c.conrelid='public.org_asset_unit_allocations'::regclass AND c.conname='org_asset_allocation_asset_fkey')
 OR(c.conrelid='public.org_asset_revisions'::regclass AND c.conname='org_asset_revision_asset_fkey')
 OR(c.conrelid='public.org_meters'::regclass AND c.conname='org_meters_asset_fkey')
 OR(c.conrelid='public.org_meter_held_readings'::regclass AND c.conname='org_meter_held_readings_asset_fkey')
 OR(c.conrelid='public.org_asset_unit_periods'::regclass AND c.conname='org_asset_unit_periods_asset_fkey')
 OR(c.conrelid='public.org_asset_cost_centre_periods'::regclass AND c.conname='org_asset_cost_centre_periods_asset_fkey')
 OR(c.conrelid='public.org_role_assignments'::regclass AND c.conname='org_role_assignments_scope_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset));
END $$;

-- The 206 unit-use source with the unit role scope recognised as a right.
CREATE OR REPLACE FUNCTION org_unit_current_usage(selected_org UUID,selected_unit UUID) RETURNS JSONB LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE employee_count BIGINT;asset_count BIGINT;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=selected_org AND id=selected_unit) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_units'::regclass
 AND NOT((c.conrelid='public.org_onboarding_commands'::regclass AND c.conname='org_onboarding_commands_org_id_unit_id_fkey')
 OR(c.conrelid='public.org_unit_commands'::regclass AND c.conname='org_unit_commands_unit_fkey')
 OR(c.conrelid='public.org_employee_profiles'::regclass AND c.conname='org_employee_profile_unit_fkey')
 OR(c.conrelid='public.org_teams'::regclass AND c.conname='org_team_unit_fkey')
 OR(c.conrelid='public.org_assets'::regclass AND c.conname='org_asset_unit_fkey')
 OR(c.conrelid='public.org_asset_unit_allocations'::regclass AND c.conname='org_asset_allocation_unit_fkey')
 OR(c.conrelid='public.org_meter_held_readings'::regclass AND c.conname='org_meter_held_readings_unit_fkey')
 OR(c.conrelid='public.org_asset_unit_periods'::regclass AND c.conname='org_asset_unit_periods_unit_fkey')
 OR(c.conrelid='public.org_role_assignments'::regclass AND c.conname='org_role_assignments_scope_unit_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';END IF;
 SELECT count(DISTINCT employee_id) INTO employee_count FROM(
 SELECT e.id AS employee_id FROM public.org_employee_profiles p JOIN public.org_employee_identities e ON(e.org_id,e.id)=(p.org_id,p.employee_id)
 WHERE p.org_id=selected_org AND p.home_unit_id=selected_unit AND e.state='ACTIVE'
 UNION
 SELECT e.id AS employee_id FROM public.org_team_links l JOIN public.org_teams t ON(t.org_id,t.id)=(l.org_id,l.team_id)
 JOIN public.org_employee_identities e ON(e.org_id,e.id)=(l.org_id,l.employee_id)
 WHERE l.org_id=selected_org AND t.unit_id=selected_unit AND l.ended_at IS NULL AND e.state='ACTIVE') current_employees;
 SELECT count(DISTINCT p.asset_id) INTO asset_count FROM public.org_asset_unit_periods p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
 WHERE p.org_id=selected_org AND p.unit_id=selected_unit AND(p.valid_to IS NULL OR p.valid_to>clock_timestamp()) AND a.state='ACTIVE';
 RETURN jsonb_build_object('assets',asset_count,'employees',employee_count,'budgets',0,'operations',0);
END $$;

-- The 206 cost-centre-use source with the cost-centre role scope recognised.
CREATE OR REPLACE FUNCTION org_cost_centre_current_usage(selected_org UUID,selected_centre UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_cost_centres WHERE org_id=selected_org AND id=selected_centre) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid IN('public.org_cost_centres'::regclass,'public.org_cost_centre_codes'::regclass)
  AND NOT((c.conrelid='public.org_cost_centre_codes'::regclass AND c.conname='org_cost_centre_codes_centre_fkey')
   OR(c.conrelid='public.org_cost_centre_commands'::regclass AND c.conname='org_cost_centre_commands_centre_fkey')
   OR(c.conrelid='public.org_cost_centres'::regclass AND c.conname='org_cost_centre_current_code_fkey')
   OR(c.conrelid='public.org_asset_cost_centre_periods'::regclass AND c.conname='org_asset_cost_centre_periods_centre_fkey')
   OR(c.conrelid='public.org_role_assignments'::regclass AND c.conname='org_role_assignments_scope_cost_centre_fkey'))) THEN
  RAISE EXCEPTION USING ERRCODE='P7804',MESSAGE='cost-centre usage source unavailable';
 END IF;
 RETURN jsonb_build_object('assets',(SELECT count(DISTINCT p.asset_id) FROM public.org_asset_cost_centre_periods p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
  WHERE p.org_id=selected_org AND p.cost_centre_id=selected_centre AND(p.valid_to IS NULL OR p.valid_to>clock_timestamp()) AND a.state='ACTIVE'),'employees',0,'budgets',0,'operations',0);
END $$;

-- The 177 team-use source with the team role scope recognised as a right.
CREATE OR REPLACE FUNCTION org_team_current_usage(selected_org UUID,selected_team UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE employee_count BIGINT;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_teams WHERE org_id=selected_org AND id=selected_team) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_teams'::regclass
  AND NOT((c.conrelid='public.org_team_links'::regclass AND c.conname='org_team_link_team_fkey')
   OR(c.conrelid='public.org_team_commands'::regclass AND c.conname='org_team_command_team_fkey')
   OR(c.conrelid='public.org_role_assignments'::regclass AND c.conname='org_role_assignments_scope_team_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P8004',MESSAGE='team usage source unavailable';END IF;
 SELECT count(*) INTO employee_count FROM public.org_team_links l JOIN public.org_employee_identities e ON(e.org_id,e.id)=(l.org_id,l.employee_id)
 WHERE l.org_id=selected_org AND l.team_id=selected_team AND l.ended_at IS NULL AND e.state='ACTIVE';
 RETURN jsonb_build_object('assets',0,'employees',employee_count,'budgets',0,'operations',0);
END $$;
