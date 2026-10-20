-- Exact historical fine profiles; legacy/2025 facts remain unchanged.
CREATE FUNCTION quality_history_money_text(raw_text TEXT) RETURNS TEXT
LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE AS $money$
 WITH stripped AS (
  SELECT btrim(raw_text, E' \t\n\r\f' || U&'\000B\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000') AS value
 )
 SELECT CASE WHEN left(value,2)='R$' THEN
  btrim(substr(value,3), E' \t\n\r\f' || U&'\000B\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000')
 ELSE value END FROM stripped
$money$;
CREATE FUNCTION quality_history_money_minor(raw_text TEXT) RETURNS NUMERIC
LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE AS $money$
 SELECT CASE WHEN quality_history_money_text(raw_text) ~ '^(0|[1-9][0-9]*|[1-9][0-9]{0,2}(\.[0-9]{3})+),[0-9]{2}$'
 THEN replace(replace(quality_history_money_text(raw_text),'.',''),',','')::numeric
 ELSE NULL END
$money$;
CREATE FUNCTION quality_history_unknown_paid(raw_text TEXT) RETURNS BOOLEAN
LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE AS $money$
 SELECT btrim(raw_text, E' \t\n\r\f' || U&'\000B\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000') LIKE 'R$%'
  AND quality_history_money_text(raw_text)='-'
$money$;

-- Locators are canonical original logical records, never caller-selected paths.
CREATE FUNCTION quality_history_record(locator TEXT, source_file TEXT) RETURNS INTEGER
LANGUAGE SQL IMMUTABLE STRICT PARALLEL SAFE AS $record$
 SELECT CASE WHEN starts_with(locator,source_file || ':')
  AND substring(locator FROM length(source_file)+2) ~ '^[1-9][0-9]{0,5}$'
 THEN substring(locator FROM length(source_file)+2)::integer ELSE NULL END
$record$;

-- Separate revision context avoids inventing2025 report-year/month fields for
-- a grouped historical source or changing the immutable2025 context contract.
CREATE TABLE quality_historical_fine_revisions (
 revision_id UUID PRIMARY KEY REFERENCES quality_revisions(id) ON DELETE CASCADE,
 input_contract TEXT NOT NULL,
 source_period TEXT NOT NULL,
 source_file TEXT NOT NULL,
 header_record INTEGER NOT NULL,
 source_clock_precision TEXT NOT NULL,
 source_clock_value TEXT NOT NULL,
 source_record_count INTEGER NOT NULL CHECK(source_record_count BETWEEN 0 AND 500000),
 metadata_count INTEGER NOT NULL CHECK(metadata_count BETWEEN 0 AND source_record_count),
 UNIQUE(revision_id,input_contract),
 UNIQUE(revision_id,source_period),
 CONSTRAINT quality_history_profile CHECK(CASE source_period
  WHEN '2016-2019' THEN input_contract='anp-sicom-utf8bom-group-2016-2019-v1' AND source_file='multas-aplicadas-2016a2019.csv' AND header_record=5
  WHEN '2020' THEN input_contract='anp-sicom-utf8bom-2020-v1' AND source_file='multas-aplicadas-2020.csv' AND header_record=7
  WHEN '2021' THEN input_contract='anp-sicom-utf8bom-csv-named-xlsx-2021-v1' AND source_file='multas-2021-junho22.xlsx' AND header_record=7
  WHEN '2022' THEN input_contract='anp-sicom-utf8bom-2022-v1' AND source_file='multas-aplicadas-2022.csv' AND header_record=7
  WHEN '2023' THEN input_contract='anp-sicom-cp850-2023-v1' AND source_file='multas-aplicadas-2023.csv' AND header_record=6
  WHEN '2024' THEN input_contract='anp-sicom-utf8bom-2024-v1' AND source_file='multas-aplicadas-2024.csv' AND header_record=7
  ELSE FALSE END),
 CONSTRAINT quality_history_clock CHECK(CASE WHEN source_period='2016-2019' THEN
  source_clock_precision='DAY' AND CASE WHEN source_clock_value ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' THEN
   source_clock_value::date BETWEEN DATE '2020-01-01' AND DATE '9999-12-31'
  ELSE FALSE END
 ELSE source_clock_precision='MONTH' AND CASE WHEN source_clock_value ~ '^[0-9]{4}-(0[1-9]|1[0-2])$' AND source_period ~ '^[0-9]{4}$' THEN
  substring(source_clock_value FROM 1 FOR 4)::integer >= source_period::integer
 ELSE FALSE END END),
 CONSTRAINT quality_history_metadata_profile CHECK(source_period IN ('2020','2021') OR metadata_count=0)
);

ALTER TABLE quality_sanction_events
 ADD COLUMN history_revision_id UUID,
 ADD COLUMN reference_label TEXT,
 ADD COLUMN reference_ref TEXT,
 ADD COLUMN source_transit_on DATE,
 ADD COLUMN unlabelled_field TEXT,
 ADD CONSTRAINT quality_sanction_history_context FOREIGN KEY(history_revision_id,source_contract)
  REFERENCES quality_historical_fine_revisions(revision_id,input_contract) ON DELETE CASCADE,
 DROP CONSTRAINT quality_sanction_source_shape;
CREATE INDEX quality_sanction_history_context_idx
 ON quality_sanction_events(history_revision_id,source_contract) WHERE history_revision_id IS NOT NULL;

ALTER TABLE quality_sanction_events ADD CONSTRAINT quality_sanction_source_shape CHECK(
 (history_revision_id IS NULL AND reference_label IS NULL AND reference_ref IS NULL
  AND source_transit_on IS NULL AND unlabelled_field IS NULL AND (
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
 ))
 OR
 (history_revision_id IS NOT NULL AND history_revision_id=revision_id
  AND native_key ~ '^observation:[0-9a-f]{64}$' AND party_cnpj ~ '^[0-9A-Z]{12}[0-9]{2}$'
  AND as_of IS NULL AND decided_on IS NULL AND extraction_month IS NULL AND report_year IS NULL AND duf_ref IS NULL
  AND reference_label IS NOT NULL AND reference_ref IS NOT NULL AND octet_length(reference_ref) BETWEEN 1 AND 65536
  AND reference_label=CASE WHEN source_contract IN ('anp-sicom-utf8bom-group-2016-2019-v1','anp-sicom-utf8bom-2020-v1','anp-sicom-utf8bom-csv-named-xlsx-2021-v1') THEN 'Auto de Infração' ELSE 'Número do DUF' END
  AND department IS NOT NULL AND octet_length(department)<=65536
  AND unlabelled_field IS NOT NULL AND octet_length(unlabelled_field)<=65536
  AND (source_contract='anp-sicom-utf8bom-2024-v1' OR unlabelled_field='')
  AND (source_transit_on IS NULL OR (source_contract NOT IN ('anp-sicom-cp850-2023-v1','anp-sicom-utf8bom-2024-v1')
   AND source_transit_on BETWEEN DATE '0001-01-01' AND DATE '9999-12-31'))
  AND due_on IS NOT NULL AND due_on BETWEEN DATE '0001-01-01' AND DATE '9999-12-31'
  AND status !~ '[[:cntrl:]]' AND proceeding_ref !~ '[[:cntrl:]]' AND reference_ref !~ '[[:cntrl:]]'
  AND department !~ '[[:cntrl:]]' AND unlabelled_field !~ '[[:cntrl:]]'
  AND amount_text IS NOT NULL AND octet_length(amount_text) BETWEEN 1 AND 65536 AND amount_text !~ '[[:cntrl:]]'
  AND paid_text IS NOT NULL AND octet_length(paid_text) BETWEEN 1 AND 65536 AND paid_text !~ '[[:cntrl:]]'
  AND (status || proceeding_ref || reference_ref || department || unlabelled_field || amount_text || paid_text) !~ U&'[\2028\2029]'
  AND amount_minor IS NOT NULL AND amount_minor>=0
  AND quality_history_money_minor(amount_text) IS NOT NULL AND quality_history_money_minor(amount_text)=amount_minor
  AND ((paid_minor IS NOT NULL AND paid_minor>=0 AND quality_history_money_minor(paid_text) IS NOT NULL AND quality_history_money_minor(paid_text)=paid_minor)
   OR (paid_minor IS NULL AND source_contract='anp-sicom-cp850-2023-v1' AND quality_history_unknown_paid(paid_text))))
);

CREATE TABLE quality_historical_fine_notes (
 revision_id UUID NOT NULL,
 source_period TEXT NOT NULL,
 evidence_ref TEXT NOT NULL CHECK(octet_length(evidence_ref) BETWEEN 1 AND 512),
 kind TEXT NOT NULL,
 note_text TEXT NOT NULL,
 PRIMARY KEY(revision_id,evidence_ref),
 FOREIGN KEY(revision_id,source_period) REFERENCES quality_historical_fine_revisions(revision_id,source_period) ON DELETE CASCADE,
 CONSTRAINT quality_history_note_allowlist CHECK(source_period IN ('2020','2021') AND (
  (kind='BLANK_SEPARATOR' AND note_text='') OR
  (kind='PUBLIC_PAYMENT_NOTE' AND (
   note_text='(*) Para verificação do valor total pago dos processos com status parcelamento em andamento é preciso acessar o link: https://app.anp.gov.br/anp-gpc-consulta-web/public/emissao-gru/list.xhtml'
   OR (source_period='2020' AND note_text='(**) Parcelamentos feitos via SAPIENS são controlados pela Procuradoria Federal. Para obter os valores já pagos entrar em contato com a AGU.')
   OR (source_period='2021' AND note_text='(**) Parcelamentos SAPIENS são controlados pela Procuradoria Federal. Para obter o s valores já pagos entrar em contato com a AGU.')))))
);
