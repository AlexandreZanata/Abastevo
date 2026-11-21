-- EMP-GOV-016: Audit new operator decisions in the immutable review itself.
-- Existing reviews are preserved with their original evidence and no actor claim.
ALTER TABLE enterprise_data_source_reviews
    ADD COLUMN operation_id UUID UNIQUE,
    ADD COLUMN recorded_by NAME,
    ADD COLUMN snapshot_digest TEXT,
    ADD COLUMN official_revision UUID,
    ADD CONSTRAINT enterprise_data_rights_operation_proof CHECK (
        (operation_id IS NULL AND recorded_by IS NULL AND snapshot_digest IS NULL AND official_revision IS NULL) OR
        (operation_id IS NOT NULL AND operation_id <> '00000000-0000-0000-0000-000000000000'::uuid
        AND recorded_by IS NOT NULL AND (
            (status = 'APPROVED' AND attribution = 'Fonte: ANP'
             AND snapshot_digest IS NOT NULL AND snapshot_digest ~ '^sha256:[a-f0-9]{64}$'
             AND (source_code = 'S-ESP-02') = (official_revision IS NOT NULL)) OR
            (status = 'WITHDRAWN' AND license_ref = '' AND attribution = ''
             AND snapshot_digest IS NULL AND official_revision IS NULL)
        ))
    );

CREATE FUNCTION enterprise_data_record_review_actor() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF NEW.operation_id IS NULL AND NOT pg_catalog.has_table_privilege(
        session_user,'public.enterprise_data_source_reviews','INSERT') THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'Data rights review operation is required';
    END IF;
    IF NEW.operation_id IS NOT NULL THEN
        NEW.recorded_by := session_user;
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER enterprise_data_source_review_actor BEFORE INSERT ON enterprise_data_source_reviews
    FOR EACH ROW EXECUTE FUNCTION enterprise_data_record_review_actor();
