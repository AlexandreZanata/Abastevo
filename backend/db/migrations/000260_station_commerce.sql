-- EMP-STN-002/004 (BR-STN-001/002, EDR-022): the station's own announced
-- prices and labelled promotions. A separate attributed commercial source
-- with product, unit, price, author, conditions and explicit validity.
-- Supersession and withdrawal are recorded state transitions; expiry
-- applies at read time. Announced or promoted values never enter official
-- or community consensus, ranking, moderation or trust (VAL-33), and no
-- official, community or regulatory row is reachable from here.
CREATE TABLE station_announced_prices(
 id UUID PRIMARY KEY CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 station_id UUID NOT NULL REFERENCES directory_stations(id) ON DELETE RESTRICT,
 fuel_product TEXT NOT NULL,
 unit TEXT NOT NULL,
 price_milli_brl BIGINT NOT NULL CHECK(price_milli_brl BETWEEN 1 AND 1000000),
 conditions TEXT NOT NULL DEFAULT '' CHECK(length(btrim(conditions))<=120),
 valid_from TIMESTAMPTZ NOT NULL CHECK(isfinite(valid_from)),
 valid_until TIMESTAMPTZ NOT NULL CHECK(isfinite(valid_until) AND valid_until>valid_from),
 author_account_id UUID NOT NULL,
 state TEXT NOT NULL CHECK(state IN('ACTIVE','WITHDRAWN','SUPERSEDED')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 CONSTRAINT station_announced_prices_product_check CHECK(fuel_product IN
   ('ETHANOL','GASOLINE_REGULAR','GASOLINE_ADDITIVED','DIESEL_S500',
    'DIESEL_S10','CNG','LPG_P13')),
 CONSTRAINT station_announced_prices_unit_check CHECK(unit IN('L','M3','KG_13'))
);
CREATE INDEX station_announced_prices_station_idx ON station_announced_prices(station_id,created_at DESC);
-- One active announcement per station, product and unit at any time; a new
-- announcement of the same key must supersede the stored one first.
CREATE UNIQUE INDEX station_announced_prices_one_active
 ON station_announced_prices(station_id,fuel_product,unit) WHERE state='ACTIVE';

-- Announced-price history is insert-only: corrections supersede or withdraw.
CREATE FUNCTION station_announced_prices_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='announced price history is insert-only';
END $$;
CREATE TRIGGER station_announced_prices_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON station_announced_prices FOR EACH STATEMENT EXECUTE FUNCTION station_announced_prices_keep_history();
-- Supersession and withdrawal only move the stored state; every other field
-- stays exactly as announced.
CREATE FUNCTION station_announced_prices_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.station_id IS DISTINCT FROM OLD.station_id
   OR NEW.fuel_product IS DISTINCT FROM OLD.fuel_product OR NEW.unit IS DISTINCT FROM OLD.unit
   OR NEW.price_milli_brl IS DISTINCT FROM OLD.price_milli_brl OR NEW.conditions IS DISTINCT FROM OLD.conditions
   OR NEW.valid_from IS DISTINCT FROM OLD.valid_from OR NEW.valid_until IS DISTINCT FROM OLD.valid_until
   OR NEW.author_account_id IS DISTINCT FROM OLD.author_account_id
   OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='announced price fields are immutable';
  END IF;
  IF OLD.state='ACTIVE' AND NEW.state NOT IN('WITHDRAWN','SUPERSEDED') THEN
   RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='announced price leaves ACTIVE only by withdrawal or supersession';
  END IF;
  IF OLD.state IN('WITHDRAWN','SUPERSEDED') AND NEW.state<>OLD.state THEN
   RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='announced price transition is terminal';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER station_announced_prices_guard BEFORE UPDATE ON station_announced_prices FOR EACH ROW EXECUTE FUNCTION station_announced_prices_guard();

-- EMP-STN-004: promotions are labelled, time-bound and attributed. The
-- stored window is authoritative; SCHEDULED/ACTIVE/ENDED derive at read
-- time, WITHDRAWN is the only persisted exit.
CREATE TABLE station_promotions(
 id UUID PRIMARY KEY CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 station_id UUID NOT NULL REFERENCES directory_stations(id) ON DELETE RESTRICT,
 label TEXT NOT NULL CHECK(length(btrim(label)) BETWEEN 1 AND 60),
 description TEXT NOT NULL CHECK(length(btrim(description)) BETWEEN 1 AND 280),
 fuel_product TEXT,
 valid_from TIMESTAMPTZ NOT NULL CHECK(isfinite(valid_from)),
 valid_until TIMESTAMPTZ NOT NULL CHECK(isfinite(valid_until) AND valid_until>valid_from),
 author_account_id UUID NOT NULL,
 state TEXT NOT NULL CHECK(state IN('PUBLISHED','WITHDRAWN')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 CONSTRAINT station_promotions_product_check CHECK(fuel_product IS NULL OR fuel_product IN
   ('ETHANOL','GASOLINE_REGULAR','GASOLINE_ADDITIVED','DIESEL_S500',
    'DIESEL_S10','CNG','LPG_P13'))
);
CREATE INDEX station_promotions_station_idx ON station_promotions(station_id,created_at DESC);
-- A station keeps one live promotion per label; reusing a label first
-- withdraws the stored promotion.
CREATE UNIQUE INDEX station_promotions_one_live
 ON station_promotions(station_id,label) WHERE state='PUBLISHED';

CREATE FUNCTION station_promotions_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='promotion history is insert-only';
END $$;
CREATE TRIGGER station_promotions_immutable BEFORE DELETE OR TRUNCATE ON station_promotions FOR EACH STATEMENT EXECUTE FUNCTION station_promotions_keep_history();
CREATE FUNCTION station_promotions_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.station_id IS DISTINCT FROM OLD.station_id
   OR NEW.label IS DISTINCT FROM OLD.label OR NEW.description IS DISTINCT FROM OLD.description
   OR NEW.fuel_product IS DISTINCT FROM OLD.fuel_product
   OR NEW.valid_from IS DISTINCT FROM OLD.valid_from OR NEW.valid_until IS DISTINCT FROM OLD.valid_until
   OR NEW.author_account_id IS DISTINCT FROM OLD.author_account_id
   OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='promotion fields are immutable';
  END IF;
  IF OLD.state='WITHDRAWN' AND NEW.state<>'WITHDRAWN' THEN
   RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='promotion withdrawal is terminal';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER station_promotions_guard BEFORE UPDATE ON station_promotions FOR EACH ROW EXECUTE FUNCTION station_promotions_guard();
