# F1 EMP-PLAT-009 exact reconciliation and allocation

Status: LOCAL_DONE
Validation: PASS

Date 2026-11-11. Scoped task validation passes; the pre-existing aggregate
domain coverage gate remains failed as detailed below. Delivery state is
INTEGRATION_PENDING; F1/M1 remain open.

## Entry and reviewed source

The owner requested reading Claude's review, then the next enterprise task
and GitHub publication. The clean enterprise worktree is on
codex/enterprise-f0-f1 at \`1034b410\`, matching fetched origin/dev and PR202.
Main \`ef96cd13\` remains an ancestor. Other worktrees and occupied local dev
are preserved. Delivery reuses [PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).

Read [the Claude review](f1-pr202-review.md), the updated PR description and
the four correction commits \`b91326d9\`, \`ce0800c4\`, \`3422c8e1\` and
\`1034b410\`. Source inspection confirmed the original-integrity/PUT fence,
credential-free company availability check, native-form safeguards,
explicit company-open button, independent logout and S3 CI composition.
The corrected panel's typecheck/build/check and 13 Node tests passed in
this execution. The PR has no separate inline review threads/submissions.

Review activation blockers remain owed: single-family revocation, real AUTH
providers, ordinary runtime login, per-member transfer limits, private-bucket
lifecycle and backup/restore authority. Pure value helpers neither depend
on these runtime providers nor activate their surfaces.

## Implemented scope

B-BR-EMP-21, B-BR-EMP-22 and BUC-EMP-12 were written before behavior.
The new Go orgvalue/domain package and Kotlin EnterpriseValues in the
existing domain module reuse financial-v1 integer representations and
existing bounds. Go imports pure metrics/kernel bounds; Kotlin reuses
PremiumBounds, PremiumUnit and PortableMoney price bounds. L/M3 are the
existing liquid/gas unit vocabulary. No replacement money hierarchy,
floating-point arithmetic, dependency, migration, route or workflow is added.

Canonical integer text parsers reject signs, leading zeros, padding,
fraction/exponent notation, Unicode digits and overflow. Nullable facts
remain explicit unknowns. Reconciliation:

- Preserves paid, canonical unit and original source-price text.
- Computes half-up(quantity_milli * unit_price_milli / 10000).
- Compares widened paid-plus-discount with a one-cent tolerance.
- Returns ARITHMETIC_MISMATCH above tolerance without rejecting or rewriting
  paid. Known input bounds prove product <= 10^15 and gross <= 2*10^12.
- Labels absent price as unknown for consistency, while supplying an exact
  rational derived milli-BRL price from paid/quantity. It never backfills
  recorded price from that derived result.

Source-price text is preserved, not parsed or silently quantized. DOC/FUEL
still owns source extraction/normalization and record acceptance. This
consistency helper checks the explicitly supplied integer price according
to BR-MONEY-005; it does not claim full-precision document conversion.

Allocation floors exact weighted shares, awards residual centavos by
descending remainder then ascending explicit line order, and returns
canonical ordered parts. Products/weight sums use existing standard-library
big integers, including sums above int64. Negative/duplicate orders,
negative weights, over-bound source and >1,000 lines are refused. Missing
source/weight or zero basis is unknown; known zero source with positive
basis yields known zeros. Inputs are unchanged; the Go paid snapshot also
does not alias the caller's pointer.

The existing personal driver allocator retains its ID-based tie rule.
These are context-free value operations, not private repository access or
organization authority. Owning use cases still resolve scope/permissions.

The [shared oracle](../../../contracts/testdata/enterprise/README.md) has
112 fixed synthetic rows. Both consumers assert the complete expected
result, header/count/unique IDs and known operations without language skips.
VAL-08/VAL-09 examples, source text, exact rounding, discount/tolerance,
unknown/zero, malformed/overflow inputs, reordered/tied shares and maximum
weights are covered. Existing Premium/driver fixtures are unchanged.

Tested blobs: Go \`c7ea2f8672a6af24be19675efc2b09e80b38feea\`,
Kotlin \`85c350a983e0cdd84e0efec0a173551c168a11bb\`, shared oracle
\`2b970a757eb5bdf9ffe419d6f8d3058de1514f7d\`. Source, tests and evidence
travel in the same atomic task commit.

## Executed checks

Linux amd64; Go 1.27.4, pinned staticcheck v0.8.1, Gradle 8.10.2 and the
existing JDK17/Kotlin toolchain. Go used GOCACHE=/tmp/enterprise-go-build;
Gradle ran offline with no new dependency. No database/provider was needed
for these pure scalar operations.

- RED: the shared Go tests failed because the domain implementation did not
  exist. Further RED cases caught known expected/derived values being erased by an
  independent missing paid/discount fact. After correction, GREEN: all 112
  oracle rows and deterministic invariant/budget/
  snapshot tests pass in both languages.
- \`go test -race -coverprofile=/tmp/enterprise-plat009-coverage.out ./internal/modules/orgvalue/domain ./internal/modules/kernel ./internal/modules/metrics ./internal/modules/ledger/domain ./internal/modules/driver/domain -count=1\`
  passed. New orgvalue domain statement coverage is 100%. Existing kernel,
  metrics, ledger and personal-driver domain suites also passed.
- Targeted \`:domain:test --tests com.anpfuel.domain.enterprise.EnterpriseValuesTest\`
  passed the initial 109 tests. The final full-suite class passed 115 tests
  (112 dynamic rows and three invariant/regression tests).
- Full \`./gradlew :domain:test :domain:jacocoTestCoverageVerification --offline --no-daemon\`
  ran 1,057 tests with zero failure/error/skip. Its separate global coverage
  verification failed; this is not recorded as a passing command.
- New EnterpriseValues logic: 100% instructions, branches and lines.
  The complete enterprise package, including generated data-class methods:
  93.12% instructions, 100% branches, 97.67% lines.
- \`go vet\` and pinned staticcheck passed for all five affected Go packages.
  Staticcheck used the documented go1.27.1 compiler workaround.
  API/worker/ops builds passed with the declared runtime toolchain.
- Mobile static guards passed. The new Go package joins the quick manifest.
  The enterprise verifier/checker regressions, explicit gofmt,
  whitespace/link and changed/new-file secret review passed.

## Existing global coverage failure

Reproduced the same full Kotlin test/coverage command in a separate
\`git archive 1034b410\` tree at /tmp/enterprise-plat009-review-baseline.
The baseline has no enterprise package and passed 942 tests with zero
failure/error/skip, but its configured coverage gate also failed:

- Baseline: 54,864/68,714 instructions = 79.8440%, minimum 90%.
- With EMP-PLAT-009: 55,690/69,601 = 80.0132%, minimum 90%.

These counts apply the existing repository/DomainModule exclusions exactly.
New logic passes its scoped meaningful coverage and increases aggregate
coverage; it does not introduce this failure. The existing threshold,
exclusions and tests were not weakened. Overall coverage acceptance remains
FAILED/PENDING and is a recorded obligation before claiming that aggregate
gate or broader phase acceptance. This task does not claim a fully green
global check or merge readiness.

## Retained delivery gates

Claude's runtime/activation blockers, unavailable online npm advisory audit
and Rust-dependent worker integration environment remain in the review
record; this task does not resolve or reinterpret them. No auth/session,
private-media, SQL, Android UI or live intake behavior changed.

F1/M1 remains open. The next bounded task is EMP-PLAT-010, transactional
enterprise events, upon authorization. Remote checks/review must apply to
the new PR head. No merge, deployment, wiki sync or release certification
is performed or inferred.
