-- 000030: verified location bands on observations (P16-T03B, B-BR-L03).
-- Append-only: three nullable band columns, no history rewrite. The
-- device fix itself (coordinates, timestamps) never persists: submit
-- verifies the claim server-side, derives the bands, and discards the
-- fix in the same call. NULL means no location evidence was supplied.
-- location_verdict carries VERIFIED/DEGRADED/MANUAL/DENIED/SIMULATED/
-- UNKNOWN; location_proximity carries the submit-time NEAR/FAR/UNKNOWN
-- band (PostGIS distance against the precise station point, frozen
-- tolerance); location_reason carries teleport-suspect or the frozen
-- fix reason for audit. No coordinate, timestamp or URL column exists
-- here by design (asserted in the store suite via information_schema).
ALTER TABLE community_observations
    ADD COLUMN location_verdict TEXT NULL,
    ADD COLUMN location_proximity TEXT NULL,
    ADD COLUMN location_reason TEXT NULL;
