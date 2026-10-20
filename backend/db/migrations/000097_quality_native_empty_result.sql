-- FQ-02B: the verified native PMQC edition includes empty result cells.
-- Preserve them as missing source evidence, never manufacture zero or discard
-- an otherwise identified assay. Existing rows and NOT NULL remain unchanged.
ALTER TABLE quality_assays DROP CONSTRAINT quality_assays_result_text_check;
ALTER TABLE quality_assays ADD CONSTRAINT quality_assays_result_text_check
 CHECK(length(result_text) <= 1024);
