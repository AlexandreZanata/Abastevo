-- EMP-DATA-001 / EMP-GOV-016: readers take the highest revision of a source
-- digest as its current rights decision. A skipped or out-of-order revision
-- would hide a later decision, so each hand-recorded decision is the next one.
-- The statement trigger of 000291 already serializes these inserts.
CREATE FUNCTION enterprise_data_next_source_review() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
    current_revision BIGINT;
BEGIN
    SELECT max(r.revision) INTO current_revision FROM public.enterprise_data_source_reviews r
    WHERE (r.source_code, r.source_digest) = (NEW.source_code, NEW.source_digest);
    IF NEW.revision <> COALESCE(current_revision, 0) + 1 THEN
        RAISE EXCEPTION USING ERRCODE = '23514',
            MESSAGE = 'enterprise Data source review revisions must be consecutive';
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER enterprise_data_source_review_sequence BEFORE INSERT ON enterprise_data_source_reviews
    FOR EACH ROW EXECUTE FUNCTION enterprise_data_next_source_review();
