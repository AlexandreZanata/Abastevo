-- P23-CITY-FEED: rebuildable indexes over the existing current projection.
-- Canonical directory municipality/state index already scopes the city join.
CREATE INDEX community_feed_recent_idx
 ON community_current_prices (fuel_product, unit, anchor_received_at DESC, station_id)
 WHERE availability = 'AVAILABLE' AND condition_kind = 'STANDARD' AND qualifier_key = 'STANDARD';
CREATE INDEX community_feed_cheapest_idx
 ON community_current_prices (fuel_product, unit, amount_milli_brl, station_id)
 WHERE availability = 'AVAILABLE' AND condition_kind = 'STANDARD' AND qualifier_key = 'STANDARD';
