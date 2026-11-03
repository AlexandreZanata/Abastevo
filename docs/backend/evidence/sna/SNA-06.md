# SNA-06 evidence

## SNA-06A opening

B-BR-STATION-PORTAL-06A. Generated station pages keep the public display
name and use sourced textual address components. A named municipality and
UF can serve as the fallback. Coordinates and municipality identifiers
never substitute for address text.

BUC-SNA-06A. Exercise street and locality formatting, a municipality-only
record, and a record with malformed coordinate-shaped address components.
Preserve the existing public CSV fields and generated station identity.

Status: LOCAL_DONE
Validation: PASS

The four regression cases first failed on missing municipality text and
coordinate-shaped address fields. The generated address now uses a named
municipality when available and rejects coordinate literals without
altering the remaining source text or any CSV schema. The portal generator
and export suites passed with race detection. Focused regression, vet,
format, whitespace and secret checks passed.
