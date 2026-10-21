# EV preparation and census contract v1 — EE-03

Date: 2026-10-21. State: CONTRACT_FROZEN / RUNTIME_PENDING.
Owns B-BR-EV-07/08/23 and BUC-EV-02/03/09/12 at the preparation boundary.
This is the envelope contract for `charging-prepared-v1` and
`vehicle-energy-reference-v1`, separate from live API and old station-prep formats.

[Planned schemas](../../contracts/openapi/ev-preparation-planned.json),
[trusted profiles](../../contracts/ev/preparation-profiles-v1.json),
[synthetic fixtures](../../contracts/testdata/ev/prepared-v1/expected-census.json),
[byte/identity goldens](../../contracts/testdata/ev/prepared-v1/byte-goldens.json),
[Rust primitives](../../tools/station-prep/src/ev_contract.rs) and
[local verifier](../../scripts/verify-ev-prepared-contract.py) are executable.
No live paths, producer, Go bridge, database, source activation or app is added.
The [EE-02 typed contract](EV_TYPED_DATA_CONTRACT.md) remains authoritative for
capabilities, numerical units, tariffs, measurements and public/private ports.

## Files and fatal boundary

A batch directory contains exactly `manifest.json`, `entities.ndjson`,
`decisions.ndjson` and `aliases.ndjson`, all regular local files. Empty streams
are zero bytes, not one empty line. No symlinks, unknown artifact names, relative
traversal, absent descriptors or private source files are accepted in that folder.

The manifest binds format, byte/identity/privacy profiles, selected trusted
profile hash, rights hash, source revision, reference hash, sorted source input
inventory (logical filename, raw bytes/hash, root count), per-artifact byte/line/
hash descriptors and per-kind census. Source basenames are identifiers, never
paths to open. `content_sha256` hashes the canonical manifest with that one field
removed; manifest framing LF is not part of its content identity.

A future preparer must read every admitted raw input and required reference,
verify input bytes/hash, independently count all source units including nested
children and produce one decision per unit. Truncation, conflicting same-key
assertions, unknown required profile/rights, corrupt reference, invalid framing,
limits or incomplete accounting abort the whole run; no accepted partial batch
may replace the previous output. A successful parse is not license, publication,
identity-link or trust approval. EE-04/05 own raw-source extraction/census and
atomic old-output preservation; EE-06 owns the actual typed Go bridge and
cancellation/close/race/resource acceptance. This microstage does not claim them.

The local verifier checks the **prepared** envelope and returns a minimized
summary only after every artifact validates. It explicitly returns
`input_bytes_verified:false` and `source_admission_enabled:false`. It never reads
raw source files, emits rows, accesses the network/DB or publishes. Its bounded
in-memory implementation is a contract oracle, not a production streaming bridge.
Existing station-prep ceilings are reused: total raw input descriptor bytes and
aggregate artifact bytes each <=100MiB, source roots/decision units and each stream
<=500,000. Records/manifests <=1MiB; strings <=64KiB; JSON depth <=32. Future
national workloads must prove these budgets or introduce a reviewed amendment;
never truncate to fit. The three typed clocks and per-payload field lists also
bound individual envelope arrays.

## Deterministic bytes and identity

`ev-json-v1` is UTF-8 without BOM, compact JSON, ASCII object keys sorted by byte
order, no Unicode normalization, original Unicode string values, JSON control
escaping (short escapes where defined, remaining controls lowercase `\u00xx`).
HTML characters and U+2028/U+2029 remain literal UTF-8; `/` is not escaped.
No floats, exponent numbers, duplicate JSON keys, non-finite numbers, unpaired
surrogates or integers outside ±9,007,199,254,740,991. Booleans are not integers.
Arrays retain order. NDJSON records are canonical objects plus one LF, including
the last record; the manifest is canonical JSON plus LF. Entities sort uniquely
by key, decisions by unit, aliases by alias key, inputs by filename. Field
provenance sorts by field; clocks are created_at, verified_at, status_at.

`ev-source-key-v1` SHA256-hashes four UTF-8 fields, each preceded by its unsigned
32-bit big-endian byte length: domain `ev-source-key-v1`, lowercase source
namespace, entity kind and exact external ID. Namespace <=64 ASCII lower/digit/
hyphen bytes; external ID 1..256 UTF-8 bytes without control characters. Revision,
provider metadata, parent coordinates, payload and array order are excluded.
OCM PLACE uses its source ID; CONNECTION_GROUP uses `PLACE-ID:Connection-ID` so a
connection ID is scoped to its parent. Missing source identity quarantines; array
position is a locator, never a generated equipment identity.

An assertion digest hashes kind/key/parent_key/payload/rights_sha256/source_clocks
only, using canonical bytes. Per-input unit locators and duplicate occurrences do
not change that digest. Source clock fields retain the source observation or
null, never fetch/save time; no generated/fetched time renews evidence. Source
revision, reference and rights changes create a different batch content identity.
An input corrected in a later revision can keep its stable source key.

## Nested loss accounting, minimization and aliases

Decision `unit` is `logical-file#zero-based-root` plus a bounded JSON pointer
relative to that root. Every source root has its own empty-pointer decision.
Every child has an existing parent in the same file/root and a descendant pointer.
PLACE/VEHICLE_REFERENCE are roots; EVSE -> PLACE; CONNECTOR -> EVSE;
CONNECTION_GROUP -> PLACE. Per kind, input = accepted + duplicates + quarantined;
root counts are separate. Manifest totals must equal the decisions, and every
accepted entity has exactly one accepted decision, with no orphan entity/child.

ACCEPTED references its emitted entity and assertion hash; DUPLICATE references
an accepted unit with the same key/assertion, never a duplicate chain or silent
first-wins conflict. Quarantine is held **inside decisions**, containing only
unit, kind, typed reason and raw hash; key/external_id/assertion/duplicate target
are null. No second quarantine stream is necessary. Quarantined descendants use
PARENT_QUARANTINED. Raw contacts, comments, photos, private addresses, contributor
GPS and arbitrary diagnostic/source payloads are absent. Typed public-place name
and coordinates are source infrastructure assertions only; eligibility and PII
minimization must be verified by the real source adapter before publication.
UNKNOWN access cannot carry coordinates; a null coordinate needs its null pair.

Payload and field-provenance paths are closed by kind/profile. Public coordinates
are bounded exact decimal strings (<=16 fractional places in this profile), not
money/energy floats or contributor GPS; out-of-range/unsupported precision fails
rather than rounding. Public source name/BR/reported point count, exact W,
connector/current reference codes and reported group quantity are separate facts.
Null/UNKNOWN retains unknown power/quantity/type; no default zero or guessed code.
Reported group quantities and NumberOfPoints are **not** independently identified
EVSE/connector counts or simultaneous capacity. An OCM connection row cannot create
physical EVSEs by expanding Quantity. Static source clocks cannot claim live status.

Field provenance records all payload fields, including nulls, with source-relative
paths; unit+input revision/hash supplies the parent locator. Provider attribution,
license and change-notice obligations stay in the rights/profile record linked by
hash, never become a source-controlled permission grant. API presentation requires
the admitted EE-08/09 source record and EE-02 attribution, not only a digest.

Aliases are separate from input census, explicit REVIEWED_SOURCE_ID references
with a proof digest and same-kind emitted target. Names/proximity do not merge
identities. Alias key cannot also be an entity; targets cannot be aliases, so
self/cycle/chained/unknown-target aliases fail. The proof hash is structural
provenance, not verification of a real review: EE-04/06/07 still owe that proof
and conflict handling at admission. Synthetic proofs never admit real links.

## Pinned profiles and evolution

- `ocm-br-20261021-v1`: candidate based on the EE-01 commit/reference/provider1
  license hashes. PLACE and CONNECTION_GROUP only; exact known ID/clocks/field
  paths frozen. This minimal staging profile omits contact/comment/photo/free
  tariff text. Full-Brazil extraction, public eligibility, unknown lookup codes,
  exact source conversion, minimized-title review and real admission remain EE-04.
- `pbe-2026-pdf-v1`: actual PDF hash, VEHICLE_REFERENCE envelope and explicit
  RIGHTS_AND_LAYOUT_PENDING. There is **no invented CSV/PDF row identity or model
  payload mapping**. Nonempty output is refused until EE-05 resolves rights and
  publishes a separately versioned, reviewed row/field/unit profile. Empty
  diagnostic envelopes do not qualify or activate PBE. The VEHICLE_REFERENCE
  model_code example is synthetic only, not a frozen PBE field layout.
- `synthetic-hierarchy-v1`: CONTRACT_TEST_ONLY; never production-admissible. Six
  accepted entities: one place, one EVSE, two physical connectors, one aggregate
  connection group (reported quantity3), one vehicle reference. Four source roots,
  14 units =6 accepted +5 duplicates +3 quarantine; one reviewed synthetic alias
  outside census. Duplicate/quarantined children never inflate physical counts.

All admission flags remain false. These profiles cite the
[actual EE-01 qualification](../backend/evidence/ev-support/EE-01.md); samples and
CC-BY provider metadata do not authorize an entire corpus or private fields.

Profile IDs are immutable and append-only once accepted; new mapping, snapshot,
license/rights, field or reference semantics get a new ID/hash. Keep old profiles
and old station-prep formats readable. Unknown profile/required field/version is
fatal, preserving the old accepted output. Extend the byte/envelope version for
incompatible framing/identity/required shape changes. Adding an optional future
producer does not silently reinterpret old hashes or relax privacy. A changed
trusted license invalidates an old manifest under the new profile, even when
artifact hashes are recomputed. Review explicit migration/replay before activation.

## Executable acceptance

Run from the repository worktree:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/tests/test-ev-prepared-contract.py
PYTHONDONTWRITEBYTECODE=1 python3 scripts/verify-ev-prepared-contract.py contracts/testdata/ev/prepared-v1/nested
cargo +1.99.0 test --locked --offline --manifest-path tools/station-prep/Cargo.toml --lib --test ev_prepared_contract --test ev_contract_vectors
```

From `backend`, run `go test -race -count=1 ./internal/platform/apicontract`.
Rust and Python share exact byte/ID goldens; Go independently verifies schema and
reviewed UTF-8/framing hashes. Semantic negative tests reseal descriptor/content
hashes so missing roots, bad census/parents/duplicates/provenance/aliases/rights
cannot pass merely because hashes agree. The CLI late-artifact corruption test
requires empty stdout and a minimized reason. Real Rust->Go source parity and
runtime admission are expressly pending EE-04–06.
