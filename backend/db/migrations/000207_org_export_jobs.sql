-- EMP-RPT-001. Organization export jobs (EDR-156, B-BR-EMP-227..230).
-- One row per requested export: the requester, the admitted kind, canonical
-- parameters and their digest, the per-job ceilings, the lifecycle state, a
-- fenced worker lease and the identity of at most one private object. The
-- object itself lives in private storage under org/<org_id>/export/<id>/
-- <sha256>; no content, storage key, URL or capability is ever stored here.
-- Metadata is kept after expiry for the audit period; rows are never deleted.
CREATE TABLE org_export_jobs(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 requester_account_id UUID NOT NULL CHECK(requester_account_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 requester_membership_id UUID NOT NULL CHECK(requester_membership_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 kind TEXT NOT NULL CHECK(octet_length(kind)<=64 AND kind~'^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
 parameters TEXT NOT NULL CHECK(octet_length(parameters) BETWEEN 2 AND 4096 AND jsonb_typeof(parameters::jsonb)='object'),
 parameters_digest TEXT NOT NULL CHECK(parameters_digest~'^[0-9a-f]{64}$'),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[0-9a-f]{64}$'),
 media_type TEXT NOT NULL CHECK(octet_length(media_type)<=128 AND media_type~'^[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*$'),
 row_limit BIGINT NOT NULL CHECK(row_limit BETWEEN 1 AND 500000),
 byte_limit BIGINT NOT NULL CHECK(byte_limit BETWEEN 1 AND 16777216),
 time_limit_ms INTEGER NOT NULL CHECK(time_limit_ms BETWEEN 1000 AND 120000),
 retention_seconds INTEGER NOT NULL CHECK(retention_seconds BETWEEN 86400 AND 2592000),
 max_attempts SMALLINT NOT NULL CHECK(max_attempts BETWEEN 1 AND 10),
 state TEXT NOT NULL CHECK(state IN('QUEUED','RUNNING','READY','EXPIRED','FAILED')),
 attempts SMALLINT NOT NULL CHECK(attempts>=0 AND attempts<=max_attempts),
 lease_token UUID,
 lease_expires_at TIMESTAMPTZ,
 object_sha256 TEXT CHECK(object_sha256~'^[0-9a-f]{64}$'),
 object_bytes BIGINT,
 row_count BIGINT,
 failure_code TEXT CHECK(failure_code IN('EXPORT_TOO_LARGE','EXPORT_RIGHTS_LOST','EXPORT_FORMAT_UNAVAILABLE','EXPORT_ATTEMPTS_EXHAUSTED')),
 requested_at TIMESTAMPTZ NOT NULL CHECK(isfinite(requested_at)),
 as_of TIMESTAMPTZ NOT NULL CHECK(as_of=requested_at),
 updated_at TIMESTAMPTZ NOT NULL CHECK(updated_at>=requested_at),
 ready_at TIMESTAMPTZ,
 expires_at TIMESTAMPTZ,
 failed_at TIMESTAMPTZ,
 expired_at TIMESTAMPTZ,
 object_deleted_at TIMESTAMPTZ,
 PRIMARY KEY(org_id,id),
 UNIQUE(org_id,operation_id),
 -- Every job has its request audit event, written in the same transaction.
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((object_sha256 IS NULL)=(object_bytes IS NULL) AND (object_sha256 IS NULL)=(row_count IS NULL)),
 CHECK(object_bytes IS NULL OR object_bytes BETWEEN 1 AND byte_limit),
 CHECK(row_count IS NULL OR row_count BETWEEN 0 AND row_limit),
 CHECK((state='RUNNING')=(lease_token IS NOT NULL) AND (lease_token IS NULL)=(lease_expires_at IS NULL)),
 CHECK(state<>'QUEUED' OR attempts<max_attempts),
 CHECK(state NOT IN('RUNNING','READY','EXPIRED') OR attempts>=1),
 CHECK((state IN('READY','EXPIRED'))=(ready_at IS NOT NULL) AND (ready_at IS NULL)=(expires_at IS NULL)),
 CHECK(ready_at IS NULL OR (ready_at>=requested_at AND expires_at>ready_at AND object_sha256 IS NOT NULL)),
 CHECK((state='FAILED')=(failure_code IS NOT NULL) AND (state='FAILED')=(failed_at IS NOT NULL)),
 CHECK((state='EXPIRED')=(expired_at IS NOT NULL)),
 CHECK(object_deleted_at IS NULL OR (state IN('FAILED','EXPIRED') AND object_sha256 IS NOT NULL))
);
-- Requester pages run newest first; the open-job limit counts live jobs.
CREATE INDEX org_export_jobs_requester ON org_export_jobs(org_id,requester_membership_id,requested_at DESC,id DESC);
CREATE INDEX org_export_jobs_open ON org_export_jobs(org_id,requester_membership_id) WHERE state IN('QUEUED','RUNNING');

ALTER TABLE org_export_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_export_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY org_export_jobs_scope ON org_export_jobs
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_export_jobs FROM PUBLIC;

-- Export metadata is retained; rows are never deleted.
CREATE FUNCTION org_export_jobs_retained() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='export metadata is retained'; END $$;
CREATE TRIGGER org_export_jobs_retained BEFORE DELETE OR TRUNCATE ON org_export_jobs
 FOR EACH STATEMENT EXECUTE FUNCTION org_export_jobs_retained();

-- A job starts QUEUED for a current ACTIVE member; afterwards only the closed
-- lifecycle may change it. Server instants are set here, never by callers.
-- Identity, requester, kind, parameters, ceilings and receipt are immutable.
CREATE FUNCTION org_export_job_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE
 now_at TIMESTAMPTZ := clock_timestamp();
 same_object BOOLEAN;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'QUEUED' OR NEW.attempts<>0 OR NEW.lease_token IS NOT NULL OR NEW.object_sha256 IS NOT NULL OR NEW.failure_code IS NOT NULL
   OR NEW.ready_at IS NOT NULL OR NEW.expires_at IS NOT NULL OR NEW.failed_at IS NOT NULL OR NEW.expired_at IS NOT NULL OR NEW.object_deleted_at IS NOT NULL THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export job must start queued';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM public.org_memberships WHERE org_id=NEW.org_id AND id=NEW.requester_membership_id
   AND account_id=NEW.requester_account_id AND state='ACTIVE') THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export requester unavailable';
  END IF;
  NEW.requested_at:=now_at; NEW.as_of:=now_at; NEW.updated_at:=now_at;
  RETURN NEW;
 END IF;
 IF (NEW.org_id,NEW.id,NEW.operation_id,NEW.requester_account_id,NEW.requester_membership_id,NEW.kind,NEW.parameters,NEW.parameters_digest,
     NEW.request_hash,NEW.media_type,NEW.row_limit,NEW.byte_limit,NEW.time_limit_ms,NEW.retention_seconds,NEW.max_attempts,NEW.requested_at,NEW.as_of)
   IS DISTINCT FROM
    (OLD.org_id,OLD.id,OLD.operation_id,OLD.requester_account_id,OLD.requester_membership_id,OLD.kind,OLD.parameters,OLD.parameters_digest,
     OLD.request_hash,OLD.media_type,OLD.row_limit,OLD.byte_limit,OLD.time_limit_ms,OLD.retention_seconds,OLD.max_attempts,OLD.requested_at,OLD.as_of) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export job identity is immutable';
 END IF;
 IF (NEW.ready_at,NEW.expires_at,NEW.failed_at,NEW.expired_at) IS DISTINCT FROM (OLD.ready_at,OLD.expires_at,OLD.failed_at,OLD.expired_at)
   OR NEW.object_deleted_at IS DISTINCT FROM OLD.object_deleted_at AND (OLD.object_deleted_at IS NOT NULL OR NEW.state<>OLD.state) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export instants are server-owned';
 END IF;
 same_object:=(NEW.object_sha256,NEW.object_bytes,NEW.row_count) IS NOT DISTINCT FROM (OLD.object_sha256,OLD.object_bytes,OLD.row_count);
 IF OLD.state='QUEUED' AND NEW.state='RUNNING' THEN
  -- A claim: one more attempt under a fresh live lease.
  IF NEW.attempts<>OLD.attempts+1 OR NEW.lease_expires_at<=now_at OR NOT same_object THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export claim refused';
  END IF;
 ELSIF OLD.state='RUNNING' AND NEW.state='RUNNING' THEN
  IF NEW.lease_token IS DISTINCT FROM OLD.lease_token THEN
   -- A new holder only after the previous lease expired.
   IF OLD.lease_expires_at>now_at OR NEW.attempts<>OLD.attempts+1 OR NEW.lease_expires_at<=now_at OR NOT same_object THEN
    RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export lease is held';
   END IF;
  ELSIF OLD.lease_expires_at<=now_at OR NEW.attempts<>OLD.attempts OR NEW.lease_expires_at<>OLD.lease_expires_at OR NEW.object_sha256 IS NULL THEN
   -- The live holder may only record its provisional object.
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export lease expired';
  END IF;
 ELSIF OLD.state='RUNNING' AND NEW.state='QUEUED' THEN
  IF NEW.attempts<>OLD.attempts OR NOT same_object THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export release refused';
  END IF;
 ELSIF OLD.state='RUNNING' AND NEW.state='READY' THEN
  -- Only the live holder publishes, and only the recorded object.
  IF OLD.lease_expires_at<=now_at OR NEW.attempts<>OLD.attempts OR NOT same_object OR OLD.object_sha256 IS NULL THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export publication refused';
  END IF;
  NEW.ready_at:=now_at; NEW.expires_at:=now_at+make_interval(secs=>NEW.retention_seconds);
 ELSIF OLD.state IN('QUEUED','RUNNING') AND NEW.state='FAILED' THEN
  IF NEW.attempts<>OLD.attempts OR NOT same_object OR NEW.failure_code IS NULL THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export failure refused';
  END IF;
  NEW.failed_at:=now_at;
 ELSIF OLD.state='READY' AND NEW.state='EXPIRED' THEN
  IF OLD.expires_at>now_at OR NEW.attempts<>OLD.attempts OR NOT same_object THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export not expired';
  END IF;
  NEW.expired_at:=now_at;
 ELSIF OLD.state IN('FAILED','EXPIRED') AND NEW.state=OLD.state THEN
  -- Settled jobs change only once more: recording the confirmed deletion.
  IF OLD.object_sha256 IS NULL OR OLD.object_deleted_at IS NOT NULL OR NEW.object_deleted_at IS NULL OR NEW.attempts<>OLD.attempts
   OR NOT same_object OR NEW.failure_code IS DISTINCT FROM OLD.failure_code THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='settled export is immutable';
  END IF;
  NEW.object_deleted_at:=now_at;
 ELSE
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export transition refused';
 END IF;
 IF NEW.state<>'FAILED' AND NEW.failure_code IS NOT NULL THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='export failure code without failure';
 END IF;
 NEW.updated_at:=GREATEST(now_at,OLD.updated_at);
 RETURN NEW;
END $$;
CREATE TRIGGER org_export_job_guard BEFORE INSERT OR UPDATE ON org_export_jobs
 FOR EACH ROW EXECUTE FUNCTION org_export_job_guard();
