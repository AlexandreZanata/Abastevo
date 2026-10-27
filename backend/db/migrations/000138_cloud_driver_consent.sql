-- DRV-15 registers four explicit driver cloud datasets and extends consent.
ALTER TABLE cloud_state DROP CONSTRAINT IF EXISTS cloud_state_consent_mask_check;
ALTER TABLE cloud_state ADD CONSTRAINT cloud_state_consent_mask_check CHECK (consent_mask BETWEEN 0 AND 127);

ALTER TABLE cloud_records DROP CONSTRAINT IF EXISTS cloud_records_dataset_check;
ALTER TABLE cloud_records ADD CONSTRAINT cloud_records_dataset_check CHECK(
 dataset IN ('vehicles','favorites','preferences','driver_earnings','driver_distance','driver_costs','driver_maintenance'));

ALTER TABLE cloud_changes DROP CONSTRAINT IF EXISTS cloud_changes_dataset_check;
ALTER TABLE cloud_changes ADD CONSTRAINT cloud_changes_dataset_check CHECK(
 dataset IN ('vehicles','favorites','preferences','driver_earnings','driver_distance','driver_costs','driver_maintenance'));
