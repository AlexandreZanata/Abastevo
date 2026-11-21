-- name: FuelingReviewPeople :one
SELECT o.owner_account_id::text AS author_account,
       f.author_membership_id::text AS author_membership,
       COALESCE(org_sync_review_employee_canonical(f.org_id,f.author_employee_id)::text,'')::text AS author_employee,
       COALESCE(org_sync_review_employee_canonical(f.org_id,f.responsible_employee_id)::text,'')::text AS responsible_employee,
       reviewer.employee_id::text AS reviewer_employee
FROM org_fuelings f JOIN org_sync_operations o ON (o.org_id,o.operation_id)=(f.org_id,f.operation_id)
JOIN org_memberships reviewer ON reviewer.org_id=f.org_id
  AND reviewer.id=sqlc.arg('membership_id')::text::uuid
  AND reviewer.account_id=sqlc.arg('account_id')::text::uuid AND reviewer.state='ACTIVE'
JOIN org_employee_identities person ON (person.org_id,person.id)=(reviewer.org_id,reviewer.employee_id)
  AND person.state='ACTIVE' AND person.merged_into IS NULL
WHERE (f.org_id,f.id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('fueling_id')::text::uuid);

-- name: ListFuelingReviewEntries :many
SELECT sequence,kind,revision,state,reason
FROM org_fueling_review_entries
WHERE (org_id,fueling_id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('fueling_id')::text::uuid)
ORDER BY sequence;

-- name: ReadFuelingDecision :one
SELECT sequence,state FROM org_fueling_review_entries
WHERE (org_id,fueling_id,operation_id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('fueling_id')::text::uuid,sqlc.arg('operation_id')::text::uuid);

-- name: ListFuelingReviewWarnings :many
SELECT code FROM org_fueling_warnings
WHERE (org_id,fueling_id,revision)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('fueling_id')::text::uuid,1)
ORDER BY code;

-- name: FuelingReviewDuplicatePending :one
SELECT org_fueling_duplicate_pending(sqlc.arg('org_id')::text::uuid,sqlc.arg('fueling_id')::text::uuid)::boolean AS pending;

-- name: InsertFuelingReviewDecision :exec
INSERT INTO org_fueling_review_entries(
    org_id,fueling_id,sequence,kind,revision,state,reason,actor_membership_id,operation_id,warning_notes,fuel_entered_tank
) VALUES(
    sqlc.arg('org_id')::text::uuid,sqlc.arg('fueling_id')::text::uuid,sqlc.arg('review_sequence')::bigint,sqlc.arg('kind')::text,1,
    sqlc.arg('state')::text,sqlc.arg('reason')::text,sqlc.arg('member')::text::uuid,sqlc.arg('operation_id')::text::uuid,
    sqlc.arg('warning_notes')::jsonb,sqlc.narg('fuel_entered_tank')::boolean
);
