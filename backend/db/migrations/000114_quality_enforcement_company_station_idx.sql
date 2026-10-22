-- Bound-link index for station occurrence pages. The revision constraint
-- always applies first; the resolution kind narrows to bound links and the
-- station join stays indexed. No fuzzy, proximity or name matching is indexed.
CREATE INDEX quality_enforcement_company_bound_station_idx
 ON quality_enforcement_company_associations(revision_id, resolution_kind, station_id);
