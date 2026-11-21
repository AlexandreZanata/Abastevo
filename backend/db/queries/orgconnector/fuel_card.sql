-- name: GetCardStatementLine :one
SELECT fact_hash, (fueling_id IS NOT NULL)::boolean AS linked
FROM org_card_statement_lines
WHERE (org_id,provider_code,source_account_id,transaction_hash)=(
    sqlc.arg('org_id')::text::uuid,sqlc.arg('provider_code')::text,
    sqlc.arg('source_account_id')::text::uuid,sqlc.arg('transaction_hash')::text);

-- name: VerifiedCardFueling :one
SELECT f.id::text AS id
FROM org_fuelings f
WHERE (f.org_id,f.id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('fueling_id')::text::uuid)
  AND f.payer='COMPANY' AND f.payment_method='FUEL_CARD'
  AND (f.occurred_at,f.occurred_submicro,f.paid_centavos)=(
      sqlc.arg('occurred_at')::timestamptz,sqlc.arg('occurred_submicro')::smallint,sqlc.arg('paid_centavos')::bigint)
  AND (SELECT e.state FROM org_fueling_review_entries e WHERE (e.org_id,e.fueling_id)=(f.org_id,f.id)
       ORDER BY e.sequence DESC LIMIT 1)='APPROVED'
  AND NOT EXISTS (SELECT 1 FROM org_card_statement_lines l WHERE (l.org_id,l.fueling_id)=(f.org_id,f.id));

-- name: StoreCardStatementLine :exec
INSERT INTO org_card_statement_lines (
    org_id,connection_id,provider_code,source_account_id,transaction_hash,fact_hash,
    occurred_at,occurred_submicro,paid_centavos,fueling_id,confirmed_by,operation_id
) VALUES (
    sqlc.arg('org_id')::text::uuid,sqlc.arg('connection_id')::text::uuid,sqlc.arg('provider_code')::text,
    sqlc.arg('source_account_id')::text::uuid,sqlc.arg('transaction_hash')::text,sqlc.arg('fact_hash')::text,
    sqlc.arg('occurred_at')::timestamptz,sqlc.arg('occurred_submicro')::smallint,sqlc.arg('paid_centavos')::bigint,
    NULLIF(sqlc.arg('fueling_id')::text,'')::uuid,sqlc.arg('confirmed_by')::text::uuid,sqlc.arg('operation_id')::text::uuid
);
