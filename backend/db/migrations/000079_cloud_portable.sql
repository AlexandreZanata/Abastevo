ALTER TABLE cloud_records DROP CONSTRAINT cloud_records_dataset_check;
ALTER TABLE cloud_records ADD CONSTRAINT cloud_records_dataset_check CHECK(dataset IN ('vehicles','favorites','preferences'));
