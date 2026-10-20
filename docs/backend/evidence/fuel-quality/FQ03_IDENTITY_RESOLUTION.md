# FQ-03 — Identity resolution and review policy

Status: LOCAL_DONE / INTEGRATION_PENDING. Date: 2026-10-14.
Base: `codex/fuel-quality-plan` atop `origin/dev 08a54dc4`. Depends: FQ-01;
Directory audit reconciled (identity ownership reused, not redefined).
BUC-FQ-02 contracted. No station write, migration, API or Android.

## Behavior before implementation

FQ-01 froze link/correction shapes but no resolution rule: nothing decided
when a source CNPJ was unknown, ambiguous, a wrong branch or a historical
replacement, and no explicit revision contract bound links to clocks.

## Delivered

- `backend/internal/modules/quality/domain/resolution.go` (new,
  stdlib-only): read-only `DirectoryView` port (`ByCNPJ`/`ByBase`, no
  writes — FQ-04 adapts the real store), `Binding` validity windows with
  date-only values, `Resolve` with five outcomes (bound / unmatched /
  ambiguous / wrong_branch / succession_review) and `LinkRevision` with
  independent source/review clocks plus `Validate` against the owning
  window. Checksum validation stays with kernel/Directory: resolution
  gates shape only and treats well-formed identifiers opaquely.
- Rules: full establishment CNPJ plus chronology decide; name, address,
  brand and proximity never participate. Exact in-window bindings bind
  (renames stay bound); out-of-window observations route to succession
  review, never auto-rebind; duplicate bindings collide into review via
  FQ-01 `ReviewLinks`; same-base different-branch routes to review without
  assigning any station; unknown or malformed identifiers quarantine with
  typed reasons. Inputs and the Directory view are never mutated.
- `resolution_test.go`: numeric/alphanumeric/leading-zero CNPJs via
  kernel vectors, two active matches, wrong branch (no station assigned),
  historical replacement (succession + in-window control), renamed station,
  same-address different-CNPJ non-merge, unmatched/malformed quarantine,
  read-only DeepEqual guard and six link-revision window/clock cases.

## Validation

- `go test ./internal/modules/quality/... -count=1`: 22/22 PASS (13 FQ-01
  + 9 FQ-03); first run green — RED was the missing coverage. A draft
  exact-key-only lookup stub that would have left wrong-branch
  unreachable was replaced by the explicit `ByBase` port during review,
  before any test ran against it.
- `go vet`, `gofmt -l` clean; `kernel` suite PASS (vocabulary untouched).
- `git diff --check` clean; new-file Markdown links resolve;
  scoped secret/PII scan clean (synthetic fixture CNPJs only).
- Rust crate untouched this round (no preparation changes).

## Recovery

Remove the proposed binding from publication: every non-bound outcome
already yields an inactive link with its reason, so dropping review output
restores the previous public state. Quarantined records replay through the
same pure function. Next: FQ-04 Go ledger, migrations and atomic
publication.
