# National registry continuity and portal discovery — 2026-11-05

Status: IN_PROGRESS

## Behavior contract

B-BR-NAT-01: The fiscal portal exposes labeled state and municipality selectors,
with municipalities constrained to the selected state. Place filtering applies
to the full admitted edition, independent of its HTML pagination. Names/CNPJ
can further narrow results. Clearing or changing state clears an incompatible
city. Late asynchronous responses cannot replace a newer selection. Empty
coverage is explicit; unmatched/ambiguous fines never acquire an inferred
city or station. Retain source dates, status and safe association limits.
The website remains static: browser filtering reads immutable release-addressed
JSON files only, never an API/database search or per-visitor backend rendering.

B-BR-NAT-02: Every station heading in the public portal uses the canonical
public display name. A known distinct CPF-minimized legal name appears below
at lower emphasis. Equal/unknown secondary names are omitted. Both names are
searchable. Snapshot hashes and Directory change detection include this
metadata so a name-only correction regenerates the static portal. Preserve
legacy snapshot compatibility, official amounts, IDs and source attribution.

B-BR-NAT-03: National refresh discovers complete official editions, freezes
source/name/reference bytes, admits only validated rows and publishes through
owned idempotent ports. Compare edition membership, record additions/changes/
missing identities and expose last success/staleness. Absence from one remessa
is not proof of closure: retain history and independent community facts;
explicit official revocation or accepted correction owns operation status.
Failed/partial/suspicious editions retain the previous successful publication.
New ANP identities must participate in the automatic name-enrichment step,
without relying on a manually prepared list of today's establishments.

B-BR-NAT-04: The user selected a VPS-local monthly OpenCNPJ snapshot. A
short-lived scheduled updater checks the official source for a newer complete
edition, verifies it before use and retains the previous snapshot. Daily ANP
refresh joins its current CNPJ membership against that snapshot automatically;
unchanged source pairs reuse a hash-bound minimized cache. Source update and
name export share a filesystem lock. Bounded process/disk/deadline guards and
private permissions protect the shared host. Logs contain only stage/count
metadata; no OpenCNPJ company/partner/contact output. The website remains
static and consumes only newly generated immutable portal publications.

BUC-NAT: Exercise place changes, out-of-order responses, no-result cities,
both-name search and unknown/equal names; verify real PostGIS export/change
detection. Replay national additions/removals/changed names and interruptions
with deterministic source fixtures and actual owned loaders. Publish one
cumulative protected dev → main batch; only observed runtime receipts establish
deployment and freshness.

## Portal checkpoint

Portal status: LOCAL_DONE / INTEGRATION_PENDING.

The static generator now carries the minimized optional legal name through
Directory export, snapshot hashing, station pages, rankings, fiscal records
and all search files. Prefixes/search text include both names. Unknown/equal
secondary labels are omitted. Source hashes remain in a disclosure below the
visible edition/date. State/city filters read immutable JSON only; no website
API/search service is introduced. Working filters hide the unfiltered HTML
fallback, which remains usable without JavaScript or after a file failure.

Validation: 78 landing tests, TypeScript/build, portal/snapshot/job Go suites,
real PostGIS export and name-only fingerprint cases, Go vet, sqlc generate/
vet/drift, whitespace and secret scan passed. The browser selected MT/Sorriso
and displayed the linked fixture's trade/secondary legal name; at 390px there
was no horizontal overflow. The new filter regression was red before behavior
and green afterward. National source automation is the next local phase;
this checkpoint does not claim staging publication.
