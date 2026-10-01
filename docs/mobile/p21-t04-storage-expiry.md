# P21-T04 — Local storage and all-copy expiry evidence

Status: LOCAL_DONE on `codex/phase-21-photo-contribution`. Issue: #84. Entry P21-T03 satisfied. Binds B-BR-C01/C06 and BUC-C02/C03 to existing contracts. No product code change in this task; evidence only.

## Services (disposable, tmpfs, synthetic credentials)

- PostGIS `postgis/postgis:18-3.6@sha256:60f6ad…` (pinned, as in `infra/compose.validation.yml`).
- S3Mock `adobe/s3mock:4.11.0@sha256:cd49108c…` **substituted for MinIO**: `docker.io/minio` pulls are denied from this workstation and `quay.io` answers 401, so the pinned MinIO image could not be fetched. Runner: untracked `/tmp` compose mirror (db identical, storage → S3Mock), removed afterwards with volumes; zero containers left.
- Migrations 000001–000030 applied; bucket `validation-private` created; no production DSN, photo or credential anywhere.

## Results (all green, `-race`)

- Evidence unit: 7/7 packages ok (media forward/sanitize, storage presign/transfer, jobs, erase, application, domain budgets).
- Evidence integration (`-tags=integration`, PostGIS + S3Mock): 7/7 packages ok — presigned SigV4 PUT/GET/DELETE transfers, reserve/complete atomicity, verify/quarantine/sweep, idle-session expiry, forward E2E. The SigV4 round-trip passing against S3Mock validates the substitution for transfer semantics; signature-enforcement parity with MinIO is **not** claimed.
- Backup harness `test-backup.sh`: 10 passed, 0 failed (encrypted artifact, secret-free manifest, verify/refusals, age alarms, pruning).
- Restore drill `test-restore.sh`: 9 passed, 0 failed (integrity, counts, deletion-ledger replay on restored snapshot, erased state preserved, drill dropped, non-drill drop refused). First attempt failed twice on my env omission (missing `ANPFUEL_TEST_DRILL_DSN_PREFIX` from the matrix script, replay hit the dev default); rerun with the matrix variable is the recorded 9/0 — the failure was harness invocation, not product.
- No-storage path `TestPublicProcessIdentityAndSignedWrites` (empty endpoint): ok.
- 24 h boundary: `ForwardDeadline` + sweep/expiry/quarantine tests green inside the suites above; deletion failures stay visible (quarantine) and retryable (bounded backoff, worker pass noted by the drill warning).

## Explicitly deferred (not waived)

- Full `test-local-backend.sh` matrix (load, edge TLS, deploy, infra-config, security, govulncheck): unchanged P08-owned coverage, not rerun here; this task scoped storage/expiry/restore.
- MinIO-parity run: rerun against pinned MinIO wherever pulls work (CI or another station) before claiming emulator parity.
- Real-provider (G09) and device-matrix (P24) proofs remain owned by their gates.
