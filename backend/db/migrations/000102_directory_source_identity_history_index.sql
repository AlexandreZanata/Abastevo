-- FQ-05W3: exact historical continuity and branch review must not scan the
-- retained identifier universe. Active-only indexes cannot serve these reads.
CREATE INDEX directory_identifiers_source_history_idx
 ON directory_identifiers(kind,normalized_value text_pattern_ops,station_id)
 INCLUDE(valid_to);
