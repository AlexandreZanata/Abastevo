-- EMP-APPR-006. Organization-wide calendar-month reporting periods (EDR-148).
-- A month without history is OPEN at the implicit revision 1 and is never
-- stored. Each lock, reopening and re-lock appends one immutable transition,
-- which is also the command receipt; the head is maintained only by trigger.
-- Period commands hold the organization root FOR UPDATE; record commands read
-- the head after FOR KEY SHARE on the same root (EDR-149, B-BR-EMP-214).
CREATE TABLE org_finance_period_transitions(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 period_month DATE NOT NULL CHECK(EXTRACT(DAY FROM period_month)=1 AND period_month>='0001-01-01' AND period_month<='9999-12-01'),
 period_id UUID NOT NULL CHECK(period_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 revision BIGINT NOT NULL CHECK(revision>=2),
 kind TEXT NOT NULL,from_state TEXT NOT NULL,to_state TEXT NOT NULL,authority TEXT NOT NULL,reason TEXT NOT NULL,
 actor_account_id UUID NOT NULL REFERENCES accounts(id),actor_membership_id UUID NOT NULL,
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(recorded_at)),
 PRIMARY KEY(org_id,period_month,revision),UNIQUE(org_id,operation_id),
 FOREIGN KEY(org_id,actor_membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((kind='LOCK' AND from_state='OPEN' AND to_state='LOCKED' AND revision=2 AND authority='fin.period.close' AND reason='')
  OR(kind='REOPEN' AND from_state='LOCKED' AND to_state='REOPENED' AND revision%2=1 AND authority='fin.period.reopen' AND org_asset_text_valid(reason,280,true))
  OR(kind='RELOCK' AND from_state='REOPENED' AND to_state='LOCKED' AND revision>=4 AND revision%2=0 AND authority='fin.period.close' AND reason=''))
);
CREATE TABLE org_finance_periods(
 org_id UUID NOT NULL REFERENCES organizations(org_id),period_month DATE NOT NULL,
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 state TEXT NOT NULL CHECK(state IN('LOCKED','REOPENED')),
 revision BIGINT NOT NULL CHECK(revision>=2 AND(state='LOCKED')=(revision%2=0)),
 PRIMARY KEY(org_id,period_month),UNIQUE(org_id,id),
 FOREIGN KEY(org_id,period_month,revision) REFERENCES org_finance_period_transitions(org_id,period_month,revision)
);
ALTER TABLE org_finance_period_transitions ENABLE ROW LEVEL SECURITY;ALTER TABLE org_finance_period_transitions FORCE ROW LEVEL SECURITY;
ALTER TABLE org_finance_periods ENABLE ROW LEVEL SECURITY;ALTER TABLE org_finance_periods FORCE ROW LEVEL SECURITY;
CREATE POLICY org_finance_period_transition_scope ON org_finance_period_transitions USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_finance_period_scope ON org_finance_periods USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_finance_period_transitions,org_finance_periods FROM PUBLIC;
-- History and receipts are append-only; a head is never removed.
CREATE TRIGGER org_finance_period_transition_history BEFORE UPDATE OR DELETE OR TRUNCATE ON org_finance_period_transitions FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();
CREATE TRIGGER org_finance_period_history BEFORE DELETE OR TRUNCATE ON org_finance_periods FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();

-- A transition is admitted only as the next revision of the current head, by
-- a current ACTIVE member of an ACTIVE organization, under the root lock.
CREATE FUNCTION org_finance_period_transition_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE head public.org_finance_periods%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND state='ACTIVE' AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept period changes';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.actor_membership_id AND account_id=NEW.actor_account_id AND state='ACTIVE') THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='period actor unavailable';END IF;
 SELECT * INTO head FROM public.org_finance_periods WHERE org_id=NEW.org_id AND period_month=NEW.period_month FOR UPDATE;
 IF NOT FOUND THEN
  IF NEW.revision<>2 OR NEW.from_state<>'OPEN' OR EXISTS(SELECT 1 FROM public.org_finance_periods WHERE org_id=NEW.org_id AND id=NEW.period_id) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='stale period transition';END IF;
 ELSIF head.id<>NEW.period_id OR head.revision=9223372036854775807 OR NEW.revision<>head.revision+1 OR NEW.from_state<>head.state THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='stale period transition';
 END IF;
 NEW.recorded_at=clock_timestamp();
 IF EXISTS(SELECT 1 FROM public.org_finance_period_transitions WHERE org_id=NEW.org_id AND period_month=NEW.period_month AND recorded_at>NEW.recorded_at) THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='period clock unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_finance_period_transition_guard BEFORE INSERT ON org_finance_period_transitions FOR EACH ROW EXECUTE FUNCTION org_finance_period_transition_guard();
CREATE FUNCTION org_finance_period_project() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.revision=2 THEN
  INSERT INTO public.org_finance_periods(org_id,period_month,id,state,revision) VALUES(NEW.org_id,NEW.period_month,NEW.period_id,NEW.to_state,NEW.revision);
 ELSE
  UPDATE public.org_finance_periods SET state=NEW.to_state,revision=NEW.revision WHERE org_id=NEW.org_id AND period_month=NEW.period_month AND id=NEW.period_id AND revision=NEW.revision-1;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='stale period transition';END IF;
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER org_finance_period_project AFTER INSERT ON org_finance_period_transitions FOR EACH ROW EXECUTE FUNCTION org_finance_period_project();
-- A head changes only to the revision of an existing transition of its period.
CREATE FUNCTION org_finance_period_head_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 IF(TG_OP='INSERT' AND NEW.revision<>2) OR(TG_OP='UPDATE' AND(NEW.org_id<>OLD.org_id OR NEW.period_month<>OLD.period_month OR NEW.id<>OLD.id OR NEW.revision<>OLD.revision+1))
  OR NOT EXISTS(SELECT 1 FROM public.org_finance_period_transitions WHERE org_id=NEW.org_id AND period_month=NEW.period_month AND revision=NEW.revision AND period_id=NEW.id AND to_state=NEW.state) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='period head without transition';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_finance_period_head_guard BEFORE INSERT OR UPDATE ON org_finance_periods FOR EACH ROW EXECUTE FUNCTION org_finance_period_head_guard();
