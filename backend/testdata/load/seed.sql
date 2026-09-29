-- Deterministic load seed (P08-T07): :STATIONS directory rows with
-- stable UUIDs derived from the sequence (no randomness), safe to
-- re-run (idempotent by primary key). Staging campaigns use larger
-- counts with the same shape; the 100k/1M acceptance matrix runs on
-- provisioned infrastructure (P09), never here.
INSERT INTO directory_stations (id, display_name, municipality_code, state)
SELECT ('b0000000-0000-4000-8000-' || lpad(to_hex(g), 12, '0'))::uuid,
    'Load Station ' || g,
    lpad((1000000 + (g % 1000))::text, 7, '0'),
    (ARRAY['SP', 'RJ', 'MG', 'RS'])[1 + (g % 4)]
FROM generate_series(1, :STATIONS) g
ON CONFLICT (id) DO NOTHING;
