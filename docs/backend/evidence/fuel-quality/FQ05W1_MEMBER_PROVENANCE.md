# FQ-05W1 — Monthly archive provenance and monotonic publication

Status: LOCAL_DONE
Validation: PASS

Date: 2026-10-16. Maintained dev, parent a83d5afd. B-BR-FQ-01/02/03/04/07/12/13,
BUC-FQ-01/02/03/04; prebehavior scope in [FQ-05W](FQ05W_NATIVE_RUNTIME.md).
This is the bounded persistence prerequisite of native runtime composition.

## Behavior and evidence

The source's annual ZIP contains twelve CSV/JSON pairs. Parent hash alone cannot
identify a monthly edition. Append-only migration 000100 freezes optional exact
member name/hash/bytes, includes member in resource primary keys, carries member
through discovery payload/dedupe and exposes it additively in revision detail.
Existing memberless JSON serialization/publication keys remain byte compatible;
99→100 real-PostGIS upgrade, exact existing revision replay and absent historical
member metadata are verified. No fabricated legacy member information.

PMQC publications now freeze validated real-calendar coverage periods through the
same atomic path as event publications. Corrections retain prior member/period
snapshots. Traversal, control characters, malformed/lowercase hash, inconsistent
member metadata, oversize member and duplicate period labels are refused.

Actual RED: publishing September then August regressed the current pointer.
GREEN: historical publication remains available, current edition and completed
watermark are monotonic. The current row stores its edition so ON CONFLICT compares
the locked row, avoiding a stale SQL statement-snapshot subquery. Eight real
publishers are blocked at a held SQL pointer lock, then released; newest edition
wins irrespective of completion order. Same-edition correction/replay suites pass.

## Validation

Disposable local Compose PostGIS on loopback 5434; Go toolchain pinned by repo.
Commands from backend/, GOCACHE=/tmp/fq-completion-go-cache:

- `ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/fq-native-rust/debug/prepare_quality go test -count=1 -race -tags=integration ./internal/modules/quality/...`: seven packages PASS.
- After adding upgrade/member tests: same command scoped to `./internal/modules/quality/adapters ./internal/modules/quality/adapters/jobs ./internal/modules/quality/application`: PASS.
- Actual member HTTP response/OpenAPI: `go test -count=1 -race -tags=integration ./internal/modules/quality/adapters/http -run TestQualityMemberProvenanceActualWire`: PASS.
- `go test -count=1 ./internal/platform/apicontract`, affected `go vet`, API/worker/migrate/ops build, pinned staticcheck, gofmt, sqlc vet and regeneration hash comparison: PASS.
- `git diff --check`, tracked-secret scanner and changed/untracked secret/PII surface review: PASS. Fixtures use synthetic identifiers only; source raw rows are private temporary files.

No native worker activation, deployment, completed source coverage or scoring
claim. Next: bounded archive/process, Directory identity fencing and worker
composition in FQ-05W, then FQ-12–17. D remains NOT_AUTHORIZED.
