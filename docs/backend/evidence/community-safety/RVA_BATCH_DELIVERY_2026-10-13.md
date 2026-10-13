# Validated safety and consent source batch — 2026-10-13

Status: INTEGRATION_PENDING
Validation: CI_PENDING

The user explicitly requested a dev → main PR now and a guarded merge only
if CI passes. This delivery integrates implemented, tested source changes;
it does not declare the entire offline roadmap or a production release done.
Baseline main/dev: 5dc9b2be834ebb1bca0a242c9115c4c43ff69527. Construction
head before delivery documentation: f7c0d7a8502925cbc66bc38d856e21621421d43d.

## Included source

- Bounded signed photo ingress; signup accounting preserved across erasure;
  per-issuer bounded JWKS recovery; durable account/privacy erasure recovery.
- Training revocation/output fences, durable copy inventories, conservative
  minimization refusal and bounded autonomous predeadline cleanup/admission.
- Actual owner-bound Android consent persistence, explicit complete terms,
  reader/UI/Keystore/recovery checks and retryable backend outage handling.
- First-OFF append-only 000074, narrow staging relay cap and batched directory
  page references with source isolation and two SQL round trips per page.
- Sanitized independent review, immediate SQL/S3/race and Android evidence,
  actual staging/deployment trials, failed repeats retained and ADB reinstall.

The original dev worktree remains occupied by an uncommitted Store edit that
removes tested intake protection. Preserve it without stash/reset/overwrite;
it is excluded from this validated delivery. A standalone temporary delivery
checkout owns its independent dev ref, so publishing/synchronizing remote dev
cannot move the original dirty checkout's branch/files beneath another effort.
Other historical mobile keyboard/preview branches are already ancestors of main.

## Acceptance and deferred operational gates

Main protection was read live: required GitHub Actions Quick verification,
strict base freshness, no forced updates, zero required human approvals.
All applicable checks and unresolved review threads must be checked on exact
head/base before invoking git-flow finish with the explicit required check.
The helper runs the final local quick gate once after remote success.
Two trailing spaces in archived Gradle output were normalized; seven probe
hashes cover separate probe sources and remain unchanged.

Existing targeted tests and live evidence are linked by current PROGRESS.
Training remains OFF and staging photo admission remains zero. Public maximum
upload edge authorization, sustained load stability, real-clock 24h/provider/
backup proof and remaining RVA08 helpfulness/RVA10–14/CS15–25 are still owed.
No release/capacity/complete-offline claim or administrative closure of these
obligations is authorized by a successful source merge. No VPS/phone mutation
is part of this GitHub delivery task. Wiki mirrors only the actual merged SHA.

## Wiki publication prerequisite repair

The existing export refused three RST chart embeds outside docs/assets. Mirror
only those three already-public SVG bytes into the allowed assets path and
rewrite their embeds; preserve all original evidence files/hashes. No exporter
allowlist or security guard is relaxed. Validate byte equivalence, focused wiki
harness and complete dry-run before publication from the eventual merged SHA.

## PR 130 current-head formatting repair

The concurrently published Privacy destination was reconciled normally into
dev; it hosts the existing owner-scoped TrainingConsentSettings without a
second consent implementation. Current-head CI on e042157 passed Android JVM
and real backend integration, but Quick verification and backend fast both
refused the same unformatted sweep_catchup_test.go. Apply gofmt only: no test
assertion, behavior or gate is removed. The evidence application package passes
`go test ./internal/modules/evidence/application`; tracked Go formatting,
`git diff --check` and `bash scripts/scan-secrets.sh` pass. New CI evidence is
required for the corrected head before guarded integration.
