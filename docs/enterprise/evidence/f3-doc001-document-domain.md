# EMP-DOC-001 — Document domain: container, independent statuses, links and allocations

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded pure-domain task of F3 (module DOC, cut C1, type DOM). It models a
private document container independently of its links, human review and
fiscal outcomes, as a Go package and a Kotlin mirror that consume one shared
vector oracle. The Kotlin mirror exists because the app captures and imports
documents offline (EMP-DOC-004). No upload, session, audited access,
persistence, route, job, app or panel surface is added; EMP-DOC-002..014 keep
their ownership. Synthetic data only. No whole-F3, M3 or C1 acceptance is
claimed by this task.

Placement follows the proposed `document` module of
[09-architecture](../09-architecture.md): Go
`backend/internal/modules/document/domain/`, vectors
`contracts/testdata/enterprise/document/`, Kotlin
`domain/src/main/kotlin/com/anpfuel/domain/enterprise/document/`.

Sources read: EDR-014; BR-TRUTH-001/002, BR-TEN-001, BR-MONEY-007
([04-business-rules](../04-business-rules.md)); the whole
[documents module](../modules/documents.md); the six Document rows of
[08-state-model](../08-state-model.md) §3 and its §4 rules;
[06-domain-model](../06-domain-model.md) §§3–5; REQ-10.1..10.6 of
[17-traceability](../17-traceability.md) §10; BR-PRIV-004/008/020 of
[11-privacy-security](../11-privacy-security.md); the EMP-PLAT-006 private
original identity `backend/internal/modules/orgmedia/domain/object.go`; and
the BR-MONEY-007 allocator `orgvalue` `Allocate` / `EnterpriseValues.allocate`.

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-185..189, BUC-EMP-96..97, EDR-134..136. All are
used; none remain reserved.

Rules B-BR-EMP-185, B-BR-EMP-186, B-BR-EMP-187, B-BR-EMP-188, B-BR-EMP-189, BUC-EMP-96, BUC-EMP-97 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-DOC-001 — Document domain: container, independent statuses, links and allocations" (frozen before behaviour; text moved verbatim at integration).

### Decision rows

Decisions EDR-134, EDR-135, EDR-136 are recorded in [16-decisions](../16-decisions.md).

## Delivered surface

- Go `backend/internal/modules/document/domain/` (`status.go`, `document.go`;
  tests `vectors_test.go`, `document_test.go`, `invariants_test.go`). Imports
  only the standard library, `kernel`, `metrics` (MaxTotalCentavos) and
  `orgvalue/domain` (MaxAllocationLines, `Allocate`). Commands return a new
  immutable value; a refusal returns the unchanged receiver and its error.
  Value types embed the redacted String/Format/LogValue/MarshalJSON pattern.
- Kotlin `domain/src/main/kotlin/com/anpfuel/domain/enterprise/document/`
  (`DocumentStatus.kt`, `EnterpriseDocument.kt`; tests
  `EnterpriseDocumentVectorsTest.kt`, `EnterpriseDocumentTest.kt`). Reuses
  `EnterprisePartition.requireId`, `PremiumBounds.MAX_TOTAL_CENTAVOS`,
  `EnterpriseValues.MAX_ALLOCATION_LINES` / `allocate`. Commands return a new
  value or throw `DocumentDomainException(failure)`; lists are unmodifiable
  copies; `toString` is redacted.
- Oracle `contracts/testdata/enterprise/document/{README.md,document-v1.psv}`:
  251 cases (ENUM 64, ALLOWED 29, SCENARIO 146, REPLACE 12), SHA256
  `27eaec5e45b641ab7ee31b70681d96690560a12a655f2f43d0f2cc2f55f1e383`. Both
  consumers assert hash, header, unique IDs, per-operation counts, allowed
  argument keys, complete result maps and that all ten error codes appear.
  The rows were hand-specified through a default-projection template helper
  (scratchpad only); no expectation was generated from either implementation.
- The EMP-PLAT-006 storage model is referenced, not re-modelled: the digest
  profile equals the private object SHA-256 format, and a Go invariant test
  proves both profiles accept/refuse the same digests. No MIME, size, key,
  file name, bytes or personal data enter the domain.

Required negative vectors present: digest match leaves fiscal NOT_CONSULTED
and conference UNCONFERRED (also after STRUCTURE_OK/extraction DONE);
refused transitions leave the original and every dimension unchanged
(`digest-after-refused-overwrite`, `storage-*`); transitions out of FAILED and
DELETED; allocations crossing organization, to unlinked or non-expense targets,
zero/negative/above-maximum/int64-overflow lines, duplicates and sums above the
amount (including the documents.md 97 350 example with the refused 8 000);
duplicate link; unknown/lowercase/padded/cross-dimension/"VALIDATED" states,
unknown dimension and unknown target kinds.

## RED → GREEN → REFACTOR

Stubs compiled with the exported API and `errNotImplemented` / `TODO()` bodies
before any behaviour.

- Go RED (`/data/dev/worktrees/f3-logs/doc001-go-red.jsonl`): 14 top-level
  tests, 13 fail and 1 pass (loader-grammar test); 251 vector subtests, 249
  fail and 2 pass (`new-org-absent`, `new-org-malformed`: the fixture scope
  cannot be built, so the harness returns SCOPE before any domain call);
  0 skips; 0.042 s.
- Kotlin RED (`/data/dev/worktrees/f3-logs/doc001-kotlin-red.log`, XML in
  `doc001-kotlin-red-xml/`): 260 tests, 259 failures (251 vectors on
  `NotImplementedError`, 8 unit), 1 pass (loader grammar), 0 errors, 0 skips;
  test time 0.305 s, Gradle 59 s wall.
- Go GREEN (`doc001-go-green.jsonl`): 14 top-level / 256 subtests (251
  vectors + 5 per-dimension graph subtests) pass, 0 failures, 0 skips, 2.539 s
  with `-race`; statement coverage 99.5% (`doc001-go-coverage.out`; the only
  miss is the defensive, unreachable allocator-error branch of SplitEvenly).
- Kotlin GREEN (`doc001-kotlin-green.log`): 260/260, 0 failures/errors/skips.
- REFACTOR: Kotlin `fail` is inlined so no unreachable post-call
  `KotlinNothingValueException` throw is generated, the unused document
  `equals/hashCode` was removed and target/line equality is now tested.
  Re-run green. Kotlin document package JaCoCo: instructions 1862/1889
  (98.57%), branches 133/138 (96.38%), lines 161/163 (98.77%).
- Mutation sanity (Go, sources restored byte-identical afterwards): removing
  the stale-fiscal check, releasing quarantine to STORED, allowing an excess
  of one centavo, making FUELING allocatable or keeping zero split shares each
  fails 2–3 shared vectors.

## Validation

Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f3-go-build`, from `backend/`:

```sh
gofmt -l ./internal/modules/document                      # empty
go vet ./internal/modules/document/...                    # pass
go test -race -count=1 -json -coverprofile=/data/dev/worktrees/f3-logs/doc001-go-coverage.out -covermode=atomic ./internal/modules/document/...   # pass, 99.5%
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/document/...   # pass
```

Kotlin (JDK 17, serialized with `flock /data/dev/worktrees/.f3-gradle.lock`),
`./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.*' --offline
--no-daemon --no-parallel --max-workers=2 --console=plain
-PcomapsSdkDir=<worktree>/build/comaps/7ad828883c4aca8a`: whole enterprise
domain suite 1611 tests, 0 failures, 0 errors, 0 skipped (document 260),
Gradle 26 s wall, no compiler warnings; log
`/data/dev/worktrees/f3-logs/doc001-kotlin-enterprise.log`. The repository
has no ktlint/detekt/spotless task, so compilation plus tests is the Kotlin
static gate (as in the F2 precedent).

`python3 docs/enterprise/tools/check_plan.py`: 348 tasks, 545 rules, 167 use
cases, 145 screens, 0 problems. `git diff --cached --check` clean. Staged
secret/PII grep hits are only the redaction tests' `secrets` lists of
synthetic identifiers asserted absent from output and the word "token" for
enum tokens; gitleaks reports no leaks. No dependency, migration, SQL, route,
OpenAPI, app/data module, DI, job or producer is added.

## Retained obligations

Persistence, upload sessions and audited private access (EMP-DOC-002);
link persistence, idempotent replay and consumer authority, including link
removal and any link-count bound (EMP-DOC-003); app capture/import and offline
queue (EMP-DOC-004); conference workflow with revisions and reasons
(EMP-DOC-005); fiscal structure, access key/QR/XML and authorized consultation
port (EMP-DOC-006, EMP-INTEG-004); fiscal items and allocation locking/races
and DB no-double-counting (EMP-DOC-007); duplicate detection (EMP-DOC-008);
extraction (EMP-DOC-009); reconciliation job and basis revisions
(EMP-DOC-010); replacement/cancellation commands with audit (EMP-DOC-011);
retention, deletion and holds (EMP-DOC-012); panel and app screens
(EMP-DOC-013, SCR-DOC-01..03); lifecycle/verification jobs (EMP-DOC-014).
Real receipts stay legally gated (EMP-GOV-002, LV-04, LV-11). No F3, M3, C1,
release or runtime acceptance is claimed.

## Proposed shared-doc changes

1. `docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`: append the CONTRACT block
   above as a new section after the most recent enterprise section.
2. `docs/enterprise/16-decisions.md`: append the EDR block rows
   EDR-134..136 after the current last row.
3. `docs/enterprise/backlog/ep09-documents.md`: set EMP-DOC-001 state to
   LOCAL_DONE and append the BACKLOG block below under its task block.
4. `docs/enterprise/09-architecture.md` module table: the `document` row may
   drop "(proposed)" for its pure domain package only (persistence, routes
   and storage port remain proposed).
5. `docs/enterprise/03-glossary.md` §9 "Reconciled": until a reviewed
   tolerance exists, the domain uses zero tolerance (EDR-136); suggested
   wording "Allocated amounts agree exactly with the document amount (no
   reviewed tolerance yet, EDR-136)".
