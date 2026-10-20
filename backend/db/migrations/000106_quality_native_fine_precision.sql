-- Native SICOM observations preserve due dates and month-only extraction clocks.
-- Legacy snapshots keep their exact-day values; no decision/as-of day is backfilled.
ALTER TABLE quality_sanction_events
 ALTER COLUMN as_of DROP NOT NULL,
 ADD COLUMN source_contract TEXT NOT NULL DEFAULT 'sanction-prepared-8-v1',
 ADD COLUMN extraction_month TEXT,
 ADD COLUMN report_year INTEGER,
 ADD COLUMN duf_ref TEXT,
 ADD COLUMN department TEXT,
 ADD COLUMN due_on DATE,
 ADD COLUMN amount_text TEXT,
 ADD COLUMN paid_text TEXT,
 ADD COLUMN paid_minor BIGINT,
 DROP CONSTRAINT quality_sanction_events_status_check,
 DROP CONSTRAINT quality_sanction_events_proceeding_ref_check;

ALTER TABLE quality_sanction_events
 ADD CONSTRAINT quality_sanction_status_bounds CHECK(octet_length(status) BETWEEN 1 AND 65536),
 ADD CONSTRAINT quality_sanction_proceeding_bounds CHECK(octet_length(proceeding_ref) BETWEEN 1 AND 65536),
 ADD CONSTRAINT quality_sanction_source_shape CHECK(
  (source_contract = 'sanction-prepared-8-v1'
   AND as_of IS NOT NULL
   AND extraction_month IS NULL AND report_year IS NULL
   AND duf_ref IS NULL AND department IS NULL AND due_on IS NULL
   AND amount_text IS NULL AND paid_text IS NULL AND paid_minor IS NULL)
  OR
  (source_contract = 'anp-sicom-cp850-9-v1'
   AND native_key ~ '^observation:[0-9a-f]{64}$'
   AND party_cnpj ~ '^[0-9A-Z]{12}[0-9]{2}$'
   AND as_of IS NULL AND decided_on IS NULL
   AND report_year IS NOT NULL AND report_year BETWEEN 2016 AND 9999
   AND extraction_month IS NOT NULL
   AND extraction_month ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'
   AND substring(extraction_month FROM 1 FOR 4)::integer >= report_year
   AND duf_ref IS NOT NULL AND octet_length(duf_ref) BETWEEN 1 AND 65536
   AND department IS NOT NULL AND octet_length(department) BETWEEN 1 AND 65536
   AND due_on IS NOT NULL
   AND due_on BETWEEN DATE '0001-01-01' AND DATE '9999-12-31'
   AND amount_minor IS NOT NULL AND amount_minor >= 0
   AND paid_minor IS NOT NULL AND paid_minor >= 0
   AND amount_text IS NOT NULL AND octet_length(amount_text) BETWEEN 1 AND 65536
   AND paid_text IS NOT NULL AND octet_length(paid_text) BETWEEN 1 AND 65536
   AND amount_text ~ '^(0|[1-9][0-9]*|[1-9][0-9]{0,2}(\.[0-9]{3})+),[0-9]{2}$'
   AND paid_text ~ '^(0|[1-9][0-9]*|[1-9][0-9]{0,2}(\.[0-9]{3})+),[0-9]{2}$'
   AND replace(replace(amount_text, '.', ''), ',', '')::numeric = amount_minor
   AND replace(replace(paid_text, '.', ''), ',', '')::numeric = paid_minor)
 );
