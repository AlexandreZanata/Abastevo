-- EMP-STN-002: supersession and withdrawal are row updates of the state
-- column, which station_announced_prices_guard already restricts. The history
-- trigger of 000260 also listed UPDATE, so every announcement and every
-- withdrawal was refused. History stays protected against deletion.
DROP TRIGGER station_announced_prices_immutable ON station_announced_prices;
CREATE TRIGGER station_announced_prices_immutable BEFORE DELETE OR TRUNCATE ON station_announced_prices
    FOR EACH STATEMENT EXECUTE FUNCTION station_announced_prices_keep_history();
