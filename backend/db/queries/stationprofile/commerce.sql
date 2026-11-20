-- Owned by stationprofile (commerce, EMP-STN-002/004). The station's own
-- attributed commercial source: one active announcement per station,
-- product and unit; supersession and withdrawal are recorded state
-- transitions; expiry applies at read time. No official or community
-- row is reachable from here.

-- name: InsertAnnouncedPrice :one
WITH superseded AS (
  UPDATE station_announced_prices SET state = 'SUPERSEDED'
  WHERE station_id = @station_id AND fuel_product = @fuel_product AND unit = @unit AND state = 'ACTIVE'
)
INSERT INTO station_announced_prices (id, station_id, fuel_product, unit, price_milli_brl, conditions, valid_from, valid_until, author_account_id, state)
VALUES (@id, @station_id, @fuel_product, @unit, @price_milli_brl, @conditions, @valid_from, @valid_until, @author_account_id, 'ACTIVE')
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
)
INSERT INTO station_promotions (id, station_id, label, description, fuel_product, valid_from, valid_until, author_account_id, state)
VALUES (@id, @station_id, @label, @description, @fuel_product, @valid_from, @valid_until, @author_account_id, 'PUBLISHED')
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
