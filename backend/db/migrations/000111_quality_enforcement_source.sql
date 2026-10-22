-- Native workbook facts deliberately do not inherit legacy occurrence/as-of
-- semantics. Existing enforcement rows, publication keys and pointers survive.
CREATE TABLE quality_enforcement_source_revisions (
 revision_id UUID PRIMARY KEY REFERENCES quality_revisions(id) ON DELETE CASCADE,
 source_profile TEXT NOT NULL CHECK (source_profile = 'anp-enforcement-xlsx-2019-v1'),
 source_file TEXT NOT NULL CHECK (octet_length(source_file) BETWEEN 1 AND 255 AND source_file !~ '[/\\[:cntrl:]]' AND source_file NOT IN ('.','..')),
 date_system TEXT NOT NULL CHECK (date_system IN ('EXCEL_1900','EXCEL_1904')),
 input_count INTEGER NOT NULL CHECK (input_count BETWEEN 0 AND 500000),
 accepted_count INTEGER NOT NULL CHECK (accepted_count BETWEEN 0 AND 500000),
 duplicate_count INTEGER NOT NULL CHECK (duplicate_count BETWEEN 0 AND 500000),
 first_document_day DATE,
 last_document_day DATE,
 CHECK (input_count = accepted_count + duplicate_count),
 CHECK ((input_count = 0 AND first_document_day IS NULL AND last_document_day IS NULL) OR
 (input_count > 0 AND first_document_day IS NOT NULL AND last_document_day IS NOT NULL AND
 first_document_day BETWEEN DATE '0001-01-01' AND DATE '9999-12-31' AND
 last_document_day BETWEEN first_document_day AND DATE '9999-12-31')),
 UNIQUE(revision_id, first_document_day, last_document_day)
);
CREATE TABLE quality_enforcement_source_records (
 id UUID PRIMARY KEY,
 revision_id UUID NOT NULL REFERENCES quality_enforcement_source_revisions(revision_id) ON DELETE CASCADE,
 native_key TEXT NOT NULL,
 subject_cnpj TEXT,
 subject_kind TEXT NOT NULL,
 uf TEXT NOT NULL CHECK (uf ~ '^[A-Z]{2}$'),
 municipality TEXT NOT NULL CHECK (octet_length(municipality) BETWEEN 1 AND 256),
 segment TEXT NOT NULL CHECK (octet_length(segment) <= 256),
 document_day DATE NOT NULL CHECK (document_day BETWEEN DATE '0001-01-01' AND DATE '9999-12-31'),
 document_number TEXT NOT NULL CHECK (octet_length(document_number) BETWEEN 1 AND 128),
 procedure TEXT NOT NULL CHECK (octet_length(procedure) <= 65536),
 result TEXT NOT NULL CHECK (octet_length(result) <= 65536),
 evidence_ref TEXT NOT NULL CHECK (octet_length(evidence_ref) BETWEEN 1 AND 512),
 fingerprint TEXT NOT NULL CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
 first_document_day DATE NOT NULL,
 last_document_day DATE NOT NULL,
 FOREIGN KEY(revision_id,first_document_day,last_document_day) REFERENCES quality_enforcement_source_revisions(revision_id,first_document_day,last_document_day),
 CHECK (document_day BETWEEN first_document_day AND last_document_day),
 CHECK ((subject_kind = 'company-shaped' AND subject_cnpj IS NOT NULL AND subject_cnpj ~ '^[0-9]{14}$') OR (subject_kind IN ('personal-shaped','other') AND subject_cnpj IS NULL)),
 CHECK (position(chr(31) IN coalesce(subject_cnpj,'') || uf || municipality || segment || document_number || procedure || result) = 0),
 CHECK (native_key = 'enforcement-source:' || fingerprint),
 CHECK (fingerprint = encode(sha256(convert_to(concat_ws(chr(31),coalesce(subject_cnpj,''),uf,municipality,segment,to_char(document_day,'YYYY-MM-DD'),document_number,procedure,result),'UTF8')),'hex')),
 UNIQUE(revision_id,native_key),
 UNIQUE(revision_id,evidence_ref)
);
