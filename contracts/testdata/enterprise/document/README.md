# Pure document-domain oracle v1

EMP-DOC-001 / B-BR-EMP-185..189, BUC-EMP-96..97, EDR-134..136. This synthetic
oracle covers a pure private-document container: original identity, six
independent status dimensions, owned same-organization links and exact
allocations. It is not an upload, audited access, persistence, link API,
conference workflow, fiscal consultation, duplicate detection or
reconciliation job. No row contains content bytes, file names, real
identifiers or personal data.

## Format

`document-v1.psv` uses the exact four-column header
`id|operation|args|expected`. Fields are semicolon-separated `key=value`, split
at the first equals sign; values are literal (no `|`, `;` or newline) and are
never trimmed. No blank or unknown rows are skipped. Consumers assert the
exact hash, row count, operation counts, nonempty unique case IDs, distinct
field names, the allowed argument keys per operation and complete result
maps, and that every operation and every error code below appears in at
least one row (as `error=` or inside `refused=`). Unknown syntax fails the
loader; it is never mapped to a domain result.

Alias UUIDs use prefix `9d000000-0000-4000-8000-` plus a twelve-digit suffix:
A=1, B=2 (organizations); D=201, N=202 (documents); E1=211, E2=212, E3=213
(expenses); F1=221 (fueling); W1=231 (work order); T1=241 (trip); S1=251
(asset); P1=261 (employee); R1=271 (meter reading). `?` means absent (empty
text). Any other alias is literal input. Digest aliases: H1 is `1a` repeated
32 times, H2 is `2b` repeated 32 times; any other digest is literal.
Outputs print aliases back for known values.

## Fixed semantics

- Original identity is organization + canonical lowercase document UUID +
  64-lowercase-hex digest (EMP-PLAT-006 object format). Nothing changes them.
  An absent/malformed organization is SCOPE; a malformed, nil, uppercase or
  absent identifier or digest is INVALID.
- Dimensions and closed states (first is initial): STORAGE UPLOADING,
  RECEIVED, STORED, QUARANTINED, FAILED, REPLACED, DELETED; EXTRACTION NONE,
  PENDING, DONE, FAILED; STRUCTURE NOT_APPLICABLE, UNCHECKED, STRUCTURE_OK,
  STRUCTURE_INVALID; CONFERENCE UNCONFERRED, CONFERRED_OK, DIVERGENT,
  ILLEGIBLE; RECONCILIATION (derived) UNALLOCATED, PARTIALLY_ALLOCATED,
  RECONCILED, OVER_ALLOCATED; FISCAL NOT_CONSULTED, AUTHORIZED_AS_OF,
  CANCELLED_AS_OF, UNAVAILABLE. Tokens are exact; a token of another
  dimension is unknown in this one.
- Transition check order: dimension known, then from/to known in it
  (INVALID); RECONCILIATION → DERIVED; FISCAL requires a consultation instant
  and other dimensions refuse one (INVALID); current ≠ from → CONFLICT;
  (from,to) outside the table → INVALID_TRANSITION; a fiscal instant not
  strictly after the previous one → CONFLICT. A refusal changes nothing.
  Tables are those of B-BR-EMP-187; REPLACED is reachable only by REPLACE.
- Links: kinds FUELING, EXPENSE, WORK_ORDER, TRIP, ASSET, EMPLOYEE,
  METER_READING; foreign organization SCOPE; same kind+id twice DUPLICATE.
- Amount: 0..1000000000000 centavos, recorded or corrected at any time.
- Allocation check order: amount absent → AMOUNT_MISSING; more than 1000
  lines → INVALID; then per line in order: foreign → SCOPE, centavos outside
  1..1000000000000 → INVALID, repeated target → DUPLICATE, kind other than
  EXPENSE → NOT_ALLOCATABLE, unlinked → NOT_LINKED; finally an exact sum above
  the amount → EXCEEDS_AMOUNT. A set replaces the previous set.
- Even split: amount absent → AMOUNT_MISSING; zero or more than 1000 targets
  → INVALID; per target in order SCOPE, DUPLICATE, NOT_ALLOCATABLE,
  NOT_LINKED; then BR-MONEY-007 largest remainder with equal weights in the
  listed order (ties to the earlier target), zero shares omitted.
- Reconciliation: allocated 0 → UNALLOCATED; below the amount →
  PARTIALLY_ALLOCATED; equal → RECONCILED; above (only after a downward
  amount correction) → OVER_ALLOCATED. `remaining` is amount − allocated and
  is negative only when OVER_ALLOCATED; `?` while the amount is absent.

## Operations

Errors are exactly INVALID, SCOPE, INVALID_TRANSITION, CONFLICT, DERIVED,
DUPLICATE, NOT_LINKED, NOT_ALLOCATABLE, EXCEEDS_AMOUNT and AMOUNT_MISSING.

- ENUM (`kind=DIMENSION|STORAGE|EXTRACTION|STRUCTURE|CONFERENCE|RECONCILIATION|FISCAL|TARGET;value`)
  returns `value` plus `derived` (DIMENSION), `initial` (states) or
  `allocatable` (TARGET). Unknown `kind` is loader grammar, not a domain case.
- ALLOWED (`dimension;from`) returns `to`, the lexicographically sorted
  generic transition targets joined by `,`, or `-` when none.
- SCENARIO (`org` default A, `id` default D, `digest` default H1, `steps`)
  creates a document, then applies `~`-separated steps in order. A refused
  step is recorded as `N:CODE` (1-based) and the scenario continues with the
  unchanged document. Steps (fields separated by `,`):
  `T,DIMENSION,FROM,TO[,INSTANT]` (instant in the shared time profile, parsed
  before the domain call; malformed → INVALID); `AMOUNT,N` (signed decimal
  int64; non-integer → INVALID); `LINK,TARGET`; `ALLOC[,TARGET:N...]` (no
  lines clears the set); `SPLIT[,TARGET...]`; `DIGEST,DIGEST` (records whether
  it equals the original, as `match`). TARGET is `KIND/ALIAS` or
  `KIND/ALIAS@ORG` (default the document organization); targets, then each
  ALLOC integer, are built first in order (unknown kind or malformed id or
  integer INVALID, absent organization SCOPE) and the first failure refuses
  the step before the domain call. A construction failure
  returns only `error`. Otherwise the result has exactly `org, id, digest,
  replaces, replaced_by, storage, extraction, structure, conference,
  reconciliation, fiscal, consulted_at` (canonical UTC or `?`), `amount`
  (or `?`), `allocated, remaining, links` (`KIND/ALIAS` in link order or
  `-`), `allocations` (`KIND/ALIAS:N` in set order or `-`), `match` (last
  DIGEST result or `?`) and `refused` (`-` when none).
- REPLACE (`old_steps`, `new` default N, `new_org` default A, `new_digest`
  default H2, `new_steps`) builds the old document A/D/H1 and the successor,
  applies their steps (any refusal there fails the loader), then replaces.
  Check order: SCOPE (different organization), INVALID (same identifier),
  DUPLICATE (successor already replaces another), INVALID_TRANSITION (old not
  STORED/QUARANTINED, then successor not STORED). Success returns old
  id/digest/storage/replaced_by/conference/fiscal/reconciliation/links/
  allocations and new id/digest/storage/replaces/links.

The frozen corpus contains 251 cases: ENUM 64, ALLOWED 29, SCENARIO 146,
REPLACE 12. SHA256:
`27eaec5e45b641ab7ee31b70681d96690560a12a655f2f43d0f2cc2f55f1e383`.
