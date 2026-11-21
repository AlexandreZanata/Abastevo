-- EMP-AGREE-001..006 (BR-AGREE-001/002, BR-TEN-005, BR-MONEY-001, EDR-023):
-- bilateral station agreements, their private negotiated price terms,
-- bounded expiring fueling authorizations (vouchers) and the station's
-- once-only redemption settlement record. An agreement names two explicit
-- parties and shares only the allowlisted fields. The redemption is the
-- station-side settlement fact; org_fuelings keeps its single signed APP
-- origin (EDR-175), so no redemption ever writes a second fueling cost.
CREATE TABLE org_station_agreements(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 station_id UUID NOT NULL REFERENCES directory_stations(id) ON DELETE RESTRICT,
 org_label TEXT NOT NULL CHECK(length(btrim(org_label)) BETWEEN 1 AND 80),
 state TEXT NOT NULL CHECK(state IN('PROPOSED','ACTIVE','SUSPENDED','ENDED','DECLINED')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 valid_from TIMESTAMPTZ NOT NULL CHECK(isfinite(valid_from)),
 valid_until TIMESTAMPTZ NOT NULL CHECK(isfinite(valid_until) AND valid_until>valid_from),
 conditions TEXT NOT NULL DEFAULT '' CHECK(length(btrim(conditions))<=280),
 proposed_by_membership_id UUID NOT NULL,
 proposed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(proposed_at)),
 decided_by_account_id UUID,
 decided_at TIMESTAMPTZ CHECK(decided_at IS NULL OR isfinite(decided_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 ended_reason TEXT NOT NULL DEFAULT '' CHECK(length(btrim(ended_reason))<=280),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_station_agreements_proposer_fkey FOREIGN KEY(org_id,proposed_by_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_station_agreements_decided_check CHECK(
  (state='PROPOSED' AND decided_by_account_id IS NULL AND decided_at IS NULL)
  OR (state IN('ACTIVE','DECLINED') AND decided_by_account_id IS NOT NULL AND decided_at IS NOT NULL)
  OR (state IN('SUSPENDED','ENDED'))
 )
);
CREATE INDEX org_station_agreements_station_idx ON org_station_agreements(station_id,state);
-- One open agreement (PROPOSED, ACTIVE or SUSPENDED) per organization and
-- station at any time; history keeps every ended or declined one.
CREATE UNIQUE INDEX org_station_agreements_one_open
 ON org_station_agreements(org_id,station_id) WHERE state IN('PROPOSED','ACTIVE','SUSPENDED');

CREATE TABLE org_station_agreement_transitions(
 org_id UUID NOT NULL,
 agreement_id UUID NOT NULL,
 revision BIGINT NOT NULL CHECK(revision>0),
 kind TEXT NOT NULL CHECK(kind IN('PROPOSE','ACCEPT','DECLINE','SUSPEND','END')),
 from_state TEXT NOT NULL CHECK(from_state IN('PROPOSED','ACTIVE','SUSPENDED')),
 to_state TEXT NOT NULL CHECK(to_state IN('PROPOSED','ACTIVE','SUSPENDED','ENDED','DECLINED')),
 actor_kind TEXT NOT NULL CHECK(actor_kind IN('ORG','STATION')),
 actor_membership_id UUID,
 actor_account_id UUID,
 reason TEXT NOT NULL DEFAULT '' CHECK(length(btrim(reason))<=280),
 operation_id UUID NOT NULL,
 at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(at)),
 PRIMARY KEY(org_id,agreement_id,revision),
 CONSTRAINT org_station_agreement_transitions_fkey FOREIGN KEY(org_id,agreement_id) REFERENCES org_station_agreements(org_id,id),
 CONSTRAINT org_station_agreement_actor_check CHECK(
  (actor_kind='ORG' AND actor_membership_id IS NOT NULL AND actor_account_id IS NULL)
  OR (actor_kind='STATION' AND actor_account_id IS NOT NULL AND actor_membership_id IS NULL))
);

-- EMP-AGREE-003: private negotiated price tables, exact product, unit and
-- conditions, visible only within the agreement that recorded them.
CREATE TABLE org_agreement_price_terms(
 org_id UUID NOT NULL,
 agreement_id UUID NOT NULL,
 fuel_product TEXT NOT NULL,
 unit TEXT NOT NULL,
 price_milli_brl BIGINT NOT NULL CHECK(price_milli_brl BETWEEN 1 AND 1000000),
 conditions TEXT NOT NULL DEFAULT '' CHECK(length(btrim(conditions))<=120),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,agreement_id,fuel_product,unit),
 CONSTRAINT org_agreement_price_terms_fkey FOREIGN KEY(org_id,agreement_id) REFERENCES org_station_agreements(org_id,id) ON DELETE CASCADE,
 CONSTRAINT org_agreement_price_terms_product_check CHECK(fuel_product IN
   ('ETHANOL','GASOLINE_REGULAR','GASOLINE_ADDITIVED','DIESEL_S500',
    'DIESEL_S10','CNG','LPG_P13')),
 CONSTRAINT org_agreement_price_terms_unit_check CHECK(unit IN('L','M3','KG_13'))
);

-- EMP-AGREE-004: bounded, expiring, single-use fueling authorizations. The
-- code is returned once at issuance; only its SHA-256 is stored. Failed
-- confirmations are bounded (BR-TEN-005) and lock the voucher.
CREATE TABLE org_fuel_authorizations(
 org_id UUID NOT NULL,
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 agreement_id UUID NOT NULL,
 station_id UUID NOT NULL,
 employee_id UUID NOT NULL,
 asset_id UUID,
 fuel_product TEXT NOT NULL,
 unit TEXT NOT NULL,
 max_quantity_milli BIGINT NOT NULL CHECK(max_quantity_milli BETWEEN 1 AND 1000000000000),
 max_centavos BIGINT NOT NULL CHECK(max_centavos BETWEEN 1 AND 1000000000000),
 valid_until TIMESTAMPTZ NOT NULL CHECK(isfinite(valid_until)),
 code_hash TEXT NOT NULL CHECK(code_hash~'^[a-f0-9]{64}$'),
 state TEXT NOT NULL DEFAULT 'ISSUED' CHECK(state IN('ISSUED','REDEEMED','CANCELLED','LOCKED')),
 failed_attempts INTEGER NOT NULL DEFAULT 0 CHECK(failed_attempts BETWEEN 0 AND 5),
 issued_by_membership_id UUID NOT NULL,
 issued_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(issued_at)),
 cancelled_reason TEXT NOT NULL DEFAULT '' CHECK(length(btrim(cancelled_reason))<=280),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,code_hash),
 CONSTRAINT org_fuel_authorizations_agreement_fkey FOREIGN KEY(org_id,agreement_id) REFERENCES org_station_agreements(org_id,id),
 CONSTRAINT org_fuel_authorizations_employee_fkey FOREIGN KEY(org_id,employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_fuel_authorizations_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_fuel_authorizations_issuer_fkey FOREIGN KEY(org_id,issued_by_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_fuel_authorizations_product_check CHECK(fuel_product IN
   ('ETHANOL','GASOLINE_REGULAR','GASOLINE_ADDITIVED','DIESEL_S500',
    'DIESEL_S10','CNG','LPG_P13')),
 CONSTRAINT org_fuel_authorizations_unit_check CHECK(unit IN('L','M3','KG_13'))
);
CREATE INDEX org_fuel_authorizations_station_idx ON org_fuel_authorizations(org_id,station_id,state);
CREATE INDEX org_fuel_authorizations_employee_idx ON org_fuel_authorizations(org_id,employee_id);

CREATE TABLE org_fuel_authorization_events(
 org_id UUID NOT NULL,
 authorization_id UUID NOT NULL,
 seq BIGINT NOT NULL CHECK(seq>0),
 kind TEXT NOT NULL CHECK(kind IN('ISSUE','CANCEL','REDEEM','FAILED_ATTEMPT','LOCK')),
 actor_kind TEXT NOT NULL CHECK(actor_kind IN('ORG','STATION')),
 actor_membership_id UUID,
 actor_account_id UUID,
 detail TEXT NOT NULL DEFAULT '' CHECK(length(btrim(detail))<=280),
 at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(at)),
 PRIMARY KEY(org_id,authorization_id,seq),
 CONSTRAINT org_fuel_authorization_events_fkey FOREIGN KEY(org_id,authorization_id) REFERENCES org_fuel_authorizations(org_id,id),
 CONSTRAINT org_fuel_authorization_events_actor_check CHECK(
  (actor_kind='ORG' AND actor_membership_id IS NOT NULL AND actor_account_id IS NULL)
  OR (actor_kind='STATION' AND actor_account_id IS NOT NULL AND actor_membership_id IS NULL))
);

-- EMP-AGREE-005: the station's once-only redemption confirmation. The
-- server derives the total from the agreement's term; matching retries
-- return the stored receipt and competing use cannot consume it twice.
CREATE TABLE org_fuel_redemptions(
 org_id UUID NOT NULL,
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 authorization_id UUID NOT NULL,
 agreement_id UUID NOT NULL,
 station_id UUID NOT NULL,
 employee_id UUID NOT NULL,
 asset_id UUID,
 fuel_product TEXT NOT NULL,
 unit TEXT NOT NULL,
 quantity_milli BIGINT NOT NULL CHECK(quantity_milli BETWEEN 1 AND 1000000000),
 price_milli_brl BIGINT NOT NULL CHECK(price_milli_brl BETWEEN 1 AND 1000000),
 total_centavos BIGINT NOT NULL CHECK(total_centavos BETWEEN 1 AND 1000000000000),
 operation_id UUID NOT NULL,
 confirmed_by_account_id UUID NOT NULL,
 confirmed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(confirmed_at)),
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,authorization_id),
 CONSTRAINT org_fuel_redemptions_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_fuel_redemptions_authorization_fkey FOREIGN KEY(org_id,authorization_id) REFERENCES org_fuel_authorizations(org_id,id),
 CONSTRAINT org_fuel_redemptions_agreement_fkey FOREIGN KEY(org_id,agreement_id) REFERENCES org_station_agreements(org_id,id),
 CONSTRAINT org_fuel_redemptions_total_check CHECK(total_centavos=(quantity_milli*price_milli_brl+500)/1000)
);
CREATE INDEX org_fuel_redemptions_agreement_idx ON org_fuel_redemptions(org_id,agreement_id,confirmed_at DESC);
CREATE FUNCTION org_fuel_redemptions_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='redemption history is insert-only';
END $$;
CREATE TRIGGER org_fuel_redemptions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_fuel_redemptions FOR EACH STATEMENT EXECUTE FUNCTION org_fuel_redemptions_keep_history();

-- Idempotent command receipts (the F6 org_f6_commands pattern).
CREATE TABLE org_f7_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 scope_kind TEXT NOT NULL CHECK(length(btrim(scope_kind)) BETWEEN 1 AND 64),
 operation_id UUID NOT NULL,
 entity_id UUID NOT NULL,
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 result_json JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,scope_kind,operation_id)
);

-- Scope guard: organization sessions carry anpfuel.org_id; the station-side
-- agreement list carries anpfuel.station_id instead and reads only the
-- station-visible rows. Station confirmations set both after resolving the
-- organization from the agreement or voucher, so private rows never leave
-- their organization scope.
CREATE FUNCTION org_f7_scope_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid
  AND NEW.station_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.station_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 RETURN NEW;
END $$;
-- The audit trails, negotiated terms, redemption facts and command receipts
-- are organization-only: station confirmations resolve the organization
-- first and keep these rows inside their scope.
CREATE FUNCTION org_f7_org_scope_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_station_agreements_scope BEFORE INSERT OR UPDATE ON org_station_agreements FOR EACH ROW EXECUTE FUNCTION org_f7_scope_guard();
CREATE TRIGGER org_station_agreement_transitions_scope BEFORE INSERT ON org_station_agreement_transitions FOR EACH ROW EXECUTE FUNCTION org_f7_org_scope_guard();
CREATE TRIGGER org_agreement_price_terms_scope BEFORE INSERT ON org_agreement_price_terms FOR EACH ROW EXECUTE FUNCTION org_f7_org_scope_guard();
CREATE TRIGGER org_fuel_authorizations_scope BEFORE INSERT OR UPDATE ON org_fuel_authorizations FOR EACH ROW EXECUTE FUNCTION org_f7_scope_guard();
CREATE TRIGGER org_fuel_authorization_events_scope BEFORE INSERT ON org_fuel_authorization_events FOR EACH ROW EXECUTE FUNCTION org_f7_org_scope_guard();
CREATE TRIGGER org_fuel_redemptions_scope BEFORE INSERT ON org_fuel_redemptions FOR EACH ROW EXECUTE FUNCTION org_f7_org_scope_guard();
CREATE TRIGGER org_f7_commands_scope_guard BEFORE INSERT ON org_f7_commands FOR EACH ROW EXECUTE FUNCTION org_f7_org_scope_guard();

-- Agreement state machine and revision discipline.
CREATE FUNCTION org_station_agreements_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='agreement revision conflict'; END IF;
  IF NEW.org_id IS DISTINCT FROM OLD.org_id OR NEW.id IS DISTINCT FROM OLD.id
   OR NEW.station_id IS DISTINCT FROM OLD.station_id OR NEW.org_label IS DISTINCT FROM OLD.org_label
   OR NEW.valid_from IS DISTINCT FROM OLD.valid_from OR NEW.valid_until IS DISTINCT FROM OLD.valid_until
   OR NEW.conditions IS DISTINCT FROM OLD.conditions
   OR NEW.proposed_by_membership_id IS DISTINCT FROM OLD.proposed_by_membership_id
   OR NEW.proposed_at IS DISTINCT FROM OLD.proposed_at THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='agreement proposal fields are immutable';
  END IF;
  IF NOT (
   (OLD.state='PROPOSED' AND NEW.state IN('ACTIVE','DECLINED','ENDED'))
   OR (OLD.state='ACTIVE' AND NEW.state IN('SUSPENDED','ENDED'))
   OR (OLD.state='SUSPENDED' AND NEW.state='ENDED')) THEN
   RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='agreement transition refused';
  END IF;
  IF OLD.state='PROPOSED' AND NEW.state='ENDED' AND NEW.decided_by_account_id IS NOT NULL THEN
   RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='withdrawn proposal keeps no station decision';
  END IF;
  NEW.updated_at=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_station_agreements_guard BEFORE UPDATE ON org_station_agreements FOR EACH ROW EXECUTE FUNCTION org_station_agreements_guard();
CREATE FUNCTION org_station_agreements_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='agreement history is insert-only';
END $$;
CREATE TRIGGER org_station_agreements_immutable BEFORE DELETE OR TRUNCATE ON org_station_agreements FOR EACH STATEMENT EXECUTE FUNCTION org_station_agreements_keep_history();
CREATE FUNCTION org_station_agreement_transitions_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='agreement transitions are insert-only';
END $$;
CREATE TRIGGER org_station_agreement_transitions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_station_agreement_transitions FOR EACH STATEMENT EXECUTE FUNCTION org_station_agreement_transitions_keep_history();

-- Price terms follow the agreement that recorded them and never change.
CREATE FUNCTION org_agreement_price_terms_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='agreement price terms are insert-only';
END $$;
CREATE TRIGGER org_agreement_price_terms_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_agreement_price_terms FOR EACH STATEMENT EXECUTE FUNCTION org_agreement_price_terms_keep_history();

-- Authorization state machine: ISSUED leaves only by cancellation, lock or
-- the single redemption; every other stored field is immutable.
CREATE FUNCTION org_fuel_authorizations_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.org_id IS DISTINCT FROM OLD.org_id OR NEW.id IS DISTINCT FROM OLD.id
   OR NEW.agreement_id IS DISTINCT FROM OLD.agreement_id OR NEW.station_id IS DISTINCT FROM OLD.station_id
   OR NEW.employee_id IS DISTINCT FROM OLD.employee_id OR NEW.asset_id IS DISTINCT FROM OLD.asset_id
   OR NEW.fuel_product IS DISTINCT FROM OLD.fuel_product OR NEW.unit IS DISTINCT FROM OLD.unit
   OR NEW.max_quantity_milli IS DISTINCT FROM OLD.max_quantity_milli OR NEW.max_centavos IS DISTINCT FROM OLD.max_centavos
   OR NEW.valid_until IS DISTINCT FROM OLD.valid_until OR NEW.code_hash IS DISTINCT FROM OLD.code_hash
   OR NEW.issued_by_membership_id IS DISTINCT FROM OLD.issued_by_membership_id
   OR NEW.issued_at IS DISTINCT FROM OLD.issued_at
   OR NEW.failed_attempts<OLD.failed_attempts THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='authorization issue fields are immutable';
  END IF;
  IF NOT (
   (OLD.state='ISSUED' AND NEW.state IN('REDEEMED','CANCELLED') AND NEW.failed_attempts=OLD.failed_attempts)
   OR (OLD.state='ISSUED' AND NEW.state='LOCKED' AND NEW.failed_attempts=OLD.failed_attempts+1)
   OR (OLD.state='ISSUED' AND NEW.state='ISSUED' AND NEW.failed_attempts=OLD.failed_attempts+1)) THEN
   RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='authorization transition refused';
  END IF;
  IF NEW.failed_attempts>5 THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='authorization attempts exhausted'; END IF;
  IF NEW.state='LOCKED' AND NEW.failed_attempts<>5 THEN
   RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='locked authorization needs the bounded attempts';
  END IF;
  NEW.updated_at=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_fuel_authorizations_guard BEFORE UPDATE ON org_fuel_authorizations FOR EACH ROW EXECUTE FUNCTION org_fuel_authorizations_guard();
CREATE FUNCTION org_fuel_authorizations_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='authorization history is insert-only';
END $$;
CREATE TRIGGER org_fuel_authorizations_immutable BEFORE DELETE OR TRUNCATE ON org_fuel_authorizations FOR EACH STATEMENT EXECUTE FUNCTION org_fuel_authorizations_keep_history();
CREATE FUNCTION org_fuel_authorization_events_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='authorization events are insert-only';
END $$;
CREATE TRIGGER org_fuel_authorization_events_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_fuel_authorization_events FOR EACH STATEMENT EXECUTE FUNCTION org_fuel_authorization_events_keep_history();

DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY[
  'org_station_agreements','org_station_agreement_transitions','org_agreement_price_terms',
  'org_fuel_authorizations','org_fuel_authorization_events','org_fuel_redemptions','org_f7_commands'
 ] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
  EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 END LOOP;
END $$;
-- The station side reads its own agreements across organizations under
-- anpfuel.station_id (allowlisted columns only; the permissive policy ORs
-- with the organization scope). Station confirmations also set the
-- organization scope once the target is resolved.
CREATE POLICY org_station_agreements_station_side ON org_station_agreements
 USING(station_id=NULLIF(current_setting('anpfuel.station_id',true),'')::uuid)
 WITH CHECK(station_id=NULLIF(current_setting('anpfuel.station_id',true),'')::uuid);
-- The once-only redemption resolves the voucher by its code digest under
-- the station scope before the organization is known (EMP-AGREE-005).
CREATE POLICY org_fuel_authorizations_station_side ON org_fuel_authorizations
 USING(station_id=NULLIF(current_setting('anpfuel.station_id',true),'')::uuid)
 WITH CHECK(station_id=NULLIF(current_setting('anpfuel.station_id',true),'')::uuid);


-- EMP-AGREE-004: the bounded fueling authorization's optional asset reference
-- joins the 244 asset-use allowlist (the house rule: every new org_assets foreign
-- key re-proves the FLEET-005 use source, and an authorization never blocks an
-- asset discard or fixes its kind — it is a right, not a usage).
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
 OR(c.conrelid='public.org_role_assignments'::regclass AND c.conname='org_role_assignments_scope_asset_fkey')
 OR(c.conrelid=to_regclass('public.org_document_links') AND c.conname='org_document_links_asset_fkey')
 OR(c.conrelid='public.org_asset_assignments'::regclass AND c.conname='org_asset_assignments_asset_fkey')
 OR(c.conrelid='public.org_fuelings'::regclass AND c.conname='org_fuelings_asset_fkey')
 OR(c.conrelid='public.org_asset_reservations'::regclass AND c.conname='org_asset_reservations_asset_fkey')
 OR(c.conrelid='public.org_asset_blocks'::regclass AND c.conname='org_asset_blocks_asset_fkey')
 OR(c.conrelid='public.org_trips'::regclass AND c.conname='org_trips_asset_fkey')
 OR(c.conrelid='public.org_trip_held_starts'::regclass AND c.conname='org_trip_held_starts_asset_fkey')
 OR(c.conrelid='public.org_asset_implement_attachments'::regclass AND c.conname='org_asset_implement_attachments_implement_fkey')
 OR(c.conrelid='public.org_asset_implement_attachments'::regclass AND c.conname='org_asset_implement_attachments_powered_fkey')
 OR(c.conrelid='public.org_checklist_template_targets'::regclass AND c.conname='org_checklist_template_targets_asset_fkey')
 OR(c.conrelid='public.org_trip_missions'::regclass AND c.conname='org_trip_missions_asset_fkey')
 OR(c.conrelid='public.org_incident_reports'::regclass AND c.conname='org_incident_reports_asset_fkey')
 OR(c.conrelid='public.org_trip_mission_vehicles'::regclass AND c.conname='org_trip_mission_vehicles_previous_fkey')
 OR(c.conrelid='public.org_trip_mission_vehicles'::regclass AND c.conname='org_trip_mission_vehicles_asset_fkey')
 OR(c.conrelid='public.org_expenses'::regclass AND c.conname='org_expenses_asset_fkey')
 OR(c.conrelid='public.org_checklist_runs'::regclass AND c.conname='org_checklist_runs_asset_fkey')
 OR(c.conrelid='public.org_fuel_authorizations'::regclass AND c.conname='org_fuel_authorizations_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)+(SELECT count(*) FROM public.org_fuelings f WHERE f.org_id=selected_org AND f.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trips t WHERE t.org_id=selected_org AND t.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trip_held_starts h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)
 +(SELECT count(*) FROM public.org_asset_implement_attachments i WHERE i.org_id=selected_org AND(i.implement_id=selected_asset OR i.powered_asset_id=selected_asset)));
END $$;
