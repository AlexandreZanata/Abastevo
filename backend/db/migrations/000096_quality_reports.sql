-- FQ-09: private document-free claims and minimal command receipts.
-- No receipt/protocol/media fields or purchase-verification states (future D).
CREATE TABLE quality_reports (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id),
 kind TEXT NOT NULL CHECK(kind IN ('EXPERIENCE','LINK_REVIEW')),
 station_id UUID NOT NULL REFERENCES directory_stations(id),
 fuel TEXT NOT NULL CHECK(fuel IN ('ETHANOL','GASOLINE_REGULAR','GASOLINE_ADDITIVED','GASOLINE_PREMIUM_GRADE','DIESEL_S500','DIESEL_S10','CNG','LPG_P13')),
 occurred_on DATE NOT NULL,
 symptom TEXT NOT NULL CHECK(symptom IN ('STARTING','PERFORMANCE','CONSUMPTION','OTHER')),
 body TEXT NOT NULL CHECK(length(body)<=1000),
 official_revision_id UUID REFERENCES quality_revisions(id),
 sample_ref TEXT NOT NULL DEFAULT '',
 revision INT NOT NULL CHECK(revision>0),
 status TEXT NOT NULL CHECK(status IN ('PENDING_REVIEW','WITHDRAWN','HIDDEN','REVIEWED')),
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 CHECK((official_revision_id IS NULL AND sample_ref='') OR (official_revision_id IS NOT NULL AND length(sample_ref) BETWEEN 1 AND 128)),
 CHECK(kind<>'LINK_REVIEW' OR official_revision_id IS NOT NULL)
);
CREATE INDEX quality_reports_owner_idx ON quality_reports(account_id,id);
CREATE INDEX quality_reports_station_fuel_idx ON quality_reports(station_id,fuel,status);
CREATE TABLE quality_report_commands (
 account_id UUID NOT NULL REFERENCES accounts(id),
 command_id UUID NOT NULL,
 payload_hash TEXT NOT NULL CHECK(payload_hash ~ '^[0-9a-f]{64}$'),
 report_id UUID NOT NULL,
 revision INT NOT NULL CHECK(revision>0),
 status TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(account_id,command_id)
);
CREATE TABLE quality_report_audit (
 report_id UUID NOT NULL,
 revision INT NOT NULL,
 action TEXT NOT NULL CHECK(action IN ('SUBMIT','EDIT','WITHDRAW','HIDE','REVIEW')),
 policy_version TEXT NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(report_id,revision)
);
ALTER TABLE moderation_cases DROP CONSTRAINT moderation_cases_target_check;
ALTER TABLE moderation_cases ADD CONSTRAINT moderation_cases_target_check CHECK(target_type IN
 ('OBSERVATION','DISPUTE','CONTRIBUTOR','EVIDENCE','COMMENT','CONTRIBUTION','STATION_CLAIM','QUALITY_REPORT'));
