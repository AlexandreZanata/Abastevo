-- 000130: allow capability-free orders and grants for the enterprise family.
-- Premium offers always carry capabilities (catalog and settlement binding
-- enforce non-empty capability sets), while an approved enterprise offer
-- carries none by definition: its paid period grants no app capability and
-- unlocks no team, station, organization or cross-account privilege.

ALTER TABLE payment_orders
    DROP CONSTRAINT IF EXISTS payment_orders_capabilities_check;

ALTER TABLE payment_orders
    ADD COLUMN family TEXT NOT NULL DEFAULT '';

ALTER TABLE payment_grants
    DROP CONSTRAINT IF EXISTS payment_grants_capabilities_check;
