ALTER TABLE subscription_purchases ADD COLUMN next_check_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp();
CREATE INDEX subscription_reconcile_due_idx ON subscription_purchases(environment,package_name,next_check_at,token_digest)
WHERE NOT retired AND token_cipher IS NOT NULL;
