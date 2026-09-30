-- Owned by feedback. One live rating per account/station/product
-- (P14-T02, B-BR-F02). Writers lock the live row, converge on equal
-- stars, bump the revision on change, then recompute the key stats in
-- the same transaction; a lost unique race re-reads the winner.

-- name: LockRating :one
SELECT id, account_id, station_id, product, stars, revision, created_at, deleted_at
FROM feedback_ratings
WHERE account_id = @account_id AND station_id = @station_id AND product = @product
    AND deleted_at IS NULL
FOR UPDATE;

-- name: InsertRating :exec
INSERT INTO feedback_ratings
    (id, account_id, station_id, product, stars, revision, created_at, deleted_at)
VALUES (@id, @account_id, @station_id, @product, @stars, @revision, @created_at, NULL);

-- name: UpdateRatingStars :exec
UPDATE feedback_ratings
SET stars = @stars, revision = @revision
WHERE id = @id AND deleted_at IS NULL;

-- name: TombstoneRating :execrows
UPDATE feedback_ratings
SET deleted_at = @now
WHERE account_id = @account_id AND station_id = @station_id AND product = @product
    AND deleted_at IS NULL;

-- name: CountSumLive :one
SELECT count(*)::bigint AS ratings_count, coalesce(sum(stars), 0)::bigint AS stars_sum
FROM feedback_ratings
WHERE station_id = @station_id AND product = @product AND deleted_at IS NULL;

-- name: UpsertStats :exec
INSERT INTO feedback_rating_stats
    (station_id, product, ratings_count, stars_sum, updated_at)
VALUES (@station_id, @product, @ratings_count, @stars_sum, @updated_at)
ON CONFLICT (station_id, product) DO UPDATE SET
    ratings_count = @ratings_count,
    stars_sum = @stars_sum,
    updated_at = @updated_at;

-- name: GetStats :one
SELECT station_id, product, ratings_count, stars_sum, updated_at
FROM feedback_rating_stats
WHERE station_id = @station_id AND product = @product;
