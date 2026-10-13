-- Expired writer/cleaner leases and copy deadlines share the bounded queue.
CREATE INDEX evidence_final_output_cleanup_idx
 ON evidence_final_outputs (COALESCE(lease_until, deadline_at), final_key)
 WHERE state != 'PURGED';
