-- name: AssetRootLock :one
SELECT state FROM organizations WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND has_snapshot FOR UPDATE;

-- name: AssetCategoryByID :one
SELECT CAST(id AS text) AS id, label FROM org_asset_categories WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID);

-- name: AssetCategoryByLabel :one
SELECT CAST(id AS text) AS id, label FROM org_asset_categories WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND md5(label)=md5(sqlc.arg(label)::text) AND label COLLATE "C"= sqlc.arg(label)::text COLLATE "C";

-- name: InsertAssetCategory :exec
INSERT INTO org_asset_categories(org_id, id, label) VALUES(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(id)::text AS UUID), sqlc.arg(label)::text);

-- name: GetAsset :one
SELECT CAST(a.id AS text) AS id, a.kind, a.category_id::text AS category_id, c.label AS category_label, a.name, a.fleet_number, a.plate, a.serial_number, a.fleet_key, a.plate_key, a.usage_basis, a.state, a.revision, COALESCE(a.unit_id::text, '')::text AS unit_id
FROM org_assets a JOIN org_asset_categories c ON(c.org_id, c.id)=(a.org_id, a.category_id)
WHERE a.org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND a.id= CAST(sqlc.arg(id)::text AS UUID);

-- name: ListAssetIDs :many
SELECT CAST(id AS text) AS id FROM org_assets WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND(sqlc.arg(after_id)::text='' OR id>CAST(NULLIF(sqlc.arg(after_id)::text, '') AS UUID)) AND((sqlc.arg(state)::text='' AND state<>'ARCHIVED') OR state= sqlc.arg(state)::text) AND(sqlc.arg(kind)::text='' OR kind= sqlc.arg(kind)::text) ORDER BY id LIMIT sqlc.arg(row_limit)::int;

-- name: InsertAsset :exec
INSERT INTO org_assets(org_id, id, kind, category_id, name, fleet_number, plate, serial_number, fleet_key, plate_key, usage_basis, state, revision, unit_id)
VALUES(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(id)::text AS UUID), sqlc.arg(kind)::text, CAST(sqlc.arg(category_id)::text AS UUID), sqlc.arg(name)::text, sqlc.arg(fleet_number)::text, sqlc.arg(plate)::text, sqlc.arg(serial_number)::text, sqlc.arg(fleet_key)::text, sqlc.arg(plate_key)::text, sqlc.arg(usage_basis)::text, 'DRAFT', 1, CAST(NULLIF(sqlc.arg(unit_id)::text, '') AS UUID));

-- name: UpdateAsset :execrows
UPDATE org_assets SET kind= sqlc.arg(kind)::text, category_id= CAST(sqlc.arg(category_id)::text AS UUID), name= sqlc.arg(name)::text, fleet_number= sqlc.arg(fleet_number)::text, plate= sqlc.arg(plate)::text, serial_number= sqlc.arg(serial_number)::text, fleet_key= sqlc.arg(fleet_key)::text, plate_key= sqlc.arg(plate_key)::text, usage_basis= sqlc.arg(usage_basis)::text, state= sqlc.arg(state)::text, revision=revision+1, unit_id= CAST(NULLIF(sqlc.arg(unit_id)::text, '') AS UUID)
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) AND revision= sqlc.arg(expected_revision)::bigint;

-- name: DeleteAsset :execrows
DELETE FROM org_assets WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID) AND revision= sqlc.arg(expected_revision)::bigint;

-- name: AssetTanks :many
SELECT position, role, capacity, unit FROM org_asset_tanks WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND asset_id= CAST(sqlc.arg(asset_id)::text AS UUID) ORDER BY position;

-- name: AssetTankProducts :many
SELECT product FROM org_asset_tank_products WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND asset_id= CAST(sqlc.arg(asset_id)::text AS UUID) AND tank_position= sqlc.arg(tank_position)::int ORDER BY product COLLATE "C";

-- name: AssetExpected :many
SELECT product, basis, value, unit, source, to_char(reference_date, 'YYYY-MM-DD')::text AS reference_date FROM org_asset_expected WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND asset_id= CAST(sqlc.arg(asset_id)::text AS UUID) ORDER BY position;

-- name: DeleteAssetExpected :exec
DELETE FROM org_asset_expected WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND asset_id= CAST(sqlc.arg(asset_id)::text AS UUID);

-- name: DeleteAssetTanks :exec
DELETE FROM org_asset_tanks WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND asset_id= CAST(sqlc.arg(asset_id)::text AS UUID);

-- name: InsertAssetTank :exec
INSERT INTO org_asset_tanks(org_id, asset_id, position, role, capacity, unit) VALUES(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(asset_id)::text AS UUID), sqlc.arg(position)::int, sqlc.arg(role)::text, sqlc.arg(capacity)::bigint, sqlc.arg(unit)::text);

-- name: InsertAssetTankProduct :exec
INSERT INTO org_asset_tank_products(org_id, asset_id, tank_position, product, unit) VALUES(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(asset_id)::text AS UUID), sqlc.arg(tank_position)::int, sqlc.arg(product)::text, sqlc.arg(unit)::text);

-- name: InsertAssetExpected :exec
INSERT INTO org_asset_expected(org_id, asset_id, position, product, basis, value, unit, source, reference_date) VALUES(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(asset_id)::text AS UUID), sqlc.arg(position)::int, sqlc.arg(product)::text, sqlc.arg(basis)::text, sqlc.arg(value)::bigint, sqlc.arg(unit)::text, sqlc.arg(source)::text, CAST(sqlc.arg(reference_date)::text AS date));

-- name: AssetCommandReceipt :one
SELECT actor_id::text AS actor_id, asset_id::text AS asset_id, kind, request_hash, result_state, result_revision, discarded FROM org_asset_commands WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND operation_id= CAST(sqlc.arg(operation_id)::text AS UUID);

-- name: InsertAssetCommand :exec
INSERT INTO org_asset_commands(org_id, operation_id, actor_id, asset_id, kind, request_hash, result_state, result_revision, discarded) VALUES(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(operation_id)::text AS UUID), CAST(sqlc.arg(actor_id)::text AS UUID), CAST(sqlc.arg(asset_id)::text AS UUID), sqlc.arg(kind)::text, sqlc.arg(request_hash)::text, sqlc.arg(result_state)::text, sqlc.arg(result_revision)::bigint, sqlc.arg(discarded)::boolean);

-- name: InsertAssetRevision :exec
INSERT INTO org_asset_revisions(org_id, asset_id, revision, operation_id, snapshot) VALUES(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(asset_id)::text AS UUID), sqlc.arg(revision)::bigint, CAST(sqlc.arg(operation_id)::text AS UUID), sqlc.arg(snapshot)::jsonb);

-- name: InsertAssetAllocation :exec
INSERT INTO org_asset_unit_allocations(org_id, asset_id, unit_id) VALUES(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(asset_id)::text AS UUID), CAST(sqlc.arg(unit_id)::text AS UUID));

-- name: AssetCurrentAllocation :one
SELECT unit_id::text AS unit_id, started_at FROM org_asset_unit_allocations WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND asset_id= CAST(sqlc.arg(asset_id)::text AS UUID) AND ended_at IS NULL;

-- name: AssetUsage :one
SELECT org_asset_current_usage(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(asset_id)::text AS UUID))::jsonb AS usage;

-- name: AssetAdmitUnit :exec
SELECT org_unit_admit(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(unit_id)::text AS UUID));

-- name: AssetRootShare :one
SELECT state FROM organizations WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND has_snapshot FOR SHARE;
