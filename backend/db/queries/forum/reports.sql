-- Forum reports and abuse triage queries (FOR-18).
-- Reconciles B-BR-FOR-08, B-BR-FOR-10, B-BR-FOR-11, BUC-FOR-06.

-- name: InsertForumReport :one
INSERT INTO forum_reports (
    id,
    account_id,
    target_type,
    target_id,
    reason,
    detail,
    status,
    moderation_case_id,
    created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (account_id, target_type, target_id) DO NOTHING
RETURNING id, account_id, target_type, target_id, reason, detail, status, moderation_case_id, created_at;

-- name: GetForumReportByAccountAndTarget :one
SELECT id, account_id, target_type, target_id, reason, detail, status, moderation_case_id, created_at
FROM forum_reports
WHERE account_id = $1 AND target_type = $2 AND target_id = $3;

-- name: CountReportsByAccountSince :one
SELECT count(*)
FROM forum_reports
WHERE account_id = $1 AND created_at >= $2;

-- name: UpdateForumReportCase :exec
UPDATE forum_reports
SET moderation_case_id = $2
WHERE id = $1;
