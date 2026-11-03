# SNA-03 — Additive public station metadata

Status: LOCAL_DONE
Validation: PASS

B-BR-STATION-API-03: Station adds nullable legal_name/address_source. Existing
field names/types and shared-cache invariance remain stable; old rows do not
invent metadata. Source-backed legal text stays verbatim where public privacy
rules permit it. B-BR-017 still removes CPF-shaped personal identifiers from
all public names; the private stored source value remains untouched. A value
left without a usable legal name is NULL, never fabricated from a display name.
Official.display_name reads the source's own value, independently of a derived
trade/correction label. No community value may gain an official anchor.

BUC-SNA-03: Propagate the two nullable fields through detail/search/nearby
queries, DTOs and OpenAPI; exercise actual PostGIS reads and HTTP contracts,
including legacy NULLs, source separation, CPF privacy, consistent cached
responses and unknown-field compatibility of existing consumers. No new UI
copy is introduced here; localized UI rendering belongs to SNA-05/06.

## Checkpoint (2026-11-03)

Tested parent 91305b9e plus this task. RED: the public legal-name privacy
function was absent. GREEN: nullable legal_name/address_source propagate through
detail/search/nearby and HTTP serialization. Legal source formatting is retained
when no personal token is present; CPF-shaped tokens are removed only from the
public view, and a fully minimized name becomes NULL. Private stored text is
unchanged. Official queries use source_display_name with a legacy fallback,
so approved/derived labels cannot overwrite the official anchor's own value.

New PostGIS cases cover all three read paths, original source formatting,
CPF privacy, legacy NULLs, source separation and an old JSON field subset.
Real HTTP verifies explicit nullable properties on legacy rows. Full affected
real PostGIS/race suites pass: read 45.506s, HTTP/cache 5.335s, registry 44.798s
and publication/adapters 37.230s. Kernel/application/read/HTTP unit/race, full
backend build, affected vet/staticcheck and sqlc vet/diff pass. Staticcheck's
default read-only cache was redirected with its supported STATICCHECK_CACHE
setting; the unchanged check then passed. OpenAPI lint passes with exactly the
parent's 722 warnings/118 informs; no new lint findings. Secret/diff review pass.

The existing Android JSONObject codec reads named known fields and does not
reject optional siblings; it was inspected, not claimed as a tested old release
APK. Targeted mobile integration/rendering still belongs to SNA-05/06. No UI
copy or locale strings changed in this server-only task. No staging mutation,
source download, ingestion or live acceptance. Next: EVC-00 inventory and source
register reconciliation, then EVC-01..03 before SNA-04..06.
