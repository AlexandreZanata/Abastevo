# PR128 / PR129 protected delivery recovery

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (focused repair checks; remote integration pending)

## Authorized behavior

User requested both existing PRs delivered to main without losing changes,
with CI approved before each merge. PR128 targets dev; PR129 delivers dev
to main. Preserve commits with normal merges, never squash or rewrite.
The occupied primary checkout and every other worktree remain untouched.

B-BR-DELIVERY-ALIAS-01: the delivery helper accepts the verified renamed
AlexandreZanata/Abastevo origin as well as its historical repository name;
foreign owners and lookalike names still refuse. The GitHub repository ID
is 1393052962 for both API routes, verified before any publication.
All existing branch, head/base, clean-tree and check guards stay mandatory.

BUC-DELIVERY-01: reconcile current main into dev, repair real CI failures,
reconcile dev into landing preserving both progress histories, merge PR128
only on its successful exact-head checks, then deliver the complete dev
through PR129 only with all applicable checks successful.

## Actual failure evidence

- PR128 run 38164638001 / job 114544601645: gofmt refuses
  backend/cmd/api/main.go in its stale dev parent. Update to current dev;
  do not discard backend changes in favor of landing's old copy.
- PR129 run 38171437195: both SEC03C real-S3 tests fail to pull
  minio/minio:RELEASE.2025-04-22T22-12-26Z. Remote registry probes also
  refuse the historical digest and Quay mirror. Preserve the real S3
  assertions and use a reviewed, immutable test-service image.

No deployment, device/soak acceptance or release certification is implied.

## Test-service dependency review

Only the two integration fixtures change their MinIO image. The replacement
is Bitnami MinIO 2025.5.24 (source commit ecde75f9112f8410cb6cacb4b76193f1475b587e),
manifest-list digest
`sha256:451fe6858cb770cc9d0e77ba811ce287420f781c7c1b806a386f6896471a349c`.
Docker registry inspection and a fresh pull succeeded. Image labels identify
Broadcom/Bitnami and their upstream container source; `minio --version` and
`/opt/bitnami/minio/licenses/minio-2025.5.24.txt` confirm GNU AGPLv3, the same
MinIO license already used by the test service. No backend/app module dependency
or runtime deployment image changes. The existing mc image is also pinned to
its already reviewed compose.validation digest, rather than floating latest.

Security review: this is a legacy test-only binary, not a current supported
production dependency or a vulnerability-free image certification. It binds
only ephemeral loopback ports with synthetic credentials; data is bounded to
64 MiB tmpfs, the process is non-root (1001), memory 512 MiB and CPU 2. Docker
removes its anonymous volumes on cleanup. No production credentials, data,
network endpoint or persistent volumes are passed. Missing Docker now fails
these integration proofs explicitly instead of reporting a skipped success.
Real SigV4/type/key/expiry/interruption/over-limit/concurrency and honest/rejected
verify-loop assertions remain intact. Future production storage maintenance
remains separate from this CI fixture recovery.

## Focused validation

Parent dev `603d5ba`, reconciled with current main `6f062a4` via normal merge
`8f74555` (no content conflict).

- Alias regression RED: 41 passed / 4 failed for the renamed origins.
- `bash scripts/tests/test-git-flow.sh`: GREEN, 45 passed / 0 failed,
  including foreign origin refusal and unchanged head/base/check guards.
- `bash scripts/tests/test-project-batch-flow.sh`: PASS.
- `cd backend && go test -race -count=1 -tags=integration -run TestSEC03C ./internal/modules/evidence/adapters/storage ./internal/modules/evidence/application`:
  PASS, both real S3 matrices, no skips (1.749s / 1.778s).
- Shell syntax, gofmt, diff whitespace and tracked secret-surface scan: PASS.

Next: publish the repair on dev; reconcile landing against that exact dev
and verify both parent histories. No PR merge has occurred at this checkpoint.
