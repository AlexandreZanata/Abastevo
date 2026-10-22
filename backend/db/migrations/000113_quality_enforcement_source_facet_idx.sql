-- Facet index for bounded enforcement reads. The revision constraint
-- always applies first; facet equalities narrow within one revision and
-- the native keyset order is preserved for stable pagination. No
-- expression, trigram or fuzzy matching is indexed: facets are exact.
CREATE INDEX quality_enforcement_source_facet_idx
 ON quality_enforcement_source_records(revision_id, uf, segment, subject_kind, document_day, native_key, id);
