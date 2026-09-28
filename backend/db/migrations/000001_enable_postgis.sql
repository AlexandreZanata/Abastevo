-- 000001: enable PostGIS under the deployment (migrator) role.
-- Append-only: never edit an applied migration; add a new one instead.
CREATE EXTENSION IF NOT EXISTS postgis;
