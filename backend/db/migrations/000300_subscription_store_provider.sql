-- STORE-01: a purchase records the store that issued it. Google Play and the
-- App Store share the app identity com.anpfuel.app, so environment and
-- package name alone cannot route a refresh to the right store. Existing rows
-- were all verified through Google Play.
ALTER TABLE subscription_purchases ADD COLUMN provider TEXT NOT NULL DEFAULT 'play'
    CHECK (provider IN ('play','app_store'));
DROP INDEX subscription_reconcile_due_idx;
CREATE INDEX subscription_reconcile_due_idx
    ON subscription_purchases(provider,environment,package_name,next_check_at,token_digest)
    WHERE NOT retired AND token_cipher IS NOT NULL;
