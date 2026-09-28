-- Owned by platform. Modules must not import another module's generated
-- package; cross-module reads use owned queries documented in DATA_MODEL.

-- name: PostGISVersion :one
SELECT PostGIS_version()::text;
