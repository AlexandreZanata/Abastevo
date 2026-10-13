-- Safety dispatchers claim one kind independently of general queue history.
CREATE INDEX job_queue_live_kind_created_idx
    ON job_queue (kind, created_at, id) WHERE status IN ('queued','leased');
