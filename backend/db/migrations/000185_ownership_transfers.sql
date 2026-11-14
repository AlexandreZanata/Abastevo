-- EMP-ORG-010: ownership transfer and the last-owner invariant.
CREATE TABLE org_ownership_transfers(
  org_id UUID NOT NULL REFERENCES organizations(org_id),
  transfer_id UUID NOT NULL CHECK(transfer_id <> '00000000-0000-0000-0000-000000000000'::uuid),
  initiator_membership_id UUID NOT NULL,
  target_membership_id UUID NOT NULL,
  step_down_to_admin BOOLEAN NOT NULL DEFAULT true,
  state TEXT NOT NULL CHECK(state IN ('PENDING','ACCEPTED','CANCELLED','DECLINED','EXPIRED')),
  expires_at TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  declined_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  operation_id UUID NOT NULL CHECK(operation_id <> '00000000-0000-0000-0000-000000000000'::uuid),
  PRIMARY KEY(org_id, transfer_id),
  FOREIGN KEY(org_id, initiator_membership_id) REFERENCES org_memberships(org_id, id),
  FOREIGN KEY(org_id, target_membership_id) REFERENCES org_memberships(org_id, id),
  CHECK(initiator_membership_id <> target_membership_id),
  CHECK(expires_at > created_at),
  CHECK((state = 'PENDING' AND accepted_at IS NULL AND cancelled_at IS NULL AND declined_at IS NULL)
     OR (state = 'ACCEPTED' AND accepted_at IS NOT NULL AND cancelled_at IS NULL AND declined_at IS NULL)
     OR (state = 'CANCELLED' AND cancelled_at IS NOT NULL AND accepted_at IS NULL AND declined_at IS NULL)
     OR (state = 'DECLINED' AND declined_at IS NOT NULL AND accepted_at IS NULL AND cancelled_at IS NULL)
     OR (state = 'EXPIRED' AND accepted_at IS NULL AND cancelled_at IS NULL AND declined_at IS NULL))
);

CREATE UNIQUE INDEX org_ownership_transfers_open ON org_ownership_transfers(org_id) WHERE state = 'PENDING';
CREATE UNIQUE INDEX org_ownership_transfers_op ON org_ownership_transfers(org_id, operation_id);
CREATE INDEX org_ownership_transfers_target ON org_ownership_transfers(org_id, target_membership_id) WHERE state = 'PENDING';

ALTER TABLE org_ownership_transfers ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_ownership_transfers FORCE ROW LEVEL SECURITY;

CREATE POLICY org_ownership_transfers_scope ON org_ownership_transfers
  USING(org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid)
  WITH CHECK(org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid);

REVOKE ALL ON org_ownership_transfers FROM PUBLIC;

CREATE TRIGGER org_ownership_transfers_history
  BEFORE DELETE OR TRUNCATE ON org_ownership_transfers
  FOR EACH STATEMENT EXECUTE FUNCTION org_identity_keep_history();

CREATE FUNCTION org_ownership_transfer_guard() RETURNS trigger
  LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;
BEGIN
  IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id', true), '')::uuid THEN
    RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='organization scope denied';
  END IF;

  SELECT state INTO root_state FROM public.organizations WHERE org_id = NEW.org_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='23503', MESSAGE='organization not found';
  END IF;

  IF TG_OP = 'INSERT' THEN
    IF root_state <> 'ACTIVE' THEN
      RAISE EXCEPTION USING ERRCODE='P7506', MESSAGE='organization must be active';
    END IF;
  ELSIF TG_OP = 'UPDATE' THEN
    IF OLD.state <> 'PENDING' THEN
      RAISE EXCEPTION USING ERRCODE='P7507', MESSAGE='transfer state is terminal';
    END IF;
    IF ROW(NEW.org_id, NEW.transfer_id, NEW.initiator_membership_id, NEW.target_membership_id, NEW.step_down_to_admin, NEW.expires_at, NEW.created_at, NEW.operation_id)
       IS DISTINCT FROM ROW(OLD.org_id, OLD.transfer_id, OLD.initiator_membership_id, OLD.target_membership_id, OLD.step_down_to_admin, OLD.expires_at, OLD.created_at, OLD.operation_id) THEN
      RAISE EXCEPTION USING ERRCODE='P7508', MESSAGE='transfer fields are immutable';
    END IF;
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER org_ownership_transfer_guard
  BEFORE INSERT OR UPDATE ON org_ownership_transfers
  FOR EACH ROW EXECUTE FUNCTION org_ownership_transfer_guard();

CREATE FUNCTION org_ownership_transfer_consume_membership() RETURNS trigger
  LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
  UPDATE public.org_ownership_transfers
     SET state = 'CANCELLED',
         cancelled_at = clock_timestamp()
   WHERE org_id = NEW.org_id
     AND target_membership_id = NEW.id
     AND state = 'PENDING';
  RETURN NEW;
END $$;

CREATE TRIGGER org_ownership_transfer_consume_membership
  AFTER UPDATE OF state ON org_memberships
  FOR EACH ROW
  WHEN (NEW.state IN ('SUSPENDED', 'ENDED') AND OLD.state IS DISTINCT FROM NEW.state)
  EXECUTE FUNCTION org_ownership_transfer_consume_membership();

CREATE FUNCTION org_ownership_transfer_consume_organization() RETURNS trigger
  LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
  UPDATE public.org_ownership_transfers
     SET state = 'CANCELLED',
         cancelled_at = clock_timestamp()
   WHERE org_id = NEW.org_id
     AND state = 'PENDING';
  RETURN NEW;
END $$;

CREATE TRIGGER org_ownership_transfer_consume_organization
  AFTER UPDATE OF state ON organizations
  FOR EACH ROW
  WHEN (NEW.state IN ('ARCHIVED', 'CLOSED') AND OLD.state IS DISTINCT FROM NEW.state)
  EXECUTE FUNCTION org_ownership_transfer_consume_organization();

DO $$ DECLARE owner_name TEXT;
BEGIN
  SELECT pg_catalog.pg_get_userbyid(relowner) INTO owner_name
    FROM pg_catalog.pg_class WHERE oid = 'public.org_memberships'::regclass;
  EXECUTE pg_catalog.format('GRANT SELECT,INSERT,UPDATE ON public.org_ownership_transfers TO %I', owner_name);
END $$;
