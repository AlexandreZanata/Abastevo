-- EMP-SUBS-004/006: keep a verified advance settlement in the durable inbox.
ALTER TABLE payment_inbox
    ADD COLUMN deferred_until TIMESTAMPTZ,
    ADD CONSTRAINT payment_inbox_deferral_pending CHECK (
        deferred_until IS NULL OR (isfinite(deferred_until) AND state='PENDING' AND processed_at IS NULL)
    );
