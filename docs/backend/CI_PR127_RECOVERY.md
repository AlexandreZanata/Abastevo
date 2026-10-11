# PR127 CI prerequisite recovery

Status: LOCAL_DONE
Validation: PASS

Scope: user-authorized CI correction and protected dev-to-main integration,
2026-10-11. No VPS changes are required for this task.

B-BR-CI127-01: fast/quick retain all static checks; worker error strings follow
Go's lowercase convention without changing readiness or reserve behavior.
B-BR-CI127-02: every full backend integration invocation builds the locked Rust
registry preparer and supplies its absolute executable path before Go tests.
The real pipeline test must fail on a missing prerequisite, never skip or use a
mock. The CI integration job uses the same pinned Go version as fast/quick.
B-BR-CI127-03: protected merge requires successful checks on the current PR
head/base; no bypass or weakening of tests, checks or branch protection.

BUC-CI127-01: a clean runner builds Rust and executes the actual preparation,
atomic loading, crash recovery, changed reads and overlap test on disposable
PostGIS; fast/quick also pass staticcheck.

Observed head `587c3a0`: backend fast and Quick verification failed ST1005 in
`station-sync/safety.go` lines 74/78; integration failed because
`ANPFUEL_TEST_PREPARER_PATH` was missing. Other SQL errors in service-container
logs belong to explicit negative tests; the failing Go case is identified
separately. Logs obtained from individual job REST endpoints because this
installed gh client returned empty aggregated failed logs.

Implementation: backend CI builds `prepare_registry` using pinned Rust 1.99.0,
`--locked` and an isolated target directory; `GITHUB_ENV` supplies its absolute
path to subsequent steps. Rust sources now select backend CI. The integration
job uses pinned Go 1.27.2 instead of an undefined `GO_VERSION_FILE`. Both
workflows locate their Go dependency cache through `backend/go.sum`. The full
local backend script and README expose the same preparer prerequisite; two
shellcheck warnings in the touched script were also corrected without changing
test selection.

Validation environment: local disposable PostgreSQL 18/PostGIS, the same pinned
image digest as CI, dedicated container/volume labeled `app=abastevo-ci-pr127`,
1 CPU and 512 MiB container limit. No persistent developer database or VPS was
used. An initial full-parallel attempt with only 384 MiB tmpfs exhausted space
and the container memory limit; that run is invalid infrastructure evidence,
not a passing test or a diagnosed application regression. The replacement
uses an isolated disk volume and `-p 2`, matching the existing local runner.

Passed targeted checks:

```sh
cargo +1.99.0 build --offline --locked --manifest-path tools/station-prep/Cargo.toml --bin prepare_registry --target-dir /tmp/abastevo-ci-pr127-rust
(cd backend && GOCACHE=/tmp/abastevo-go-cache GOTOOLCHAIN=go1.27.1 staticcheck ./...)
(cd backend && GOCACHE=/tmp/abastevo-go-cache go test -race -count=1 ./cmd/station-sync)
bash -n scripts/tests/test-local-backend.sh
shellcheck scripts/tests/test-local-backend.sh
bash scripts/scan-secrets.sh
git diff --check
```

Both workflow files also passed Python/PyYAML parsing. Against only the disposable
database, with `ANPFUEL_TEST_PREPARER_PATH` pointing at the binary above:

```sh
ANPFUEL_DATABASE_URL="$ANPFUEL_TEST_DATABASE_URL" go -C backend run ./cmd/migrate
go -C backend test -race -count=1 -p 2 -timeout 10m -tags=integration ./...
```

Full integration: PASS, exit 0, 80 tested packages and no failed packages. This
includes `TestDailySyncActualRustLoaderRecoveryChangedReadAndOverlap` with the
actual Rust executable, migration recovery, publication, ownership negatives
and concurrent replay. Storage was deliberately unavailable, as in backend CI;
successful private-S3 runtime validation is not claimed. The separate all-tools
local matrix was not rerun: its changed shell is syntax/static checked, and its
actual preparer/Go integration prerequisite was exercised above.

Tested source is the CI correction atop `587c3a0`, with changed files listed in
the same atomic commit as this evidence; later edits affect documentation only.
Red evidence is retained in the failed jobs on that parent. Private local logs
are under `.local/ci-pr127-20261011/` and are excluded from Git/wiki.

Delivery: INTEGRATION_PENDING, existing protected PR127, dev to main. Actual
protection was read: strict `Quick verification` from GitHub Actions, enforced
for administrators, force push disabled. Current-head/base remote checks and
one local aggregate inside `git-flow.sh finish` still gate merge. Publication
outcomes belong to PR metadata/local notes after integration, then the next
useful source checkpoint. No VPS deployment, release certification, 24-hour
soak or maximum-user claim follows from these CI corrections.
