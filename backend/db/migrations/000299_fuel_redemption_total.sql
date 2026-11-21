-- EMP-AGREE-005 (BR-MONEY-005): quantity in milli-units times price in
-- milli-BRL per unit divides by 10 000 to centavos, rounded half-up. The
-- check of 000261 divided by 1 000, so every redemption stored ten times the
-- negotiated amount and the voucher limit was compared against it.
-- Redemption history is insert-only. A row written under the earlier rule is
-- kept as recorded, so the corrected rule is enforced on new rows only.
ALTER TABLE org_fuel_redemptions DROP CONSTRAINT org_fuel_redemptions_total_check;
ALTER TABLE org_fuel_redemptions ADD CONSTRAINT org_fuel_redemptions_total_check
    CHECK (total_centavos = (quantity_milli * price_milli_brl + 5000) / 10000) NOT VALID;
