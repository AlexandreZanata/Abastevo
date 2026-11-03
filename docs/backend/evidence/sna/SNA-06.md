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

## SNA-06B opening

B-BR-STATION-ADMIN-06B. The signed-in admin shell may inspect the current
public directory record by exact station ID. Display name, legal name
and address source are separate fields. Missing facts stay unknown.
Station lookup does not change the record or grant access to private data.

BUC-SNA-06B. Exercise valid and invalid lookup, missing facts, text-only
rendering and stale response rejection across a new request or route change.

Status: LOCAL_DONE
Validation: PASS

The admin DOM tests initially failed because the station route was absent.
The session-held shell now opens a read-only station lookup by UUID and
renders the public fields as text. It clears results on route changes,
ignores stale responses and handles null legacy facts without deriving
a name or source. The existing station detail endpoint and session policy
are unchanged. All 50 admin tests, typecheck, build, distribution,
whitespace and secret checks pass.
