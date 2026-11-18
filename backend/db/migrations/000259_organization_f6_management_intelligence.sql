-- F6 Management, Maintenance, Internal Fuel Stock, Intelligence, Findings,
-- Reporting, Retention/Privacy, and Group Consolidation
-- (EMP-STOCK-001..007, EMP-FLEET-010, EMP-MAINT-001..012, EMP-KPI-001..006,
-- EMP-FIND-001..005, EMP-RPT-003..006, EMP-FUEL-009..010, EMP-EXP-007,
-- EMP-FIN-005..006, EMP-DOC-012, EMP-PRIV-001/002/004, EMP-ORG-012,
-- EMP-IDN-019, EMP-AUTH-010, EMP-PUB-002).
-- Avoids SQL FOREIGN KEY constraints referencing org_units, org_cost_centres,
-- org_assets, or org_teams so structural pg_constraint audits stay intact;
-- foreign references to those tables are enforced inside BEFORE triggers.

CREATE FUNCTION org_f6_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization f6 history is append-only';
END $$;

CREATE FUNCTION org_f6_scope_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN
  RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';
 END IF;
 RETURN NEW;
END $$;

-- Unified F6 idempotency receipt table for panel HTTP commands.
CREATE TABLE org_f6_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 scope_kind TEXT NOT NULL CHECK(length(btrim(scope_kind)) BETWEEN 1 AND 64),
 operation_id UUID NOT NULL,
 entity_id UUID NOT NULL,
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 result_json JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,scope_kind,operation_id)
);
CREATE TRIGGER org_f6_commands_scope_guard BEFORE INSERT ON org_f6_commands FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();
CREATE TRIGGER org_f6_commands_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_f6_commands FOR EACH STATEMENT EXECUTE FUNCTION org_f6_keep_history();

-- EMP-STOCK-001..005: Internal fuel tanks, append-only movements, internal
-- fuelings and single-count operational expenses.
CREATE TABLE org_fuel_tanks(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 unit_id UUID,
 name TEXT NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 160),
 product TEXT NOT NULL CHECK(product IN('GASOLINE_REGULAR','GASOLINE_ADDITIVED','GASOLINE_PREMIUM_GRADE','ETHANOL','DIESEL_S500','DIESEL_S10','CNG','ELECTRIC_DC','ELECTRIC_AC')),
 unit TEXT NOT NULL CHECK(unit IN('L','M3','KWH')),
 capacity_milli BIGINT NOT NULL CHECK(capacity_milli BETWEEN 1 AND 1000000000),
 quantity_milli BIGINT NOT NULL DEFAULT 0 CHECK(quantity_milli BETWEEN -1000000000 AND 1000000000),
 value_centavos BIGINT NOT NULL DEFAULT 0 CHECK(value_centavos BETWEEN 0 AND 1000000000000),
 allow_negative BOOLEAN NOT NULL DEFAULT false,
 state TEXT NOT NULL DEFAULT 'ACTIVE' CHECK(state IN('ACTIVE','ARCHIVED')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id)
);
CREATE UNIQUE INDEX org_fuel_tanks_active_name ON org_fuel_tanks(org_id,lower(btrim(name))) WHERE state='ACTIVE';

CREATE FUNCTION org_fuel_tanks_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF NEW.unit_id IS NOT NULL THEN
  PERFORM 1 FROM public.org_units u WHERE u.org_id=NEW.org_id AND u.id=NEW.unit_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 END IF;
 IF NOT NEW.allow_negative AND NEW.quantity_milli<0 THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='negative stock not allowed';
 END IF;
 IF TG_OP='INSERT' THEN
  NEW.revision=1; NEW.created_at=clock_timestamp(); NEW.updated_at=NEW.created_at;
 ELSE
  IF NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fuel tank revision conflict'; END IF;
  IF NEW.product<>OLD.product OR NEW.unit<>OLD.unit THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fuel tank product immutable'; END IF;
  NEW.created_at=OLD.created_at; NEW.updated_at=clock_timestamp();
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_fuel_tanks_guard BEFORE INSERT OR UPDATE ON org_fuel_tanks FOR EACH ROW EXECUTE FUNCTION org_fuel_tanks_guard();

CREATE TABLE org_fuel_tank_movements(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 tank_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('RECEIPT','DISPENSE','TRANSFER_OUT','TRANSFER_IN','ADJUSTMENT')),
 product TEXT NOT NULL,
 unit TEXT NOT NULL,
 delta_quantity_milli BIGINT NOT NULL CHECK(delta_quantity_milli<>0),
 amount_centavos BIGINT NOT NULL CHECK(amount_centavos>=0),
 pool_quantity_after_milli BIGINT NOT NULL,
 pool_value_after_centavos BIGINT NOT NULL CHECK(pool_value_after_centavos>=0),
 asset_id UUID,
 counterpart_tank_id UUID,
 supplier_id UUID,
 document_id UUID,
 fueling_id UUID,
 expense_id UUID,
 reason TEXT NOT NULL DEFAULT '' CHECK(length(reason)<=500),
 author_membership_id UUID NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL CHECK(isfinite(occurred_at)),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(recorded_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_fuel_tank_movements_op UNIQUE(org_id,operation_id,kind),
 CONSTRAINT org_fuel_tank_movements_tank_fkey FOREIGN KEY(org_id,tank_id) REFERENCES org_fuel_tanks(org_id,id),
 CONSTRAINT org_fuel_tank_movements_counterpart_fkey FOREIGN KEY(org_id,counterpart_tank_id) REFERENCES org_fuel_tanks(org_id,id),
 CONSTRAINT org_fuel_tank_movements_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_fuel_tank_movements_adjust_reason CHECK(kind<>'ADJUSTMENT' OR length(btrim(reason))>=1)
);
CREATE UNIQUE INDEX org_fuel_tank_movements_dispense_once ON org_fuel_tank_movements(org_id,fueling_id) WHERE kind='DISPENSE' AND fueling_id IS NOT NULL;
CREATE INDEX org_fuel_tank_movements_tank ON org_fuel_tank_movements(org_id,tank_id,recorded_at DESC);

CREATE FUNCTION org_fuel_tank_movements_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_fuel_tanks%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 SELECT * INTO t FROM public.org_fuel_tanks WHERE org_id=NEW.org_id AND id=NEW.tank_id;
 IF NOT FOUND OR t.product<>NEW.product OR t.unit<>NEW.unit THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='tank product mismatch';
 END IF;
 IF NEW.asset_id IS NOT NULL THEN
  PERFORM 1 FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.id=NEW.asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 END IF;
 IF NEW.kind='RECEIPT' AND NEW.expense_id IS NOT NULL THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='stock purchase cannot recognize expense directly';
 END IF;
 NEW.recorded_at=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER org_fuel_tank_movements_guard BEFORE INSERT ON org_fuel_tank_movements FOR EACH ROW EXECUTE FUNCTION org_fuel_tank_movements_guard();
CREATE TRIGGER org_fuel_tank_movements_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_fuel_tank_movements FOR EACH STATEMENT EXECUTE FUNCTION org_f6_keep_history();

CREATE TABLE org_fuel_tank_reconciliations(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 tank_id UUID NOT NULL,
 measured_milli BIGINT NOT NULL CHECK(measured_milli>=0),
 recorded_milli BIGINT NOT NULL,
 variance_milli BIGINT NOT NULL,
 state TEXT NOT NULL CHECK(state IN('OPEN','EXPLAINED','ADJUSTED')),
 adjustment_movement_id UUID,
 reason TEXT NOT NULL DEFAULT '' CHECK(length(reason)<=500),
 author_membership_id UUID NOT NULL,
 measured_at TIMESTAMPTZ NOT NULL CHECK(isfinite(measured_at)),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(recorded_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_fuel_tank_rec_op UNIQUE(org_id,operation_id),
 CONSTRAINT org_fuel_tank_rec_tank_fkey FOREIGN KEY(org_id,tank_id) REFERENCES org_fuel_tanks(org_id,id),
 CONSTRAINT org_fuel_tank_rec_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id)
);
CREATE TRIGGER org_fuel_tank_rec_scope BEFORE INSERT OR UPDATE ON org_fuel_tank_reconciliations FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

CREATE TABLE org_internal_fuelings(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 tank_id UUID NOT NULL,
 movement_id UUID NOT NULL,
 asset_id UUID NOT NULL,
 unit_id UUID,
 cost_centre_id UUID,
 product TEXT NOT NULL,
 unit TEXT NOT NULL,
 quantity_milli BIGINT NOT NULL CHECK(quantity_milli>0),
 cost_centavos BIGINT NOT NULL CHECK(cost_centavos>=0),
 fill TEXT NOT NULL CHECK(fill IN('FULL','PARTIAL','UNKNOWN')),
 meter_reading_value BIGINT,
 occurred_at TIMESTAMPTZ NOT NULL CHECK(isfinite(occurred_at)),
 reporting_day DATE NOT NULL,
 author_membership_id UUID NOT NULL,
 review_state TEXT NOT NULL DEFAULT 'APPROVED' CHECK(review_state IN('SUBMITTED','APPROVED','REJECTED')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_internal_fuelings_op UNIQUE(org_id,operation_id),
 CONSTRAINT org_internal_fuelings_tank_fkey FOREIGN KEY(org_id,tank_id) REFERENCES org_fuel_tanks(org_id,id),
 CONSTRAINT org_internal_fuelings_mov_fkey FOREIGN KEY(org_id,movement_id) REFERENCES org_fuel_tank_movements(org_id,id),
 CONSTRAINT org_internal_fuelings_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id)
);

CREATE FUNCTION org_internal_fuelings_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 PERFORM 1 FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.id=NEW.asset_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF NEW.unit_id IS NOT NULL THEN
  PERFORM 1 FROM public.org_units u WHERE u.org_id=NEW.org_id AND u.id=NEW.unit_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 END IF;
 IF NEW.cost_centre_id IS NOT NULL THEN
  PERFORM 1 FROM public.org_cost_centres c WHERE c.org_id=NEW.org_id AND c.id=NEW.cost_centre_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_internal_fuelings_guard BEFORE INSERT OR UPDATE ON org_internal_fuelings FOR EACH ROW EXECUTE FUNCTION org_internal_fuelings_guard();

-- F6 Single-count operational expense ledger (work order close, internal stock
-- dispense, fines & recurring costs). Never permits stock receipts to post.
CREATE TABLE org_operating_cost_entries(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 origin_kind TEXT NOT NULL CHECK(origin_kind IN('WORK_ORDER','INTERNAL_STOCK_DISPENSE','FINE_OR_RECURRING')),
 source_id UUID NOT NULL,
 work_order_id UUID,
 movement_id UUID,
 recurring_cost_id UUID,
 asset_id UUID,
 unit_id UUID,
 cost_centre_id UUID,
 supplier_id UUID,
 category TEXT NOT NULL CHECK(category IN('FUEL','MAINTENANCE','FINE','LEASE','INSURANCE','TOLL','PARKING','OTHER')),
 operating_cost BOOLEAN NOT NULL DEFAULT true,
 paid_centavos BIGINT NOT NULL CHECK(paid_centavos BETWEEN 0 AND 1000000000000),
 company_centavos BIGINT NOT NULL CHECK(company_centavos BETWEEN 0 AND 1000000000000 AND company_centavos<=paid_centavos),
 reporting_day DATE NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL CHECK(isfinite(occurred_at)),
 review_state TEXT NOT NULL DEFAULT 'APPROVED' CHECK(review_state IN('SUBMITTED','APPROVED','VOIDED')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_operating_cost_origin_unique UNIQUE(org_id,origin_kind,source_id),
 CONSTRAINT org_operating_cost_wo_unique UNIQUE(org_id,work_order_id),
 CONSTRAINT org_operating_cost_mov_unique UNIQUE(org_id,movement_id),
 CONSTRAINT org_operating_cost_rec_unique UNIQUE(org_id,recurring_cost_id)
);
CREATE INDEX org_operating_cost_reporting ON org_operating_cost_entries(org_id,reporting_day,asset_id);
CREATE TRIGGER org_operating_cost_scope BEFORE INSERT OR UPDATE ON org_operating_cost_entries FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

-- EMP-FLEET-010: Asset documents with kind and declared expiry day.
CREATE TABLE org_asset_document_links(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 asset_id UUID NOT NULL,
 document_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(length(btrim(kind)) BETWEEN 1 AND 64),
 expiry_day DATE,
 is_cover_photo BOOLEAN NOT NULL DEFAULT false,
 state TEXT NOT NULL DEFAULT 'ACTIVE' CHECK(state IN('ACTIVE','REPLACED','DETACHED')),
 replaced_by_id UUID,
 author_membership_id UUID NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_asset_doc_links_doc_fkey FOREIGN KEY(org_id,document_id) REFERENCES org_documents(org_id,id),
 CONSTRAINT org_asset_doc_links_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id)
);
CREATE INDEX org_asset_doc_links_asset ON org_asset_document_links(org_id,asset_id,state,expiry_day);

CREATE FUNCTION org_asset_doc_links_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 PERFORM 1 FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.id=NEW.asset_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 NEW.updated_at=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER org_asset_doc_links_guard BEFORE INSERT OR UPDATE ON org_asset_document_links FOR EACH ROW EXECUTE FUNCTION org_asset_doc_links_guard();

-- EMP-MAINT-001..006: Preventive plans, occurrences, work orders & holds.
CREATE TABLE org_maintenance_plans(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 asset_id UUID NOT NULL,
 unit_id UUID,
 cost_centre_id UUID,
 name TEXT NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 160),
 tasks_json JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(tasks_json)='array'),
 distance_interval_m BIGINT CHECK(distance_interval_m IS NULL OR distance_interval_m BETWEEN 1 AND 1000000000000),
 distance_lead_m BIGINT CHECK(distance_lead_m IS NULL OR distance_lead_m BETWEEN 1 AND 1000000000000),
 hour_interval_s BIGINT CHECK(hour_interval_s IS NULL OR hour_interval_s BETWEEN 1 AND 1000000000),
 hour_lead_s BIGINT CHECK(hour_lead_s IS NULL OR hour_lead_s BETWEEN 1 AND 1000000000),
 time_interval_days INTEGER CHECK(time_interval_days IS NULL OR time_interval_days BETWEEN 1 AND 3650),
 time_interval_months INTEGER CHECK(time_interval_months IS NULL OR time_interval_months BETWEEN 1 AND 120),
 time_lead_days INTEGER CHECK(time_lead_days IS NULL OR time_lead_days>=1),
 baseline_day DATE NOT NULL,
 baseline_odometer_m BIGINT,
 baseline_hour_s BIGINT,
 rebase_rule TEXT NOT NULL DEFAULT 'ACTUAL' CHECK(rebase_rule IN('ACTUAL','SCHEDULED')),
 blocking BOOLEAN NOT NULL DEFAULT false,
 state TEXT NOT NULL DEFAULT 'ACTIVE' CHECK(state IN('ACTIVE','ARCHIVED')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_maint_plans_dist_pair CHECK((distance_interval_m IS NULL)=(distance_lead_m IS NULL) AND(distance_lead_m IS NULL OR distance_lead_m<=distance_interval_m)),
 CONSTRAINT org_maint_plans_hour_pair CHECK((hour_interval_s IS NULL)=(hour_lead_s IS NULL) AND(hour_lead_s IS NULL OR hour_lead_s<=hour_interval_s)),
 CONSTRAINT org_maint_plans_at_least_one_trigger CHECK(distance_interval_m IS NOT NULL OR hour_interval_s IS NOT NULL OR time_interval_days IS NOT NULL OR time_interval_months IS NOT NULL)
);

CREATE FUNCTION org_maintenance_plans_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 PERFORM 1 FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.id=NEW.asset_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF NEW.unit_id IS NOT NULL THEN
  PERFORM 1 FROM public.org_units u WHERE u.org_id=NEW.org_id AND u.id=NEW.unit_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 END IF;
 IF NEW.cost_centre_id IS NOT NULL THEN
  PERFORM 1 FROM public.org_cost_centres c WHERE c.org_id=NEW.org_id AND c.id=NEW.cost_centre_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 END IF;
 NEW.updated_at=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER org_maintenance_plans_guard BEFORE INSERT OR UPDATE ON org_maintenance_plans FOR EACH ROW EXECUTE FUNCTION org_maintenance_plans_guard();

CREATE TABLE org_maintenance_occurrences(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 plan_id UUID NOT NULL,
 asset_id UUID NOT NULL,
 sequence INTEGER NOT NULL CHECK(sequence>=1),
 state TEXT NOT NULL CHECK(state IN('UPCOMING','DUE','OVERDUE','DONE','SKIPPED')),
 due_odometer_m BIGINT,
 due_hour_s BIGINT,
 due_day DATE,
 used_m BIGINT,
 remaining_m BIGINT,
 used_s BIGINT,
 remaining_s BIGINT,
 days_to_due INTEGER,
 projected_due_day DATE,
 deciding_trigger TEXT NOT NULL DEFAULT 'NONE' CHECK(deciding_trigger IN('NONE','DISTANCE','HOURS','TIME')),
 reading_revision BIGINT NOT NULL DEFAULT 0,
 completed_day DATE,
 completed_odometer_m BIGINT,
 completed_hour_s BIGINT,
 skip_reason TEXT NOT NULL DEFAULT '' CHECK(length(skip_reason)<=500),
 work_order_id UUID,
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_maint_occ_plan_fkey FOREIGN KEY(org_id,plan_id) REFERENCES org_maintenance_plans(org_id,id)
);
-- Only one open occurrence (UPCOMING, DUE, OVERDUE) per plan at any time.
CREATE UNIQUE INDEX org_maint_occ_one_open ON org_maintenance_occurrences(org_id,plan_id) WHERE state IN('UPCOMING','DUE','OVERDUE');
CREATE TRIGGER org_maint_occ_scope BEFORE INSERT OR UPDATE ON org_maintenance_occurrences FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

CREATE TABLE org_work_orders(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 number BIGINT GENERATED BY DEFAULT AS IDENTITY,
 operation_id UUID NOT NULL,
 asset_id UUID NOT NULL,
 unit_id UUID,
 cost_centre_id UUID,
 kind TEXT NOT NULL CHECK(kind IN('PREVENTIVE','CORRECTIVE','INSPECTION')),
 origin_kind TEXT NOT NULL DEFAULT 'MANUAL' CHECK(origin_kind IN('MANUAL','PREVENTIVE_OCCURRENCE','INCIDENT_REPORT','CHECKLIST_RUN','DRIVER_REQUEST')),
 origin_id UUID,
 supplier_id UUID,
 description TEXT NOT NULL CHECK(length(btrim(description)) BETWEEN 1 AND 1000),
 state TEXT NOT NULL CHECK(state IN('DRAFT','REQUESTED','QUOTED','APPROVED','SCHEDULED','IN_EXECUTION','COMPLETED','CLOSED','REJECTED','CANCELLED')),
 requester_membership_id UUID NOT NULL,
 reviewer_membership_id UUID,
 selected_quote_cents BIGINT CHECK(selected_quote_cents IS NULL OR selected_quote_cents>=0),
 quote_valid_until DATE,
 invoice_required BOOLEAN NOT NULL DEFAULT false,
 invoice_document_id UUID,
 approved_cents BIGINT CHECK(approved_cents IS NULL OR approved_cents>=0),
 tolerance_percent INTEGER NOT NULL DEFAULT 10 CHECK(tolerance_percent BETWEEN 0 AND 100),
 lines_json JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(lines_json)='array'),
 parts_cents BIGINT NOT NULL DEFAULT 0 CHECK(parts_cents>=0),
 service_cents BIGINT NOT NULL DEFAULT 0 CHECK(service_cents>=0),
 discount_cents BIGINT NOT NULL DEFAULT 0 CHECK(discount_cents>=0),
 paid_cents BIGINT CHECK(paid_cents IS NULL OR paid_cents>=0),
 arithmetic_mismatch BOOLEAN NOT NULL DEFAULT false,
 expense_id UUID,
 hold_open BOOLEAN NOT NULL DEFAULT false,
 hold_started_at TIMESTAMPTZ CHECK(hold_started_at IS NULL OR isfinite(hold_started_at)),
 hold_ended_at TIMESTAMPTZ CHECK(hold_ended_at IS NULL OR isfinite(hold_ended_at)),
 scheduled_at TIMESTAMPTZ CHECK(scheduled_at IS NULL OR isfinite(scheduled_at)),
 started_at TIMESTAMPTZ CHECK(started_at IS NULL OR isfinite(started_at)),
 completed_at TIMESTAMPTZ CHECK(completed_at IS NULL OR isfinite(completed_at)),
 closed_at TIMESTAMPTZ CHECK(closed_at IS NULL OR isfinite(closed_at)),
 reason TEXT NOT NULL DEFAULT '' CHECK(length(reason)<=500),
 after_the_fact BOOLEAN NOT NULL DEFAULT false,
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_work_orders_requester_fkey FOREIGN KEY(org_id,requester_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_work_orders_reviewer_fkey FOREIGN KEY(org_id,reviewer_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_work_orders_self_review CHECK(reviewer_membership_id IS NULL OR reviewer_membership_id<>requester_membership_id),
 CONSTRAINT org_work_orders_closed_needs_expense CHECK(state<>'CLOSED' OR (expense_id IS NOT NULL AND NOT hold_open))
);
CREATE INDEX org_work_orders_asset_hold ON org_work_orders(org_id,asset_id,hold_open) WHERE hold_open;
CREATE INDEX org_work_orders_state ON org_work_orders(org_id,state,updated_at DESC);

CREATE FUNCTION org_work_orders_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 PERFORM 1 FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.id=NEW.asset_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 IF NEW.unit_id IS NOT NULL THEN
  PERFORM 1 FROM public.org_units u WHERE u.org_id=NEW.org_id AND u.id=NEW.unit_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 END IF;
 IF NEW.cost_centre_id IS NOT NULL THEN
  PERFORM 1 FROM public.org_cost_centres c WHERE c.org_id=NEW.org_id AND c.id=NEW.cost_centre_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found'; END IF;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='work order revision conflict'; END IF;
  NEW.created_at=OLD.created_at;
 ELSE
  NEW.created_at=clock_timestamp();
 END IF;
 NEW.updated_at=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER org_work_orders_guard BEFORE INSERT OR UPDATE ON org_work_orders FOR EACH ROW EXECUTE FUNCTION org_work_orders_guard();

-- EMP-MAINT-007: Asset & driver document expiry tracking.
CREATE TABLE org_compliance_items(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 subject_kind TEXT NOT NULL CHECK(subject_kind IN('ASSET','EMPLOYEE')),
 subject_id UUID NOT NULL,
 unit_id UUID,
 item_kind TEXT NOT NULL CHECK(length(btrim(item_kind)) BETWEEN 1 AND 64),
 expiry_day DATE NOT NULL,
 lead_days INTEGER NOT NULL DEFAULT 30 CHECK(lead_days BETWEEN 1 AND 365),
 state TEXT NOT NULL CHECK(state IN('UPCOMING','DUE','OVERDUE','DONE','SKIPPED')),
 document_id UUID,
 licence_category TEXT NOT NULL DEFAULT '',
 skip_reason TEXT NOT NULL DEFAULT '' CHECK(length(skip_reason)<=500),
 last_alert_day DATE,
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id)
);
CREATE INDEX org_compliance_items_subject ON org_compliance_items(org_id,subject_kind,subject_id,state,expiry_day);
CREATE TRIGGER org_compliance_items_scope BEFORE INSERT OR UPDATE ON org_compliance_items FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

-- EMP-MAINT-010, EMP-MAINT-011, EMP-MAINT-012: Components/tyres, parts stock, insurance policies.
CREATE TABLE org_fleet_components(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 asset_id UUID,
 kind TEXT NOT NULL CHECK(kind IN('TYRE','BATTERY','PART','OTHER')),
 serial_number TEXT NOT NULL CHECK(length(btrim(serial_number)) BETWEEN 1 AND 120),
 position TEXT NOT NULL DEFAULT '' CHECK(length(position)<=64),
 state TEXT NOT NULL CHECK(state IN('INSTALLED','REMOVED','SCRAPPED')),
 installed_at TIMESTAMPTZ CHECK(installed_at IS NULL OR isfinite(installed_at)),
 installed_meter_value BIGINT,
 removed_at TIMESTAMPTZ CHECK(removed_at IS NULL OR isfinite(removed_at)),
 removed_meter_value BIGINT,
 work_order_id UUID,
 reason TEXT NOT NULL DEFAULT '' CHECK(length(reason)<=500),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id)
);
CREATE UNIQUE INDEX org_fleet_components_installed ON org_fleet_components(org_id,serial_number) WHERE state='INSTALLED';
CREATE TRIGGER org_fleet_components_scope BEFORE INSERT OR UPDATE ON org_fleet_components FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

CREATE TABLE org_parts_stock_items(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 unit_id UUID,
 code TEXT NOT NULL CHECK(length(btrim(code)) BETWEEN 1 AND 64),
 name TEXT NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 160),
 unit TEXT NOT NULL CHECK(unit IN('UNIT','LITRE','KG','METRE')),
 quantity_milli BIGINT NOT NULL DEFAULT 0 CHECK(quantity_milli>=0),
 value_centavos BIGINT NOT NULL DEFAULT 0 CHECK(value_centavos>=0),
 work_order_id UUID,
 last_movement_kind TEXT NOT NULL DEFAULT 'RECEIPT',
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_parts_stock_code_unique UNIQUE(org_id,code)
);
CREATE TRIGGER org_parts_stock_scope BEFORE INSERT OR UPDATE ON org_parts_stock_items FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

CREATE TABLE org_insurance_policies(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 policy_number TEXT NOT NULL CHECK(length(btrim(policy_number)) BETWEEN 1 AND 80),
 insurer_name TEXT NOT NULL CHECK(length(btrim(insurer_name)) BETWEEN 1 AND 160),
 valid_from DATE NOT NULL,
 expiry_day DATE NOT NULL,
 asset_ids_json JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(asset_ids_json)='array'),
 state TEXT NOT NULL CHECK(state IN('ACTIVE','EXPIRED','CANCELLED')),
 claim_state TEXT NOT NULL DEFAULT 'NONE' CHECK(claim_state IN('NONE','OPEN','SETTLED','DENIED')),
 incident_report_id UUID,
 expense_id UUID,
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id)
);
CREATE TRIGGER org_insurance_policies_scope BEFORE INSERT OR UPDATE ON org_insurance_policies FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

-- EMP-KPI-001..005: Versioned KPI snapshots & price comparison snapshots.
CREATE TABLE org_kpi_snapshots(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 indicator_id TEXT NOT NULL CHECK(indicator_id~'^IND-[0-9]{3}$'),
 method_version TEXT NOT NULL,
 period_start DATE NOT NULL,
 period_end DATE NOT NULL,
 dimension_kind TEXT NOT NULL CHECK(dimension_kind IN('ORG','UNIT','COST_CENTRE','DRIVER','ASSET','TRIP','SUPPLIER')),
 dimension_id UUID,
 include_pending BOOLEAN NOT NULL DEFAULT false,
 status TEXT NOT NULL CHECK(status IN('COMPUTED','INSUFFICIENT_DATA')),
 insufficiency_reason TEXT NOT NULL DEFAULT '',
 value_rational TEXT NOT NULL DEFAULT '',
 display_value TEXT NOT NULL DEFAULT '',
 unit TEXT NOT NULL DEFAULT '',
 numerator BIGINT NOT NULL DEFAULT 0,
 denominator BIGINT NOT NULL DEFAULT 0,
 approved_count INTEGER NOT NULL DEFAULT 0,
 pending_count INTEGER NOT NULL DEFAULT 0,
 excluded_count INTEGER NOT NULL DEFAULT 0,
 sample_small BOOLEAN NOT NULL DEFAULT false,
 source_revision BIGINT NOT NULL DEFAULT 1 CHECK(source_revision>0),
 superseded_by_id UUID,
 computed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(computed_at)),
 PRIMARY KEY(org_id,id)
);
CREATE INDEX org_kpi_snapshots_lookup ON org_kpi_snapshots(org_id,indicator_id,dimension_kind,period_start,period_end,include_pending,computed_at DESC);
CREATE TRIGGER org_kpi_snapshots_scope BEFORE INSERT ON org_kpi_snapshots FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

CREATE TABLE org_price_comparisons(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 fueling_id UUID NOT NULL,
 fueling_revision BIGINT NOT NULL DEFAULT 1 CHECK(fueling_revision>0),
 reference_source TEXT NOT NULL CHECK(reference_source IN('OFFICIAL','COMMUNITY','ANNOUNCED','NEGOTIATED')),
 reference_revision BIGINT NOT NULL DEFAULT 1 CHECK(reference_revision>0),
 methodology_version TEXT NOT NULL DEFAULT 'price-comparison-v1',
 status TEXT NOT NULL CHECK(status IN('COMPUTED','INSUFFICIENT_DATA')),
 insufficiency_reason TEXT NOT NULL DEFAULT '',
 place_rule TEXT NOT NULL DEFAULT 'SAME_MUNICIPALITY' CHECK(place_rule IN('SAME_STATION','SAME_MUNICIPALITY','UNKNOWN')),
 product TEXT NOT NULL,
 unit TEXT NOT NULL,
 price_condition TEXT NOT NULL,
 reference_condition TEXT NOT NULL DEFAULT '',
 coverage_stations INTEGER NOT NULL DEFAULT 0,
 quantity_milli BIGINT NOT NULL,
 paid_cents BIGINT NOT NULL,
 effective_unit_price_milli BIGINT NOT NULL,
 reference_unit_price_milli BIGINT,
 reference_cents BIGINT,
 difference_cents BIGINT,
 difference_per_unit_milli BIGINT,
 superseded_by_id UUID,
 computed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(computed_at)),
 PRIMARY KEY(org_id,id)
);
CREATE INDEX org_price_comparisons_fueling ON org_price_comparisons(org_id,fueling_id,fueling_revision DESC);
CREATE TRIGGER org_price_comparisons_scope BEFORE INSERT ON org_price_comparisons FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

-- EMP-FIND-001..005: Finding detection settings, runs, and lifecycle.
CREATE TABLE org_finding_settings(
 org_id UUID NOT NULL PRIMARY KEY REFERENCES organizations(org_id),
 duplicate_window_minutes INTEGER NOT NULL DEFAULT 30 CHECK(duplicate_window_minutes BETWEEN 5 AND 1440),
 max_speed_kmh INTEGER NOT NULL DEFAULT 150 CHECK(max_speed_kmh BETWEEN 80 AND 200),
 tank_tolerance_percent INTEGER NOT NULL DEFAULT 5 CHECK(tank_tolerance_percent BETWEEN 0 AND 20),
 outlier_threshold_percent INTEGER NOT NULL DEFAULT 20 CHECK(outlier_threshold_percent BETWEEN 10 AND 50),
 doc_quantity_tolerance_percent INTEGER NOT NULL DEFAULT 1 CHECK(doc_quantity_tolerance_percent BETWEEN 0 AND 5),
 doc_date_tolerance_days INTEGER NOT NULL DEFAULT 1 CHECK(doc_date_tolerance_days BETWEEN 0 AND 7),
 price_threshold_percent INTEGER NOT NULL DEFAULT 5 CHECK(price_threshold_percent BETWEEN 1 AND 30),
 price_min_diff_cents BIGINT NOT NULL DEFAULT 1000 CHECK(price_min_diff_cents BETWEEN 0 AND 50000),
 reference_source TEXT NOT NULL DEFAULT 'OFFICIAL' CHECK(reference_source IN('OFFICIAL','COMMUNITY','ANNOUNCED','NEGOTIATED')),
 enabled_types_json JSONB NOT NULL DEFAULT '["FT-001","FT-002","FT-003","FT-004","FT-005","FT-006"]'::jsonb CHECK(jsonb_typeof(enabled_types_json)='array'),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at))
);
CREATE TRIGGER org_finding_settings_scope BEFORE INSERT OR UPDATE ON org_finding_settings FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

CREATE TABLE org_finding_runs(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 method_version TEXT NOT NULL DEFAULT 'findings-v1',
 period_start DATE NOT NULL,
 period_end DATE NOT NULL,
 state TEXT NOT NULL CHECK(state IN('RUNNING','COMPLETED','FAILED')),
 raised_count INTEGER NOT NULL DEFAULT 0,
 superseded_count INTEGER NOT NULL DEFAULT 0,
 requested_by_membership_id UUID NOT NULL,
 started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(started_at)),
 completed_at TIMESTAMPTZ CHECK(completed_at IS NULL OR isfinite(completed_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_finding_runs_actor_fkey FOREIGN KEY(org_id,requested_by_membership_id) REFERENCES org_memberships(org_id,id)
);
CREATE UNIQUE INDEX org_finding_runs_one_active ON org_finding_runs(org_id) WHERE state='RUNNING';
CREATE TRIGGER org_finding_runs_scope BEFORE INSERT OR UPDATE ON org_finding_runs FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

CREATE TABLE org_findings(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 finding_type TEXT NOT NULL CHECK(finding_type IN('FT-001','FT-002','FT-003','FT-004','FT-005','FT-006')),
 subject_key TEXT NOT NULL CHECK(length(btrim(subject_key)) BETWEEN 1 AND 240),
 asset_id UUID,
 unit_id UUID,
 cost_centre_id UUID,
 author_membership_id UUID,
 severity TEXT NOT NULL CHECK(severity IN('LOW','MEDIUM','HIGH')),
 state TEXT NOT NULL CHECK(state IN('OPEN','IN_REVIEW','CONFIRMED','EXPLAINED','FALSE_POSITIVE','SUPERSEDED')),
 method_version TEXT NOT NULL DEFAULT 'findings-v1',
 title TEXT NOT NULL CHECK(length(btrim(title)) BETWEEN 1 AND 280),
 non_proof_note TEXT NOT NULL CHECK(length(btrim(non_proof_note)) BETWEEN 1 AND 500),
 evidence_json JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(evidence_json)='object'),
 assigned_membership_id UUID,
 decided_by_membership_id UUID,
 decision_reason TEXT NOT NULL DEFAULT '' CHECK(length(decision_reason)<=500),
 followup_action TEXT NOT NULL DEFAULT '' CHECK(length(followup_action)<=200),
 run_id UUID,
 superseded_by_run_id UUID,
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_findings_self_decide CHECK(decided_by_membership_id IS NULL OR author_membership_id IS NULL OR decided_by_membership_id<>author_membership_id),
 CONSTRAINT org_findings_decided_reason CHECK(state NOT IN('CONFIRMED','EXPLAINED','FALSE_POSITIVE') OR length(btrim(decision_reason))>=1)
);
CREATE UNIQUE INDEX org_findings_one_open_subject ON org_findings(org_id,finding_type,subject_key,method_version) WHERE state IN('OPEN','IN_REVIEW');
CREATE TRIGGER org_findings_scope BEFORE INSERT OR UPDATE ON org_findings FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

-- EMP-RPT-004: Scheduled reports & per-recipient delivery.
CREATE TABLE org_report_schedules(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 owner_membership_id UUID NOT NULL,
 report_kind TEXT NOT NULL CHECK(length(btrim(report_kind)) BETWEEN 1 AND 64),
 cadence TEXT NOT NULL CHECK(cadence IN('DAILY','WEEKLY','MONTHLY')),
 parameters_json JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(parameters_json)='object'),
 recipient_membership_ids_json JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(recipient_membership_ids_json)='array'),
 state TEXT NOT NULL CHECK(state IN('ACTIVE','PAUSED')),
 pause_reason TEXT NOT NULL DEFAULT '' CHECK(length(pause_reason)<=300),
 last_run_at TIMESTAMPTZ CHECK(last_run_at IS NULL OR isfinite(last_run_at)),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_report_schedules_owner_fkey FOREIGN KEY(org_id,owner_membership_id) REFERENCES org_memberships(org_id,id)
);
CREATE TRIGGER org_report_schedules_scope BEFORE INSERT OR UPDATE ON org_report_schedules FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

-- EMP-EXP-007, EMP-FIN-005, EMP-DOC-012, EMP-PRIV-001/002/004, EMP-ORG-012,
-- EMP-AUTH-010, EMP-PUB-002.
CREATE TABLE org_recurring_and_fine_costs(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 asset_id UUID NOT NULL,
 unit_id UUID,
 cost_centre_id UUID,
 kind TEXT NOT NULL CHECK(kind IN('FINE','LEASE','INSURANCE','RECURRING_OTHER')),
 recurrence_key TEXT NOT NULL CHECK(length(btrim(recurrence_key)) BETWEEN 1 AND 120),
 period_month TEXT NOT NULL CHECK(period_month~'^[0-9]{4}-[0-9]{2}$'),
 amount_centavos BIGINT NOT NULL CHECK(amount_centavos BETWEEN 0 AND 1000000000000),
 expense_id UUID NOT NULL,
 state TEXT NOT NULL DEFAULT 'APPROVED' CHECK(state IN('SUBMITTED','APPROVED','VOIDED')),
 author_membership_id UUID NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL CHECK(isfinite(occurred_at)),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_recurring_costs_once UNIQUE(org_id,recurrence_key,period_month),
 CONSTRAINT org_recurring_costs_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id)
);
CREATE TRIGGER org_recurring_costs_scope BEFORE INSERT OR UPDATE ON org_recurring_and_fine_costs FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

CREATE TABLE org_payable_statements(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('CARD_STATEMENT','SUPPLIER_INVOICE')),
 supplier_id UUID,
 reference_code TEXT NOT NULL CHECK(length(btrim(reference_code)) BETWEEN 1 AND 120),
 period_month TEXT NOT NULL CHECK(period_month~'^[0-9]{4}-[0-9]{2}$'),
 payable_ids_json JSONB NOT NULL CHECK(jsonb_typeof(payable_ids_json)='array'),
 total_centavos BIGINT NOT NULL CHECK(total_centavos>=0),
 settled_centavos BIGINT NOT NULL DEFAULT 0 CHECK(settled_centavos>=0),
 state TEXT NOT NULL DEFAULT 'OPEN' CHECK(state IN('OPEN','SETTLED','CANCELLED')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id)
);
CREATE TRIGGER org_payable_statements_scope BEFORE INSERT OR UPDATE ON org_payable_statements FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

CREATE TABLE org_retention_and_holds(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 record_kind TEXT NOT NULL CHECK(record_kind IN('POLICY','LEGAL_HOLD','DELETION_LEDGER','DSR_REQUEST','ERASURE_TOMBSTONE')),
 category TEXT NOT NULL CHECK(length(btrim(category)) BETWEEN 1 AND 64),
 target_id UUID,
 retention_days INTEGER CHECK(retention_days IS NULL OR retention_days BETWEEN 1 AND 3650),
 hold_active BOOLEAN NOT NULL DEFAULT false,
 expires_at TIMESTAMPTZ CHECK(expires_at IS NULL OR isfinite(expires_at)),
 purged_at TIMESTAMPTZ CHECK(purged_at IS NULL OR isfinite(purged_at)),
 replay_hash TEXT NOT NULL DEFAULT '',
 responsible_context TEXT NOT NULL DEFAULT 'ORGANIZATION' CHECK(responsible_context IN('ORGANIZATION','PLATFORM')),
 state TEXT NOT NULL CHECK(length(btrim(state)) BETWEEN 1 AND 64),
 reason TEXT NOT NULL DEFAULT '' CHECK(length(reason)<=500),
 actor_membership_id UUID,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id)
);
CREATE INDEX org_retention_holds_lookup ON org_retention_and_holds(org_id,record_kind,category,target_id);
CREATE TRIGGER org_retention_holds_scope BEFORE INSERT OR UPDATE ON org_retention_and_holds FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

CREATE TABLE org_consolidation_grants(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 counterpart_org_id UUID NOT NULL REFERENCES organizations(org_id),
 allowed_datasets_json JSONB NOT NULL CHECK(jsonb_typeof(allowed_datasets_json)='array'),
 proposal_code_hash TEXT NOT NULL CHECK(length(btrim(proposal_code_hash))>=16),
 state TEXT NOT NULL CHECK(state IN('PROPOSED','ACTIVE','DECLINED','ENDED')),
 proposed_by_org_id UUID NOT NULL REFERENCES organizations(org_id),
 valid_until TIMESTAMPTZ NOT NULL CHECK(isfinite(valid_until)),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_consolidation_distinct CHECK(counterpart_org_id<>org_id)
);
CREATE TRIGGER org_consolidation_grants_scope BEFORE INSERT OR UPDATE ON org_consolidation_grants FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

CREATE TABLE org_custom_roles(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 key TEXT NOT NULL CHECK(key~'^[a-z][a-z0-9_]{1,63}$'),
 name TEXT NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 120),
 permissions_json JSONB NOT NULL CHECK(jsonb_typeof(permissions_json)='array'),
 state TEXT NOT NULL DEFAULT 'ACTIVE' CHECK(state IN('ACTIVE','ARCHIVED')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(updated_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_custom_roles_key_unique UNIQUE(org_id,key)
);
CREATE TRIGGER org_custom_roles_scope BEFORE INSERT OR UPDATE ON org_custom_roles FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

CREATE TABLE org_voluntary_price_publications(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 fueling_id UUID NOT NULL,
 station_id TEXT NOT NULL CHECK(length(btrim(station_id)) BETWEEN 1 AND 120),
 product TEXT NOT NULL,
 unit TEXT NOT NULL,
 unit_price_milli BIGINT NOT NULL CHECK(unit_price_milli>0),
 price_condition TEXT NOT NULL,
 published_by_membership_id UUID NOT NULL,
 published_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(published_at)),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_voluntary_pub_once UNIQUE(org_id,fueling_id),
 CONSTRAINT org_voluntary_pub_actor_fkey FOREIGN KEY(org_id,published_by_membership_id) REFERENCES org_memberships(org_id,id)
);
CREATE TRIGGER org_voluntary_pub_scope BEFORE INSERT ON org_voluntary_price_publications FOR EACH ROW EXECUTE FUNCTION org_f6_scope_guard();

DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY[
  'org_f6_commands','org_fuel_tanks','org_fuel_tank_movements','org_fuel_tank_reconciliations',
  'org_internal_fuelings','org_operating_cost_entries','org_asset_document_links',
  'org_maintenance_plans','org_maintenance_occurrences','org_work_orders','org_compliance_items',
  'org_fleet_components','org_parts_stock_items','org_insurance_policies','org_kpi_snapshots',
  'org_price_comparisons','org_finding_settings','org_finding_runs','org_findings',
  'org_report_schedules','org_recurring_and_fine_costs','org_payable_statements',
  'org_retention_and_holds','org_consolidation_grants','org_custom_roles',
  'org_voluntary_price_publications'
 ] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
  EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
  EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;
