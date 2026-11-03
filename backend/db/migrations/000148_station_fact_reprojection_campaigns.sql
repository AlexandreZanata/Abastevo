-- SNA-02C: immutable accountable requests only; work/progress already belongs
-- to the platform queue. Creating this table does not enqueue or run backfill.
CREATE TABLE directory_station_fact_reprojection_campaigns (
 campaign_id UUID PRIMARY KEY,
 profile_version INTEGER NOT NULL CHECK (profile_version=1),
 operator_ref TEXT NOT NULL CHECK (octet_length(operator_ref) BETWEEN 1 AND 200 AND btrim(operator_ref)<>''),
 reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 500 AND btrim(reason)<>''),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
