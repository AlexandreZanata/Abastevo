-- FQ-13: derived official per-station/per-fuel dimensions. Derived facts are
-- versioned by methodology and policy, bound to one published revision and
-- to the input digest that produced them; the score stays NULL/UNSCORED
-- until calibration is accepted. Corrected source revisions recompute through
-- the current-pointer fence, and stale bases can never overwrite a newer one.
CREATE TABLE quality_station_fuel_projections(
 station_id UUID NOT NULL REFERENCES directory_stations(id) ON DELETE RESTRICT,
 fuel TEXT NOT NULL CHECK(fuel IN
  ('ETHANOL','GASOLINE_REGULAR','GASOLINE_ADDITIVED','GASOLINE_PREMIUM_GRADE',
   'DIESEL_S500','DIESEL_S10','CNG','LPG_P13')),
 methodology TEXT NOT NULL CHECK(length(methodology) BETWEEN 1 AND 64),
 policy TEXT NOT NULL CHECK(length(policy) BETWEEN 1 AND 64),
 basis_source TEXT NOT NULL CHECK(length(basis_source) BETWEEN 1 AND 32),
 basis_stream TEXT NOT NULL CHECK(length(basis_stream) BETWEEN 1 AND 64),
 basis_revision UUID NOT NULL REFERENCES quality_revisions(id) ON DELETE RESTRICT,
 basis_published_at TIMESTAMPTZ NOT NULL,
 input_digest TEXT NOT NULL CHECK(input_digest ~ '^[0-9a-f]{64}$'),
 samples BIGINT NOT NULL CHECK(samples >= 0),
 visits BIGINT NOT NULL CHECK(visits >= 0),
 conforming BIGINT NOT NULL CHECK(conforming >= 0),
 nonconforming BIGINT NOT NULL CHECK(nonconforming >= 0),
 unresolved BIGINT NOT NULL CHECK(unresolved >= 0),
 oldest_observed_on DATE,
 newest_observed_on DATE,
 score_state TEXT NOT NULL CHECK(score_state IN ('UNSCORED')),
 score INTEGER,
 generation BIGINT NOT NULL DEFAULT 1 CHECK(generation >= 1),
 computed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(station_id, fuel, methodology),
 CHECK(conforming + nonconforming + unresolved = samples)
);
