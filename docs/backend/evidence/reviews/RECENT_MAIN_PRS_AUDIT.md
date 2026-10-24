# Independent audit of recently merged main PRs

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Started: 2026-10-22. Completed: 2026-10-23. Pinned main: `c3de5c03bce626300da3f10c1b64820d6451f1fc`.
Range: PR140–146, base `2b9cf1e48944839f7c5252068da4f732de0b9bd1`;
383 changed paths including evidence, generated code and tests.

[Machine-readable local acceptance](RECENT_MAIN_PRS_ACCEPTANCE.json).
[Independently fetched merged heads/checks](RECENT_MAIN_PRS_REMOTE_CHECKS.json).

## Scope and authorization

The user requested review, validation and necessary fixes while preserving an
agent working elsewhere. This audit starts only from merged main, on isolated
`codex/audit-recent-main-prs`. Neither the primary dirty checkout nor occupied
`dev`/`main-reconciliation` is edited, switched, stashed, reset, synchronized,
published or merged. Unmerged expansion/EV work is outside this audit.

Reviewed clusters: Go runtime/security and Home/community local refresh;
location generation/search; private ledger ownership, consent, exact money,
odometer/report windows and erasure; Go/Kotlin/Rust tariff parity; typed price
ranking/revision cache; native enforcement parsing, privacy, jobs, publication,
migrations and HTTP; current-company association; Android quality/occurrence
HTTP/cache, explorer/filter/detail state and delivery guards.

B-BR/BUC before behavior: preserve local content and honest stale/unavailable
state; exact money and source clocks; explicit official/community and
source-global/current-company scopes; no raw personal data or invented legal
finality/historical ownership. Current-company attribution requires current
full-CNPJ Directory evidence, fenced against retirement/succession/withdrawal.
Observed HTTP revocation/no-store cannot resurrect old evidence. Traversal must
remain complete and bounded. Failed/incomplete enabled CI blocks delivery even
when omitted from the configured required list.

## Confirmed findings and corrections

1. **Current-company publication proof (PR145).** Enforcement publication
   validated/staged association plans but omitted their bindings and identity
   from the shared guarded request. A bound plan could publish/replay without
   Directory proof, and changed decisions could replay an older plan. Derive
   admitted bindings, require the existing transaction guard before publication
   and replay, and include the canonical plan in publication identity. Nil
   association behavior and frozen legacy replay remain unchanged.
2. **Live station attribution (PR145).** Station pages/coverage trusted stored
   bound links plus active station status. Retiring a CNPJ while leaving the
   station active retained false attribution. Reads now lock the accepted
   revision/context and use Directory-owned evidence/guard ports in the same
   transaction. Bounded 512-candidate batches aggregate only valid links and
   refill public pages; coverage counts the complete eligible population.
   Mutable company proofs answer `no-store`, including conditional reads.
   Duplicate/malformed query parameters are refused.
3. **Revocation/cache and stale disclosure (PR141/146).** Occurrence no-store
   left older entries reusable; prefixed origins failed revocation clearing.
   The same prefix/HTTP-revocation issue existed in PMQC. Both clients evict on
   explicit HTTP refusal/no-store, including 304 no-store, before later outage
   fallback. Validated PMQC snapshots remain available as explicitly stale
   after malformed/foreign responses, matching its existing contract. Invalid
   occurrence frames cannot poison a cache. Occurrence pages now expose the
   fallback stale flag; wrong detail IDs and mixed private row fields refuse
   before persistence. A shared synthetic complete Go wire fixture prevents
   Kotlin rejection of accepted nullable legacy fields.
4. **Android state/revision races (PR146).** Detail loads swallowed cancellation,
   allowed obsolete success/error publication and never surfaced stale state.
   Add generation fencing and clear content when switching identity. Explorer
   coverage and continuation now use the displayed revision; incompatible
   coverage cannot replace its lease. Persist filter changes, preserve valid
   rows on refresh/failure, distinguish unavailable source from an empty search,
   and propagate cancellation.
5. **Nullable geography interoperability (PR145).** Associated stations without
   canonical location emitted empty state text, violating the nullable UF
   contract and causing Android to reject the page. Emit JSON null for missing
   station state/municipality; source geography is never substituted.
6. **Stale migration acceptance (PR142/145).** Seven upgrade tests stopped their
   exact expected suffix at migration000111. Extend those exact lists through
   000114 while retaining row hashes, frozen-source/pointer checks, empty
   association checks, replay and rollback assertions. No migration is edited
   or new migration invented.
7. **Delivery guard ignored enabled failures.** Historical GitHub results show
   [PR142 integration failed](https://github.com/AlexandreZanata/Abastevo/actions/runs/39777774444/job/119820998453)
   and [PR145 integration failed](https://github.com/AlexandreZanata/Abastevo/actions/runs/39835513636/job/120023290159)
   at their merged heads while Quick verification succeeded. The helper only
   inspected names passed via `--required`. It now fetches one complete
   paginated exact-head snapshot and structurally checks all enabled signals
   before local quick/push/merge. Failed/incomplete optional checks block;
   required checks still need completed/success. Optional path skips/neutral
   outcomes are not success evidence. API, pagination, shape and head errors
   refuse. No protection setting or required context is weakened.
8. **Domain coverage gate.** Baseline functional tests passed, but the unchanged
   90% instruction gate failed at 89.46%. Add a behavioral catalog regression
   for production event defaults: repeated dispatch of identical source
   payloads generates distinct IDs, uses dispatch time rather than an old
   source/fixture clock and preserves every payload. All explicit-ID/time
   tests remain. Coverage is now 90.44%; threshold/exclusions are unchanged.

During repair, regression tests also caught and removed nested pool acquisition
in the new reader (use Directory `EvidenceAt` in the caller's transaction) and
a collation-sensitive pagination sentinel. UUID keysets with an explicit
initial boundary replace that sentinel. These were repair iterations, not
claims about defects shipped in the earlier PRs.

## Reproduction and acceptance

Initial backend unit/race, Kotlin suites/build/lint and Rust tests passed.
New regressions failed on the reviewed behavior for unguarded publication,
replay, stale cache/disclosure, obsolete detail success/error, revision mixing,
missing geography and the delivery guard. The full tagged matrix additionally
exposed the seven outdated migration assertions. Repair-only fixture/build
failures are retained in local logs; they are superseded by successful scoped
reruns rather than presented as green aggregate output.

Final combined backend result: **116 packages passed**, 26 generated/empty
packages have no tests; **3287 successful test outcomes including subtests**,
four explicit opt-ins skipped. This reconciles the one broad tagged run with
final affected-package reruns; it is not a fabricated second full-run exit.
Skipped: SEC05E soak, vector regeneration, Android-consent cross-runtime and
real PMQC methodology. None is claimed as runtime acceptance.

- Real disposable PostGIS18/PostGIS3.6 and private S3; complete migrations to
  000114. Existing services/volumes are not used or pruned.
- Final affected race matrix: API root, Directory adapter, all quality packages
  and API contracts. Includes publication/replay, retirement, concurrent
  identifier/revision withdrawal, stale conditional reads, one-connection
  pool, null location, strict filters and a **513-record/512-station** census
  whose final station crosses a private batch boundary. All public pages are
  bounded, unique and complete; no public CNPJ.
- Actual pinned local source: **244247 records**, two complete **2443-page**
  traversals with replay/private correction and every public field compared;
  PASS under race (319.18 seconds in this environment). Source/raw data remain
  outside Git. This is not production latency or source-admission evidence.
- Kotlin: domain644/application390/data477/app410; no failures/errors; two
  pre-existing data opt-ins skipped. Debug APK/build and lint PASS; domain
  coverage 25384/28067 instructions (90.44%), gate PASS.
- Rust170 tests, 42 shared tariff vectors, fmt/clippy PASS; no dependency added.
- Go vet/staticcheck, deterministic sqlc generate/vet, OpenAPI zero errors,
  govulncheck zero vulnerabilities and focused security suites PASS.
- Git-flow real-temporary-Git/fake-GitHub harness:49 PASS/0 failures;
  JSON/pagination/head/state parser:7 matrix tests PASS; project-batch/worktree
  isolation harness PASS. `make test-flow` now includes the parser matrix and
  passed its two CI-cadence tests. Old helper reproduced two no-quick/no-merge failures.
  New parser also accepts the real completed PR146 check-response shape without
  performing any remote mutation.
- Diff/secrets review includes staged new files. No source flag, SDK/runtime
  dependency, schema, main protection or unrelated product behavior changed.

Commands used (scope/environment supplied from private local receipts):

```sh
# One broad matrix; failures are corrected and the affected union rerun below.
cd backend
go test -json -race -tags=integration -p 2 -timeout 12m ./...
go test -json -race -tags=integration -p 2 -timeout 8m \
  ./cmd/api ./internal/modules/directory/adapters ./internal/modules/quality/... \
  ./internal/platform/apicontract
go vet ./...
GOTOOLCHAIN=go1.27.1 staticcheck ./...
sqlc vet && sqlc generate
govulncheck ./...
cd ..
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest \
  :app:assembleDebug :app:lintDebug
./gradlew :domain:jacocoTestCoverageVerification
cargo +1.99.0 test --offline --locked --manifest-path tools/station-prep/Cargo.toml
cargo +1.99.0 fmt --manifest-path tools/station-prep/Cargo.toml --check
cargo +1.99.0 clippy --offline --locked --manifest-path tools/station-prep/Cargo.toml --all-targets -- -D warnings
make test-flow
bash scripts/check-security.sh
bash scripts/scan-secrets.sh
git diff --check
```

## Delivery boundary

All seven PR merges and main c3de5c03 were independently rechecked remotely.
Historical CI results certify neither this correction head nor production.
Correction integration remains pending: preserve the occupied maintained dev,
reconcile these atomic corrections into it when that front is ready, validate
changed inputs and use protected dev→main delivery with the new guard and
current-head checks. No intermediate PR/CI wait, deployment, source activation,
tag, issue closure, wiki publication or production certification occurs here.
Device/manual, signed staging contribution→feed, source rights/admission and
production measurements remain explicitly OWED.
