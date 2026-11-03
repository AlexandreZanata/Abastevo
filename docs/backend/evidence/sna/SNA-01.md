# SNA-01 — Names and full addresses at ingestion

Status: LOCAL_DONE
Validation: PASS

Bounded children, declared before behavior: SNA-01A covers the pure Go name
rule and CSV/official API parsing; SNA-01B covers the existing Rust v2 batch's
additive v3 name enrichment and matching Go admission. SNA-02 owns persistence,
projection and backfill. No intermediate parsing commit claims stored/public
name acceptance or authorizes importing data.

B-BR-STATION-NAME-01: Given a separately approved correction, then a usable
trade name, then a legal name, select in that order. Trade names made only of
punctuation, tested placeholders, the establishment CNPJ, or the same legal
name after case/whitespace normalization are absent. Preserve source casing;
keep legal_name verbatim. Source names never absorb a community correction.

B-BR-STATION-ADDRESS-01 (parsing): Preserve the ANP component keys used by
export (street, number, complement, district, postal_code, municipio, uf), plus
municipio_ibge where available. Never infer an address or use coordinates as
address text. Source precedence/projection follow in SNA-02/SNA-04.

BUC-SNA-01: Ingest a bounded source snapshot, retain legal and trade fields,
compute its source display name through the pure rule, preserve full addresses
and all existing quarantine/status/coordinate checks. A changed parser uses a
new snapshot/checksum identity; previously complete runs cannot mask new parsing.

Observed base 415c5af0: Rust emits station-assertion-v2, not v1; its existing
RFB geocode struct has no trade name. SNA-01B will add a separately versioned,
local-file-only enrichment input, preserving old artifacts and rights metadata.
Public CPF minimization remains binding at the existing read boundary; retaining
the official legal name does not authorize exposing personal identifiers.

## SNA-01A checkpoint (2026-11-03)

Status: LOCAL_DONE (Go parsing child) / SNA-01 IN_PROGRESS
Validation: PASS

Tested parent 415c5af0 plus this child. RED reproduced dropped name/address
fields and replay of an old parser completion. The domain guard also refused
an external Unicode-normalization import; the final rule uses stdlib only.
GREEN: full domain/registry race suites; real PostGIS registry (33.434s),
read (32.190s) and adapter publication/recovery/concurrency (22.034s) suites.
The latter initially required its missing Rust executable; built the actual
locked prepare_registry and reran the complete failed package, no skip.
Affected vet/gofmt, secret scan and diff-check PASS.

CSV parser identity is registry-csv-v2 and API is registry-api-v3. Logical
reconcile inputs resolve the current versioned snapshot; explicit historical
versioned identities remain addressable. New checksums prevent an immutable
old row masking the new parse. Full address and raw legal/trade values are
retained in Assertion; SNA-02 must add storage/projection before that is a
stored/public legal-name feature. No backfill, source download or staging write.
Next: SNA-01B additive v3 batch and deterministic pinned name enrichment.

## SNA-01B opening

B-BR-STATION-NAME-01B: The versioned local enrichment selects an active usable
RFB trade name by exact CNPJ, else the latest usable survey name, else none.
Never invent or title-case a value. Contradictory values at one source clock
refuse the input; a newer inactive RFB row cannot revive stale active data.

BUC-SNA-01B: Verify declared local pins/bounds/row counts before enrichment,
join through a CNPJ index, emit named v3 rows with source provenance and
correct succession checksums, bind provenance again at Go admission, preserve
v1/v2 compatibility and all existing source quarantine/publication behavior.
The input schema/budgets are frozen in contracts/station-prep/station-names-v3.md.
SNA-02 still owns storing these new fields and approved-correction projection.

## SNA-01B checkpoint (2026-11-03)

Status: LOCAL_DONE
Validation: PASS

Tested parent 5cfa2717 plus this child. RED: the Go strict decoder refused
name_inputs, and Rust lacked the v3 profile. GREEN: complete locked Rust suite
(including legacy golden/emission compatibility); three new Rust name/CLI
cases; clippy all-targets with warnings denied; Go domain/registry race, vet
and real PostGIS registry suite (29.790s). The actual new Rust executable emits
v3, Go preserves untrimmed legal names/components, both admission paths refuse
unbound pins, and the prepared transaction leaves no partial run after failure.
The final legal-name/source-binding guard was rechecked through the real
Rust→Go/PostGIS case (1.664s). Initial local httptest was sandbox-denied; the
same full affected race suite passed with local sockets permitted, no skip.

The minimized RFB/survey descriptor is bounded and verified before atomic
publication; no new dependency or source download. Old v1/v2 are readable and
the legacy CLI emits unchanged bytes. New source fields still await SNA-02
storage/projection; this does not claim backfill, API or national coverage.
Diff-check and changed-file secret surface reviewed. Next: SNA-02.
