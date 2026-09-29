// Package read serves current community prices from the indexed
// projection (P06-T05): one row per exact price key, never a history
// replay. Query-time expiry and HIGH decay apply at read, independent
// of worker health (B-BR-008): expired projections surface as UNKNOWN
// with STALE freshness, and HIGH decays to MEDIUM once its anchor
// requirement fails. Missing keys report absence; callers render
// null/UNKNOWN, never an ANP substitution (B-BR-001).
package read
