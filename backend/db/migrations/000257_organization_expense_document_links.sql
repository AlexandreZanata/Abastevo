-- EMP-EXP-004. EXPENSE document links (B-BR-EMP-520, EDR-338). An expense
-- receipt is an EMP-DOC-003 link of target kind EXPENSE carried by its own
-- typed same-organization reference to the stored expense, exactly like the
-- 000245 FUELING kind. Additive only: one nullable column, one composite
-- foreign key and the widened kind and binding checks; existing links keep
-- their bindings. No function, trigger, usage source, policy or row changes;
-- org_expenses stays insert-only and gains no owner of its own here.
ALTER TABLE org_document_links ADD COLUMN expense_id UUID;
ALTER TABLE org_document_links ADD CONSTRAINT org_document_links_expense_fkey FOREIGN KEY(org_id,expense_id) REFERENCES org_expenses(org_id,id),
 DROP CONSTRAINT org_document_links_target_kind_check,
 ADD CONSTRAINT org_document_links_target_kind_check CHECK(target_kind IN('ASSET','EMPLOYEE','METER_READING','INCIDENT','FUELING','EXPENSE')),
 DROP CONSTRAINT org_document_links_binding,
 ADD CONSTRAINT org_document_links_binding CHECK(CASE target_kind
  WHEN 'ASSET' THEN asset_id IS NOT NULL AND asset_id=target_id AND num_nonnulls(employee_id,reading_id,held_reading_id,incident_report_id,fueling_id,expense_id)=0
  WHEN 'EMPLOYEE' THEN employee_id IS NOT NULL AND employee_id=target_id AND num_nonnulls(asset_id,reading_id,held_reading_id,incident_report_id,fueling_id,expense_id)=0
  WHEN 'METER_READING' THEN num_nonnulls(asset_id,employee_id,incident_report_id,fueling_id,expense_id)=0 AND num_nonnulls(reading_id,held_reading_id)=1
   AND COALESCE(reading_id,held_reading_id)=target_id
  WHEN 'INCIDENT' THEN incident_report_id IS NOT NULL AND incident_report_id=target_id AND num_nonnulls(asset_id,employee_id,reading_id,held_reading_id,fueling_id,expense_id)=0
  WHEN 'FUELING' THEN fueling_id IS NOT NULL AND fueling_id=target_id AND num_nonnulls(asset_id,employee_id,reading_id,held_reading_id,incident_report_id,expense_id)=0
  WHEN 'EXPENSE' THEN expense_id IS NOT NULL AND expense_id=target_id AND num_nonnulls(asset_id,employee_id,reading_id,held_reading_id,incident_report_id,fueling_id)=0
  ELSE false END);
