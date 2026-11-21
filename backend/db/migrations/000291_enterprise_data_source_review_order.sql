-- EMP-DATA-001: rights revisions and the Data transactions that use them
-- share one order. Ordinary readers use a shared lock from catalog.sql.
CREATE FUNCTION enterprise_data_order_source_review() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended('enterprise-data-source-rights', 0));
    RETURN NULL;
END $$;

CREATE TRIGGER enterprise_data_source_review_order BEFORE INSERT ON enterprise_data_source_reviews
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_data_order_source_review();
