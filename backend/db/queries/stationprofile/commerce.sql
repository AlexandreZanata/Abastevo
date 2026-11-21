-- Owned by stationprofile (commerce, EMP-STN-002/004). The station's own
-- attributed commercial source: one active announcement per station,
-- product and unit; supersession and withdrawal are recorded state
-- transitions; expiry applies at read time. No official or community
-- row is reachable from here.

-- The insert reads the supersession result, so the stored active row leaves
-- the one-active index before the new row enters it. An unreferenced
-- data-modifying CTE would run after the insert and collide with that index.
-- name: InsertAnnouncedPrice :one
WITH superseded AS (
  UPDATE station_announced_prices SET state = 'SUPERSEDED'
  WHERE station_id = @station_id AND fuel_product = @fuel_product AND unit = @unit AND state = 'ACTIVE'
  RETURNING id
)
INSERT INTO station_announced_prices (id, station_id, fuel_product, unit, price_milli_brl, conditions, valid_from, valid_until, author_account_id, state)
SELECT @id, @station_id, @fuel_product, @unit, @price_milli_brl, @conditions, @valid_from, @valid_until, @author_account_id, 'ACTIVE'
FROM (SELECT count(*) FROM superseded) AS previous
RETURNING id, station_id, fuel_product, unit, price_milli_brl, conditions, valid_from, valid_until, author_account_id, state, created_at;

-- name: WithdrawAnnouncedPrice :one
UPDATE station_announced_prices SET state = 'WITHDRAWN'
WHERE id = @id AND station_id = @station_id AND state = 'ACTIVE'
RETURNING id, station_id, fuel_product, unit, price_milli_brl, conditions, valid_from, valid_until, author_account_id, state, created_at;

-- name: ListAnnouncedPrices :many
SELECT id, station_id, fuel_product, unit, price_milli_brl, conditions, valid_from, valid_until, author_account_id, state, created_at
FROM station_announced_prices
WHERE station_id = @station_id
ORDER BY created_at DESC LIMIT 200;

-- name: InsertPromotion :one
WITH withdrawn AS (
  UPDATE station_promotions SET state = 'WITHDRAWN'
  WHERE station_id = @station_id AND label = @label AND state = 'PUBLISHED'
  RETURNING id
)
INSERT INTO station_promotions (id, station_id, label, description, fuel_product, valid_from, valid_until, author_account_id, state)
SELECT @id, @station_id, @label, @description, @fuel_product, @valid_from, @valid_until, @author_account_id, 'PUBLISHED'
FROM (SELECT count(*) FROM withdrawn) AS previous
RETURNING id, station_id, label, description, fuel_product, valid_from, valid_until, author_account_id, state, created_at;

-- name: WithdrawPromotion :one
UPDATE station_promotions SET state = 'WITHDRAWN'
WHERE id = @id AND station_id = @station_id AND state = 'PUBLISHED'
RETURNING id, station_id, label, description, fuel_product, valid_from, valid_until, author_account_id, state, created_at;

-- name: ListPromotions :many
SELECT id, station_id, label, description, fuel_product, valid_from, valid_until, author_account_id, state, created_at
FROM station_promotions
WHERE station_id = @station_id
ORDER BY created_at DESC LIMIT 200;
