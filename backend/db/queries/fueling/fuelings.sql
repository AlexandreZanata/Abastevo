-- name: FuelingAuthorEmployee :one
-- EMP-FUEL-002: the author's employee record, read in the intake transaction.
SELECT employee_id::text AS employee_id FROM org_memberships
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(membership_id)::text AS uuid);

-- name: FuelingReferenceFacts :one
-- Admission facts of one fueling, taken after the root and asset locks.
SELECT a.state AS asset_state,
 EXISTS(SELECT 1 FROM org_employee_identities e WHERE e.org_id=a.org_id AND e.id=CAST(sqlc.arg(responsible_id)::text AS uuid) AND e.state='ACTIVE' AND e.merged_into IS NULL)::boolean AS responsible_active,
 EXISTS(SELECT 1 FROM org_asset_tank_products p WHERE p.org_id=a.org_id AND p.asset_id=a.id AND p.product=sqlc.arg(product)::text)::boolean AS product_accepted,
 COALESCE((SELECT f.operation_id::text FROM org_fuelings f WHERE f.org_id=a.org_id AND f.id=CAST(sqlc.arg(fueling_id)::text AS uuid)),'')::text AS existing_operation
FROM org_assets a WHERE a.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND a.id=CAST(sqlc.arg(asset_id)::text AS uuid);

-- name: InsertFueling :exec
INSERT INTO org_fuelings(org_id,id,operation_id,payload_digest,asset_id,responsible_employee_id,author_membership_id,author_employee_id,enrollment_id,
 occurred_at,occurred_submicro,entry_zone,device_offset_seconds,product,unit,quantity,paid_centavos,discount_centavos,unit_price_milli_brl,unit_price_text,
 fill,payer,payment_method,origin,place_kind,place_name,meter_id,segment_id,reading_id,missing_reading_reason,unit_id,cost_centre_id,
 gross_centavos,expected_centavos,difference_centavos,derived_price_milli_brl,review_state,revision)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(payload_digest)::text,
 CAST(sqlc.arg(asset_id)::text AS uuid),CAST(sqlc.arg(responsible_employee_id)::text AS uuid),CAST(sqlc.arg(author_membership_id)::text AS uuid),
 CAST(sqlc.arg(author_employee_id)::text AS uuid),CAST(sqlc.arg(enrollment_id)::text AS uuid),
 sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint,
 sqlc.arg(product)::text,sqlc.arg(unit)::text,sqlc.arg(quantity)::bigint,sqlc.arg(paid_centavos)::bigint,sqlc.narg(discount_centavos)::bigint,
 sqlc.narg(unit_price_milli_brl)::bigint,sqlc.narg(unit_price_text)::text,sqlc.arg(fill)::text,sqlc.arg(payer)::text,sqlc.arg(payment_method)::text,
 sqlc.arg(origin)::text,sqlc.narg(place_kind)::text,sqlc.narg(place_name)::text,CAST(sqlc.narg(meter_id)::text AS uuid),CAST(sqlc.narg(segment_id)::text AS uuid),
 CAST(sqlc.narg(reading_id)::text AS uuid),sqlc.narg(missing_reading_reason)::text,CAST(sqlc.narg(unit_id)::text AS uuid),CAST(sqlc.narg(cost_centre_id)::text AS uuid),
 sqlc.arg(gross_centavos)::bigint,sqlc.narg(expected_centavos)::bigint,sqlc.narg(difference_centavos)::bigint,sqlc.narg(derived_price_milli_brl)::text,
 sqlc.arg(review_state)::text,sqlc.arg(revision)::bigint);

-- name: InsertFuelingWarning :exec
INSERT INTO org_fueling_warnings(org_id,fueling_id,revision,code)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(fueling_id)::text AS uuid),sqlc.arg(revision)::bigint,sqlc.arg(code)::text);

-- name: InsertFuelingReviewEntry :exec
INSERT INTO org_fueling_review_entries(org_id,fueling_id,sequence,kind,revision,state,reason,actor_membership_id,operation_id)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(fueling_id)::text AS uuid),sqlc.arg(sequence)::bigint,sqlc.arg(kind)::text,
 sqlc.arg(revision)::bigint,sqlc.arg(state)::text,sqlc.arg(reason)::text,CAST(sqlc.arg(actor_membership_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid));

-- name: GetFueling :one
SELECT f.id::text AS id,f.operation_id::text AS operation_id,f.payload_digest,f.asset_id::text AS asset_id,
 f.responsible_employee_id::text AS responsible_employee_id,f.author_membership_id::text AS author_membership_id,
 f.author_employee_id::text AS author_employee_id,f.enrollment_id::text AS enrollment_id,
 f.occurred_at,f.occurred_submicro,f.entry_zone,f.device_offset_seconds,f.received_at,f.product,f.unit,f.quantity,f.paid_centavos,
 f.discount_centavos,f.unit_price_milli_brl,f.unit_price_text,f.fill,f.payer,f.payment_method,f.origin,f.place_kind,f.place_name,
 COALESCE(f.meter_id::text,'')::text AS meter_id,COALESCE(f.segment_id::text,'')::text AS segment_id,COALESCE(f.reading_id::text,'')::text AS reading_id,
 f.missing_reading_reason,COALESCE(f.unit_id::text,'')::text AS unit_id,COALESCE(f.cost_centre_id::text,'')::text AS cost_centre_id,
 f.gross_centavos,f.expected_centavos,f.difference_centavos,f.derived_price_milli_brl,f.review_state,f.revision,
 COALESCE((SELECT array_agg(w.code ORDER BY w.code) FROM org_fueling_warnings w WHERE w.org_id=f.org_id AND w.fueling_id=f.id AND w.revision=f.revision),'{}')::text[] AS warnings,
 COALESCE((SELECT r.state FROM org_fueling_review_entries r WHERE r.org_id=f.org_id AND r.fueling_id=f.id ORDER BY r.sequence DESC LIMIT 1),'')::text AS review_entry_state
FROM org_fuelings f WHERE f.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND f.id=CAST(sqlc.arg(id)::text AS uuid);
