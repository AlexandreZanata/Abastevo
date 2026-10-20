
-- Restricted quality queue; no report body, owner or document references.
-- name: ListQualityReviewCases :many
SELECT * FROM moderation_cases WHERE target_type='QUALITY_REPORT'
AND status IN ('OPEN','IN_REVIEW') AND id> @after_id ORDER BY id LIMIT 101;
