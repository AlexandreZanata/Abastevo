-- Revision-scoped source precision survives empty snapshots and corrections.
CREATE TABLE quality_native_fine_revisions (
 revision_id UUID PRIMARY KEY REFERENCES quality_revisions(id) ON DELETE CASCADE,
 report_year INTEGER NOT NULL CHECK(report_year BETWEEN 2016 AND 9999),
 extraction_month TEXT NOT NULL CHECK(extraction_month ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
 CONSTRAINT quality_native_fine_clock_order CHECK(substring(extraction_month FROM 1 FOR 4)::integer >= report_year)
);
