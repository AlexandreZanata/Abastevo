-- name: ExpenseAuthorEmployee :one
-- EMP-EXP-002: the author's employee record, read in the intake transaction.
SELECT employee_id::text AS employee_id FROM org_memberships
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(membership_id)::text AS uuid);

-- name: ExpenseReferenceFacts :one
-- Admission facts of one direct expense, taken after the root and asset locks.
SELECT a.state AS asset_state,
 COALESCE((SELECT x.operation_id::text FROM org_expenses x WHERE x.org_id=a.org_id AND x.id=CAST(sqlc.arg(expense_id)::text AS uuid)),'')::text AS existing_operation
FROM org_assets a WHERE a.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND a.id=CAST(sqlc.arg(asset_id)::text AS uuid);

-- name: ExpenseReportingZones :one
-- BR-TIME-004: the organization zone and, when a unit is named, its zone.
SELECT o.time_zone AS organization_zone,
 COALESCE((SELECT u.time_zone FROM org_units u WHERE u.org_id=o.org_id AND u.id=CAST(sqlc.narg(unit_id)::text AS uuid)),'')::text AS unit_zone
FROM organizations o WHERE o.org_id=CAST(sqlc.arg(org_id)::text AS uuid);

-- name: ExpenseFuelingSource :one
-- EDR-287: the stored fueling a FUELING expense is projected from.
SELECT f.id::text AS id,f.operation_id::text AS operation_id,f.payload_digest,f.revision,f.asset_id::text AS asset_id,
 f.author_membership_id::text AS author_membership_id,f.author_employee_id::text AS author_employee_id,f.enrollment_id::text AS enrollment_id,
 f.occurred_at,f.occurred_submicro,f.entry_zone,f.device_offset_seconds,f.paid_centavos,f.payer,
 COALESCE(f.unit_id::text,'')::text AS unit_id,COALESCE(f.cost_centre_id::text,'')::text AS cost_centre_id,f.review_state
FROM org_fuelings f WHERE f.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND f.id=CAST(sqlc.arg(id)::text AS uuid);

-- name: PendingFuelingCosts :many
-- EDR-287 catch-up: stored fuelings of the organization without their expense.
SELECT f.id::text AS id,f.revision FROM org_fuelings f
WHERE f.org_id=CAST(sqlc.arg(org_id)::text AS uuid)
 AND NOT EXISTS(SELECT 1 FROM org_expenses x WHERE x.org_id=f.org_id AND x.fueling_id=f.id)
ORDER BY f.received_at,f.id;

-- name: InsertExpense :execrows
-- EDR-290: a lost race on any unique key inserts nothing; the caller re-reads.
INSERT INTO org_expenses(org_id,id,origin_kind,source_id,fueling_id,source_revision,operation_id,payload_digest,asset_id,
 author_membership_id,author_employee_id,enrollment_id,category,paid_centavos,company_centavos,payer,
 occurred_at,occurred_submicro,entry_zone,device_offset_seconds,reporting_day,reporting_zone,reporting_source,
 unit_id,cost_centre_id,review_state,revision)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),sqlc.arg(origin_kind)::text,CAST(sqlc.arg(source_id)::text AS uuid),
 CAST(sqlc.narg(fueling_id)::text AS uuid),sqlc.arg(source_revision)::bigint,CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(payload_digest)::text,
 CAST(sqlc.arg(asset_id)::text AS uuid),CAST(sqlc.arg(author_membership_id)::text AS uuid),CAST(sqlc.arg(author_employee_id)::text AS uuid),
 CAST(sqlc.arg(enrollment_id)::text AS uuid),sqlc.arg(category)::text,sqlc.arg(paid_centavos)::bigint,sqlc.arg(company_centavos)::bigint,sqlc.arg(payer)::text,
 sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint,
 sqlc.arg(reporting_day)::date,sqlc.arg(reporting_zone)::text,sqlc.arg(reporting_source)::text,
 CAST(sqlc.narg(unit_id)::text AS uuid),CAST(sqlc.narg(cost_centre_id)::text AS uuid),sqlc.arg(review_state)::text,sqlc.arg(revision)::bigint)
ON CONFLICT DO NOTHING;

-- name: GetExpense :one
SELECT x.id::text AS id,x.origin_kind,x.source_id::text AS source_id,COALESCE(x.fueling_id::text,'')::text AS fueling_id,x.source_revision,
 x.operation_id::text AS operation_id,x.payload_digest,x.asset_id::text AS asset_id,x.author_membership_id::text AS author_membership_id,
 x.author_employee_id::text AS author_employee_id,x.enrollment_id::text AS enrollment_id,x.category,x.paid_centavos,x.company_centavos,x.payer,
 x.occurred_at,x.occurred_submicro,x.entry_zone,x.device_offset_seconds,x.reporting_day::text AS reporting_day,x.reporting_zone,x.reporting_source,
 COALESCE(x.unit_id::text,'')::text AS unit_id,COALESCE(x.cost_centre_id::text,'')::text AS cost_centre_id,x.received_at,x.review_state,x.revision
FROM org_expenses x WHERE x.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND x.id=CAST(sqlc.arg(id)::text AS uuid);

-- name: ExpenseIDByOrigin :one
SELECT x.id::text AS id FROM org_expenses x
WHERE x.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND x.origin_kind=sqlc.arg(origin_kind)::text AND x.source_id=CAST(sqlc.arg(source_id)::text AS uuid);
