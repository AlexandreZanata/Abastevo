-- FQ-11: document-free revision-bound review. No purchase verification.
ALTER TABLE quality_reports ADD COLUMN review_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE quality_report_audit DROP CONSTRAINT quality_report_audit_action_check;
ALTER TABLE quality_report_audit ADD CONSTRAINT quality_report_audit_action_check
 CHECK(action IN ('SUBMIT','EDIT','WITHDRAW','HIDE','REVIEW','APPEAL'));
CREATE TABLE quality_report_review_targets (
 id UUID PRIMARY KEY,
 report_id UUID NOT NULL REFERENCES quality_reports(id) ON DELETE CASCADE,
 report_revision INT,
 binding_state TEXT NOT NULL CHECK(binding_state IN ('BOUND','LEGACY_UNBOUND')),
 CHECK((binding_state='BOUND' AND report_revision IS NOT NULL AND report_revision>0) OR
       (binding_state='LEGACY_UNBOUND' AND report_revision IS NULL)),
 UNIQUE(report_id,report_revision)
);
-- Existing cases did not freeze a revision; never guess their historical target.
INSERT INTO quality_report_review_targets(id,report_id,binding_state)
 SELECT DISTINCT r.id,r.id,'LEGACY_UNBOUND' FROM quality_reports r
 JOIN moderation_cases c ON c.target_type='QUALITY_REPORT' AND c.target_id=r.id::text;
CREATE TABLE quality_report_appeals (
 report_id UUID NOT NULL REFERENCES quality_reports(id) ON DELETE CASCADE,
 revision INT NOT NULL CHECK(revision>0),
 justification TEXT NOT NULL CHECK(length(justification) BETWEEN 1 AND 1000),
 created_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(report_id,revision)
);
-- Minimal unlinked decisions survive account erasure; no owner/body/justification.
CREATE TABLE quality_report_review_decisions (
 action_id UUID PRIMARY KEY,
 target_id UUID NOT NULL,
 report_revision INT CHECK(report_revision>0),
 action TEXT NOT NULL,
 reason_code TEXT NOT NULL CHECK(reason_code IN ('TRIAGE','CLAIM_RECORDED','DUPLICATE','INSUFFICIENT_EVIDENCE','UNSUPPORTED_LINK','ABUSE','PRIVACY','SUPERSEDED')),
 occurred_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX quality_reports_group_idx ON quality_reports(station_id,fuel,occurred_on,id);
