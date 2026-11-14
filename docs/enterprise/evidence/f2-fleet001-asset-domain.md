# EMP-FLEET-001 — Pure enterprise asset domain

Date: 2026-11-13.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Source: tested/published dev `fac002e7`; owned `codex/enterprise-f2`.
[Task216](https://github.com/AlexandreZanata/Abastevo/issues/216),
[phase215](https://github.com/AlexandreZanata/Abastevo/issues/215), cumulative PR203.
B-BR-EMP-96/97, BUC-EMP-49 and EDR-061 precede shared vectors/behavior.
The task introduces only organization-scoped asset value types, explicit
category/product catalog descriptors, exact tanks/consumption and activation
checks in Go/Kotlin. Personal repositories, UUID provenance guesses, persistence,
commands and runtime meter presence are outside this domain checkpoint.

Required: shared RED→GREEN vectors in both languages, boundaries/unknown catalogs/
foreign activation facts/mixed units/quantities/kind-change/redaction and defensive
copy checks; affected static/format/compile and secret/diff review. Whole F2
scope/dependencies remain tracked in [execution](f2-execution.md).

## Accepted source and recommended decisions

Go fleet/domain and Kotlin enterprise/fleet now implement distinct asset IDs,
explicit organization references, literal ASCII plate/fleet keys, bounded Unicode
metadata, three kinds, usage/lifecycle values, exact catalog-backed tanks and
consumption references, and activation readiness from scoped unit/meter facts.
Category catalogs are known-code only; they do not invent a kind matrix. The
runtime catalog and actual meter existence remain owning-source work.
No personal repository, three-vehicle limit, SQL, route or active runtime binding
is introduced. The new Go domain is registered in the quick package manifest.

The shared oracle contains133 distinct cases across normalization, tanks,
expected references, draft validation, activation, kind changes, states and
scope. Go/Kotlin consume every row and assert the expected operation counts.
UTF-8 hex preserves separators and Unicode exactly. Oracle SHA256:
`48105e2dc9462438a83d1dbf54108d7cf149ee15c363dc1f3003c12d20d34a7d`.

Independent review corrected Unicode scalar/UTF-16 product ordering and missing
organization error parity. Additional RED regressions exposed typed-nil catalog
panic in Go and inconsistent stored units under a changing catalog in both
languages. Unit consistency is checked across every tank and reference before
updating the accepted-unit map. Immutable snapshots, getters and all Go fmt/
JSON/slog or Kotlin string diagnostics preserve privacy. A raw UUID is not
claimed to reveal its origin table.

## Executed acceptance

- Go RED before implementation; final race run passes1.035s,10 top-level tests
  containing133 shared subcases plus9 independent boundary tests. Coverage98.0%
  statements. Vet, staticcheck and pinned Go1.27.4 format checks pass.
- Kotlin RED before implementation, then scope/catalog negative regressions
  before fixes. Final143 tests (133 shared,10 boundaries) pass13s with zero
  failures/errors/skips. Scoped new-package JaCoCo:157/157 lines,
  1706/1715 instructions(99.4752%),211/218 branches(96.789%). This does not
  certify the historical whole-project Kotlin coverage gate.
- The new Kotlin throwing helper was refactored to an exception factory with
  explicit throws, eliminating unexecuted Nothing-return compiler paths; no
  test, coverage exclusion or threshold was weakened.
- An independent literal-profile oracle verifies all23 normalization cases;
  final independent code review finds no remaining concrete blocker.
- Plan consistency348 tasks/zero problems, staged secret and whitespace review
  pass. This pure domain requires no device test or database migration.

```sh
go test -race -count=1 -coverprofile=/tmp/enterprise-fleet001-go.cover ./internal/modules/fleet/domain
go vet ./internal/modules/fleet/domain
GOTOOLCHAIN=go1.27.1 XDG_CACHE_HOME=/tmp/enterprise-staticcheck staticcheck ./internal/modules/fleet/domain
./gradlew --offline --no-daemon --no-parallel --max-workers=2 :domain:test --tests 'com.anpfuel.domain.enterprise.fleet.*'
python3 docs/enterprise/tools/check_plan.py
git diff --cached --check
git diff --cached --binary | gitleaks stdin --redact --no-banner
```

Go uses the pinned toolchain and GOCACHE=/tmp/enterprise-go-build from backend/;
Gradle runs at the repository root with JDK17. RED/GREEN logs are retained under
`/tmp/enterprise-fleet001-*`; Kotlin XML/JaCoCo are under domain/build/.
Tested Kotlin main SHA256:
`fe7ac158e1512642ef1254edb7554a6c5ae71051fdf443c1e41c9839c37f98e7`.

Next bounded dependency: EMP-OFFL-001 protocol freeze. The entire F2 remains
IN_PROGRESS; M1/M2/C1 and private runtime/manual/external gates stay open.
