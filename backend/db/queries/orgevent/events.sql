-- Own intent/receipt tables; explicit audit/queue dependencies bind authority.
-- name: CurrentScope :one
SELECT COALESCE(current_setting('anpfuel.org_id',true),'')::text AS org_id;

-- name: InsertIntent :exec
INSERT INTO org_event_intents(org_id,event_id,consumer,event_name,target_type,target_id,revision,command_hash,job_id)
VALUES(CAST(@org_id::text AS uuid),CAST(@event_id::text AS uuid),@consumer,@event_name,@target_type,CAST(@target_id::text AS uuid),@revision,@command_hash,CAST(@job_id::text AS uuid));

-- name: LockLease :one
SELECT j.id::text FROM job_queue j
WHERE j.id=CAST(@job_id::text AS uuid) AND j.kind='org-event'
 AND j.status='leased' AND j.lease_token= @lease_token AND j.worker_id= @worker_id
 AND j.lease_expires_at>clock_timestamp() AND j.payload= @payload::jsonb
 AND j.dedupe_key= @dedupe_key AND j.max_attempts=5 AND j.attempts BETWEEN 1 AND 5
FOR UPDATE;

-- name: ReadIntent :one
SELECT i.event_name,i.target_type,i.target_id::text,i.revision
FROM org_event_intents i JOIN org_audit_events a ON a.org_id=i.org_id AND a.operation_id=i.event_id
WHERE i.org_id=CAST(@org_id::text AS uuid) AND i.event_id=CAST(@event_id::text AS uuid)
 AND i.consumer= @consumer AND i.job_id=CAST(@job_id::text AS uuid)
 AND i.command_hash=a.request_hash;

-- name: ReserveReceipt :one
INSERT INTO org_event_receipts(org_id,event_id,consumer)
VALUES(CAST(@org_id::text AS uuid),CAST(@event_id::text AS uuid),@consumer)
ON CONFLICT(org_id,event_id,consumer) DO NOTHING
RETURNING event_id::text;

-- name: LeaseStillLive :one
SELECT EXISTS(SELECT 1 FROM job_queue WHERE id=CAST(@job_id::text AS uuid)
 AND status='leased' AND lease_token= @lease_token AND worker_id= @worker_id
 AND lease_expires_at>clock_timestamp()) AS live;
