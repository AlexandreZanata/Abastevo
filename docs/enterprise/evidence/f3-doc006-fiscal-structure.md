# EMP-DOC-006 — Fiscal structure: access key, NFC-e QR code and XML structural validation

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 pure-domain task in its own worktree
(`codex/enterprise-f3-doc006`), based on the integrated head with EMP-DOC-001
(pure document domain) and EMP-DOC-002 (document persistence). The
coordinator owns the shared docs, backlog state, PROGRESS, pushes and the PR.
This task adds the pure Go subpackage
`backend/internal/modules/document/domain/fiscal`, its Kotlin mirror
`com.anpfuel.domain.enterprise.document.fiscal` (access key, QR code and key
comparison; the app reads NFC-e QR codes during capture, EMP-DOC-009) and one
shared oracle `contracts/testdata/enterprise/fiscal/fiscal-v1.psv`. No
migration, SQL, route, OpenAPI, Android app/data module, DI, persistence, job,
producer, provider contact, URL fetch or fiscal consultation is added.
Synthetic data only: issuers are alphanumeric synthetic CNPJs
(`SYNTHE000001..`), keys are built from synthetic fields with computed check
digits, hashes and digests are synthetic hex.

Sources read: EDR-014, EDR-134 and AS-10 ([16-decisions](../16-decisions.md));
BR-TRUTH-001/002 ([04-business-rules](../04-business-rules.md)); BR-PRIV-020
([11-privacy-security](../11-privacy-security.md) §4); REQ-10.2 and REQ-10.4
([17-traceability](../17-traceability.md) §10); the whole
[documents module](../modules/documents.md); the Document structure and fiscal
consultation rows of [08-state-model](../08-state-model.md) §3; the fiscal
document facts of [18-references](../18-references.md) §3 (access-key layout,
alphanumeric-CNPJ key pattern and ASCII−48 values from NT 2025.001, QR
versions and the `p` prefix, cancellation windows) and §4 (alphanumeric CNPJ);
task blocks EMP-DOC-006..009 of [EP09](../backlog/ep09-documents.md). Reused,
not duplicated: the EMP-DOC-001 domain (`document/domain`: `Transition`,
`Structure`, `Unchecked`, `StructureOK`, `StructureInvalid`; Kotlin
`DocumentTransition`/`DocumentState`), the kernel CNPJ type
(`kernel.ParseCNPJ`, alphanumeric-aware; the Kotlin `valueobject.Cnpj` is
digits-only without check digits, so the Kotlin mirror needs no CNPJ type: the
issuer field is only exposed and the XML issuer check is server-only), and the
exact centavos parser (`orgvalue` `ParseCentavos` /
`EnterpriseValues.parseCentavos`, bound `MaxTotalCentavos` 10^12).
Precedents: EMP-FUEL-001 (`21601ae8`) and EMP-DOC-001
([evidence](f3-doc001-document-domain.md)).

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-231..234, BUC-EMP-118..119, EDR-158..159. All are
used; none remains reserved.

Proposed contract section title: "Enterprise fiscal structure checks
(EMP-DOC-006)".

Rules B-BR-EMP-231, B-BR-EMP-232, B-BR-EMP-233, B-BR-EMP-234, BUC-EMP-118, BUC-EMP-119 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-DOC-006 — Fiscal structure: access key, NFC-e QR code and XML structural validation" (frozen before behaviour; text moved verbatim at integration).

### Decision rows

Recommended routine decisions taken unattended (rows only, for
[16-decisions](../16-decisions.md)):

Decisions EDR-158, EDR-159 are recorded in [16-decisions](../16-decisions.md).

## Shape frozen before behaviour

Go package `fiscal` (`backend/internal/modules/document/domain/fiscal`),
importing only the standard library, `kernel` (CNPJ), `orgvalue/domain`
(`ParseCentavos`) and the parent `document/domain` (structure transition):

- Tokens: `Outcome` (`StructureOK`, `StructureInvalid`, `Unsupported`),
  `Kind` (`AccessKeyInput`, `QRCodeInput`, `XMLInput`, `Comparison`), `Check`
  and `Reason` (closed lists; `Reason.Outcome()`), `Environment`
  (`Production`, `Homologation`), `Layout` (`Online`, `Offline`).
- Checks: `CheckAccessKey(string) Result`, `CheckQRCode(string) Result`,
  `CheckXML([]byte) Result`, `Compare(a, b Result) (Result, error)` with
  `ErrNotComparable`.
- `Result`: `Kind()`, `Outcome()`, `Reason()`, `Checks()`, `AccessKey()`,
  `QRCode()`, `XML()` (values only for STRUCTURE_OK) and
  `StructureTransition() (domain.Transition, bool)`. `AccessKey`,
  `QRReference`, `XMLSummary` and `Amount` (exact text, optional centavos)
  are immutable; all embed the redacted String/Format/LogValue/MarshalJSON
  pattern of `document/domain`.
- Limits: `MaxQRBytes` 2953, `MaxXMLBytes` 1 MiB, `MaxXMLDepth` 32,
  `MaxXMLElements` 100 000, `MaxItems` 990.

Kotlin mirror (`domain/src/main/kotlin/com/anpfuel/domain/enterprise/document/fiscal/`):
`FiscalStructure.checkAccessKey`, `checkQrCode`, `compare`, the same tokens
without the XML-only checks and reasons, and
`FiscalStructureResult.structureTransition(): DocumentTransition?`.


## Delivered surface

- Go `backend/internal/modules/document/domain/fiscal/` — `result.go`
  (tokens, `Result`, `Amount`, `Compare`, `StructureTransition`, exact
  TDec_1302 amount via `orgvalue` `ParseCentavos`), `key.go`
  (`CheckAccessKey`, IBGE UF list, ASCII−48 modulo 11), `qr.go`
  (`CheckQRCode`), `xml.go` (`CheckXML`, bounded `encoding/xml` scan); tests
  `vectors_test.go`, `fiscal_test.go`. Imports only the standard library,
  `kernel` (`ParseCNPJ`), `orgvalue/domain` and the parent `document/domain`.
  All value types embed the redacted String/Format/LogValue/MarshalJSON
  pattern; accessors return copies. No dependency is added.
- Kotlin `domain/src/main/kotlin/com/anpfuel/domain/enterprise/document/fiscal/FiscalStructure.kt`
  (`FiscalStructure.checkAccessKey`, `checkQrCode`, `compare`;
  `FiscalStructureResult.structureTransition()` returns an EMP-DOC-001
  `DocumentTransition` or null) and tests `FiscalStructureVectorsTest.kt`,
  `FiscalStructureTest.kt`. Reuses `EnterpriseValues.parseCentavos`; lists
  are unmodifiable copies; `toString` is redacted. The XML-only checks and
  reasons do not exist in Kotlin (EDR-159).
- Oracle `contracts/testdata/enterprise/fiscal/{README.md,fiscal-v1.psv}`:
  307 cases (KEY 64, QR 110, XML 111, COMPARE 13, FEED 9), SHA256
  `ad8cd1e15a4359b3e622741cd23030420612f506a0ff2238c40ce20502d84f08`; 118
  Go-only rows (every XML row and every COMPARE/FEED row with an XML
  operand), 189 mirrored rows. Both consumers assert hash, header, unique
  IDs, operation counts, allowed argument keys, complete result maps, the
  Go-only count and coverage of every outcome, check, reason and error they
  implement; each reason is asserted to carry its fixed outcome. Inputs were
  built by a scratchpad generator with its own modulo-11 and CNPJ check
  digits; every expectation is written per row, none comes from either
  implementation. Large inputs use the oracle's `unit`×`repeat` expansion.
- Required negatives present as vectors: malformed XML (unclosed, mismatched,
  two roots, text outside root, Latin-1/1.1 declarations, control character,
  a raw invalid UTF-8 byte) and oversized XML (1 MiB + 1, including
  an entity flood refused before parsing, with the exact-1-MiB boundary OK);
  DOCTYPE/DTD/entities refused without expansion (billion laughs, XXE SYSTEM
  entity, external DTD, parameter entity, empty DOCTYPE, ENTITY inside the
  root, DOCTYPE after the root) and an undefined entity left unexpanded;
  depth 33 and an unclosed 5 000-deep flood (32 OK), 100 001 elements
  (100 000 OK); wrong check digit, wrong length, lowercase, spaced, prefixed,
  full-width/Arabic-Indic digits and letters outside the issuer positions;
  QR with unexpected host (non-government, look-alike suffix, user info,
  port, IP, trailing dot, empty/hyphen/underscore/non-ASCII labels, 64-char
  label, 254-char host), parameters (extra, duplicate `p`, legacy plus `p`,
  trailing `&` → INVALID; no `p`, legacy v1 `chNFe`, uppercase `P` →
  UNSUPPORTED) and version (1, 4, 200, `02`, empty, missing → UNSUPPORTED;
  v3 contingency variant → UNSUPPORTED; malformed v2 field counts/shapes →
  INVALID); QR key ≠ XML key and QR key ≠ typed key (KEY_MISMATCH); a
  structurally valid NFC-e with `protNFe` cStat 100 "Autorizado" and a cStat
  101 sample stay STRUCTURE_OK only, feed Structure only and leave fiscal
  NOT_CONSULTED (`feed-xml-authorized-protocol-stays-not-consulted`); the
  result types expose no authorization/cancellation/consultation/status
  member (reflection tests in both languages), and a structure command
  re-aimed at FISCAL is refused by the document domain because it never
  carries a consultation instant.
- Recorded limitation pinned by a vector: remainders 0 and 1 both give digit
  0, so a digit-0 key can collide with a mistyped key
  (`key-check-digit-collision-accepted` is STRUCTURE_OK); unit tests prove
  every other single numeric substitution is refused, against an independent
  digit helper.

## RED → GREEN → REFACTOR

Stubs compiled with the final exported API: Go check functions returned the
zero `Result`, `StructureTransition` returned nothing and `Compare` a
not-implemented error; Kotlin bodies were `TODO()`.

- Go RED (`/data/dev/worktrees/f3-logs/doc006-go-red.jsonl`, first corpus of
  302 rows): 13 top-level tests, 8 fail, 5 pass (loader grammar, the
  declared reason→outcome catalog, the synthetic-issuer sanity check and the
  type-shape tests for redaction and the absent authorization surface); 302
  vector subtests, 302 fail; 0 skips; 0.018 s.
- Kotlin RED (`/data/dev/worktrees/f3-logs/doc006-kotlin-red.log`, XML in
  `doc006-kotlin-red-xml/`): 192 tests, 190 failures (185 vectors and 5 unit
  on `NotImplementedError`), 2 pass (loader grammar, reflection surface),
  0 errors, 0 skips; test time 0.686 s, Gradle 60 s wall.
- GREEN exposed one wrong oracle expectation, corrected rather than
  weakened: `key-swapped-number-digits` assumed the transposition is always
  detected, but the published digit maps remainders 0 and 1 to 0; the row now
  transposes a digit-2 key (detected) and the new
  `key-check-digit-collision-accepted` pins the digit-0 collision. The
  generator now asserts with its own algorithm that every KEY_CHECK_DIGIT row
  is detectable, and the unit property tests were corrected the same way. Two
  unit-test sample keys of 45 characters were fixed to 44. Coverage then
  showed one dead branch (a `.gov.br` suffix always yields three labels; the
  empty-label rule covers `.gov.br` alone), removed in both languages, and
  three boundary rows were added (`qr-host-only-no-path-no-query`,
  `qr-host-length-253`, `qr-host-length-254`), plus `xml-invalid-utf8-byte`
  so the "not UTF-8" clause of B-BR-EMP-234 has its own row. The frozen rule text gained
  the matching precision (host grammar/length, escape decoding, digit-0
  collision); no rule changed meaning.
- RED replay on the final 307-row corpus with the stubs restored through
  `go test -overlay` (worktree untouched;
  `/data/dev/worktrees/f3-logs/doc006-go-red-replay.jsonl`): 13 top-level,
  8 fail / 5 pass; 307 subtests, 307 fail; 0.012 s. The Kotlin RED covered
  185 of the final 189 mirrored rows; the four rows added later
  (collision and three QR host boundaries) passed at their first Kotlin run.
- Go GREEN (`/data/dev/worktrees/f3-logs/doc006-go-green.jsonl`): fiscal
  13 top-level / 307 subtests pass, 0 failures, 0 skips, 2.513 s with
  `-race`; statement coverage 100.0% (380/380,
  `doc006-go-coverage.out`). The whole `./internal/modules/document/...`
  untagged suite passes (626 tests).
- Kotlin GREEN: fiscal 196 tests (189 vectors, loader, 6 unit), whole
  enterprise domain suite 2285 tests, 0 failures/errors/skips, Gradle 26 s
  wall, no compiler warnings
  (`/data/dev/worktrees/f3-logs/doc006-kotlin-enterprise.log`). JaCoCo for
  the fiscal package: instructions 2093/2107 (99.34%), branches 247/260
  (95.00%), lines 168/168 (100%).
- REFACTOR: the XML watch tables became explicit literals (no `init`
  mutation); a redaction test flake was removed (the slog timestamp could
  contain a secret's digits; the time attribute is now dropped); Kotlin
  refuses a payload longer than 2953 chars before UTF-8 encoding it. Re-run
  green with the gates below.
- Mutation sanity (Go, final sources replaced only through `-overlay`, final
  corpus; logs `/data/dev/worktrees/f3-logs/doc006-mutation/`): ignoring
  DOCTYPE (7 vectors fail), depth off by one (1), any host (16), QR model 55
  accepted (2), three-decimal totals (2), UNSUPPORTED feeding structure (3),
  remainder 1 → digit 10 (9), protocol key ignored (1), extra parameters
  accepted (4), element limit +1 (1), size limit +1 (1), unknown model
  reported invalid (6), late declaration accepted (2) and centavos above the
  bound (3): all 14 mutants are killed by shared vectors.

## Validation

Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f3-go-build`,
`GOTMPDIR/TMPDIR=/data/dev/cache/f3-tmp`, from `backend/`:

```sh
gofmt -l ./internal/modules/document                      # empty
go vet ./internal/modules/document/...                    # pass
go test -race -count=1 -json -coverprofile=/data/dev/worktrees/f3-logs/doc006-go-coverage.out -covermode=atomic ./internal/modules/document/...   # pass; fiscal 100.0%
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/document/...   # pass (STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck)
```

Kotlin (JDK 17, serialized with `flock /data/dev/worktrees/.f3-gradle.lock`):
`./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.*' --offline
--no-daemon --no-parallel --max-workers=2 --console=plain
-PcomapsSdkDir=<worktree>/build/comaps/7ad828883c4aca8a`: 2285 tests, 0
failures. No ktlint/detekt/spotless task exists in the build files, so
compilation plus tests is the Kotlin static gate (as in the EMP-DOC-001 and
F2 precedents).

`python3 docs/enterprise/tools/check_plan.py`: 348 tasks, 545 rules, 167 use
cases, 145 screens, 0 problems. `git diff --cached --check` clean. The staged
secret/PII grep hits only the redaction tests' `secret` lists of synthetic
values asserted absent from output, the word "token" (XML decoder tokens and
state tokens) and the NF-e element name `emit/CPF` with the all-zero
synthetic CPF of the UNSUPPORTED issuer vector; `gitleaks protect --staged`
reports no leaks. No dependency, migration,
SQL, route, OpenAPI, app/data module, DI, job, producer, network call or
provider contact is added. Synthetic data only.

## Retained obligations

Persisting the structure result, its reason and checks with the document
revision and commanding Structure through the EMP-DOC-002 store (owner:
EMP-DOC-014 job / BUC-EMP-119); the XML upload inspector and type-by-content
activation (EMP-DOC-014; XML stays inactive in C1 per EMP-DOC-002); fiscal
items, totals and allocations from a STRUCTURE_OK XML (EMP-DOC-007); the
scoped duplicate signal by key and digest (EMP-DOC-008); the on-device QR
capture flow, confidence and manual entry (EMP-DOC-009, BUC-EMP-118); any
authorized fiscal consultation, signature or protocol verification
(EMP-INTEG-004, EMP-GOV-011, REQ-10.6); a reviewed per-state QR URL list and
the QR version-3 contingency layout once a primary source is pinned; CPF
issuers (UNSUPPORTED today). Version-2 field shapes beyond the recorded `p`
prefix and the TDec_1302 pattern are prior knowledge marked `UNVERIFIED`
against the primary manuals. Real receipts stay legally gated (OQ-04,
EMP-GOV-002). No F3, M3, C1, release or runtime acceptance is claimed.

## Proposed shared-doc changes

1. `docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`: append the CONTRACT block as
   a section titled "Enterprise fiscal structure checks (EMP-DOC-006)",
   preceded by the usual dated "EMP-DOC-006 — Fiscal structure: access key,
   NFC-e QR code and XML structural validation (2026-11-14)" heading with the
   evidence link.
2. `docs/enterprise/16-decisions.md`: append rows EDR-158 and EDR-159 from
   the EDR block after the current last EDR row.
3. `docs/enterprise/backlog/ep09-documents.md`: set EMP-DOC-006 state to
   LOCAL_DONE and append the BACKLOG block under its task block.
4. `docs/enterprise/modules/documents.md` (header paragraph after the
   EMP-DOC-002 sentence): "EMP-DOC-006 adds pure structure checks for access
   keys, NFC-e QR codes and a bounded server-only XML envelope
   ([evidence](../evidence/f3-doc006-fiscal-structure.md)); a STRUCTURE_OK
   result commands only the structure status and never fiscal
   consultation."
5. `docs/enterprise/18-references.md` §3, row "Check-digit weights 2 to 9…":
   append "Implemented and pinned by EMP-DOC-006 vectors; remainders 0 and 1
   both give 0, so a digit-0 key can collide with a single mistyped
   character." Row "NFC-e QR code has three versions…": append "EMP-DOC-006
   handles version 2 (online 5 fields, offline 8 fields — field shapes
   `UNVERIFIED` against the primary manual) and version 3 online; other
   versions and the version-3 contingency layout are UNSUPPORTED."
6. `docs/enterprise/09-architecture.md` module table: the `document` row may
   list the pure `domain/fiscal` subpackage (persistence and jobs remain
   proposed).
