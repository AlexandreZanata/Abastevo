-- Owned by quality; private account scope is mandatory in every owner read.
-- name: GetReport :one
SELECT * FROM quality_reports WHERE id= @id AND account_id= @account_id;

-- name: LockReport :one
SELECT * FROM quality_reports WHERE id= @id AND account_id= @account_id FOR UPDATE;

-- name: LockQualityReportReview :one
SELECT * FROM quality_reports WHERE id= @id FOR UPDATE;

-- name: GetReportCommand :one
SELECT * FROM quality_report_commands WHERE account_id= @account_id AND command_id= @command_id;

-- name: InsertReport :exec
INSERT INTO quality_reports(id,account_id,kind,station_id,fuel,occurred_on,symptom,body,official_revision_id,sample_ref,revision,status,created_at,updated_at)
VALUES(@id,@account_id,@kind,@station_id,@fuel,@occurred_on,@symptom,@body,@official_revision_id,@sample_ref,1,'PENDING_REVIEW',@at,@at);

-- name: UpdateReport :exec
UPDATE quality_reports SET body= @body,occurred_on= @occurred_on,symptom= @symptom,revision= @revision,status= @status,review_reason='',updated_at= @at
WHERE id= @id AND account_id= @account_id;

-- name: InsertReportCommand :exec
INSERT INTO quality_report_commands(account_id,command_id,payload_hash,report_id,revision,status,created_at)
VALUES(@account_id,@command_id,@payload_hash,@report_id,@revision,@status,@at);

-- name: InsertReportAudit :exec
INSERT INTO quality_report_audit(report_id,revision,action,policy_version,occurred_at)
VALUES(@report_id,@revision,@action,@policy_version,@at);

-- name: ListOwnerReports :many
SELECT * FROM quality_reports WHERE account_id= @account_id AND id> @after_id ORDER BY id LIMIT @page_limit;

-- Lock the current pointer and target rows until the report commits. Corrections
-- cannot race publication into a command acknowledged against a stale lease.
-- name: LockReportSampleTarget :many
SELECT a.id FROM quality_assays a
JOIN quality_links l ON l.revision_id=a.revision_id AND l.record_key='pmqc-assay-v1:'||a.sample_ref AND l.active AND l.station_id= @station_id
JOIN quality_current_revisions c ON c.revision_id=a.revision_id AND c.source='pmqc' AND c.stream='pmqc-assay-v1'
JOIN quality_revisions r ON r.id=c.revision_id AND r.status='published'
WHERE a.revision_id= @revision_id AND a.sample_ref= @sample_ref AND a.fuel= @fuel
FOR SHARE OF c,r,a,l;

-- name: EraseQualityReports :execrows
DELETE FROM quality_reports r WHERE r.account_id= @account_id AND r.id IN (SELECT p.id FROM quality_reports p WHERE p.account_id= @account_id ORDER BY p.id LIMIT 100);

-- name: EraseQualityReportCommands :execrows
DELETE FROM quality_report_commands r WHERE r.account_id= @account_id AND r.command_id IN (SELECT p.command_id FROM quality_report_commands p WHERE p.account_id= @account_id ORDER BY p.command_id LIMIT 100);

-- name: EnsureReportReviewTarget :one
INSERT INTO quality_report_review_targets(id,report_id,report_revision,binding_state)
VALUES(@id,@report_id,@report_revision,'BOUND')
ON CONFLICT(report_id,report_revision) DO UPDATE SET report_id=EXCLUDED.report_id RETURNING id;

-- name: GetReportReviewTarget :one
SELECT * FROM quality_report_review_targets WHERE id= @id;

-- name: ApplyReportReview :exec
UPDATE quality_reports SET revision= @revision,status= @status,review_reason= @reason,
 body=CASE WHEN @clear_text::boolean THEN '' ELSE body END,updated_at= @at WHERE id= @id;

-- name: InsertReportAppeal :exec
INSERT INTO quality_report_appeals(report_id,revision,justification,created_at) VALUES(@report_id,@revision,@justification,@at);

-- name: ClearReportAppeals :exec
DELETE FROM quality_report_appeals WHERE report_id= @report_id;

-- name: GetReportAppeal :one
SELECT justification FROM quality_report_appeals WHERE report_id= @report_id AND revision= @revision;

-- name: InsertReportReviewDecision :exec
INSERT INTO quality_report_review_decisions(action_id,target_id,report_revision,action,reason_code,occurred_at)
VALUES(@action_id,@target_id,@report_revision,@action,@reason_code,@at);

-- name: GetReportReviewDecision :one
SELECT * FROM quality_report_review_decisions WHERE action_id= @action_id AND target_id= @target_id;

-- name: ListReportGroup :many
SELECT r.account_id,r.status,r.id,
 (r.official_revision_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM quality_revisions v WHERE v.id=r.official_revision_id AND v.status='published'
 ))::boolean AS superseded
FROM quality_reports r WHERE r.station_id= @station_id AND r.fuel= @fuel AND r.occurred_on= @day
AND r.id> @after_id ORDER BY r.id LIMIT 101;

-- Restricted operator read, never used by the public/owner handlers.
-- name: GetReviewReport :one
SELECT * FROM quality_reports WHERE id= @id;

-- name: HasReportTriage :one
SELECT EXISTS(SELECT 1 FROM quality_report_review_decisions WHERE target_id= @target_id AND action='REVIEW');
