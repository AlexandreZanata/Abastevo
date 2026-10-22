# Go 1.27.3 security and toolchain reconciliation

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-21. Parent: 550c5e1294c61e9b93cb84a0e842513535474fc6.
Delivery: existing protected dev -> main PR #140; no deployment or tag.

## Before behavior

The approved feed delivery failed Quick verification/fast at GO-2026-6809
in crypto/x509.Certificate.Verify, reached by station-profile verification.
The official stable release index on 2026-10-21 lists Go1.27.3 as latest;
the Go vulnerability report identifies it as the fix for the1.27 series.
Sources: https://go.dev/dl/?mode=json and
https://pkg.go.dev/vuln/GO-2026-6809.

Risk: critical certificate verification and build provenance. Follow
AI_ENGINEERING_CONTRACT, CI_PLAN and DELIVERY_WORKFLOW. Keep the Go1.27
language level and dependency set. Update the exact production toolchain,
CI runtime, Docker builder digest and executable smoke/recovery pins as a
single reviewed change. Staticcheck's existing1.27.1 analysis-only workaround
stays separate from compiled/runtime/vulnerability-scanned binaries.
Retain vulnerability checks and negative security-gate cases; no bypass.

Acceptance: official release/digest verification; existing security mutant
harness plus stale-toolchain refusal; affected gate harnesses, full backend
unit/race/PostGIS and certificate/auth tests on the new runtime, static/
sqlc/OpenAPI/security checks, image build/smoke, source/secret diff review.
Run the local aggregate once through guarded finish only after all required
remote checks pass. Existing Android/native evidence can be reused only for
unchanged app inputs; this update is not production certification.

Recovery: preserve the validated source; refuse integration if any required
check fails. Never downgrade to a known vulnerable toolchain or suppress the
scanner. Keep other worktrees and unpublished expansion commits isolated.

## Delivered and actual acceptance

- Official release index confirmed latest stable Go1.27.3. Exact runtime
  pins now agree in go.mod, Quick/backend workflows, smoke/load/restore
  commands and current tooling docs. The language level remains1.27;
  go.sum and the application dependency set are unchanged.
- Official Docker Registry manifest for `library/golang:1.27.3-bookworm`
  returned digest `sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5`.
  Its exact response bytes independently hash to that header; saved locally
  at `/tmp/bfp-go1273-official-image-manifest.json`. The builder is pinned
  to both version and digest, retaining the existing nonroot runtime images.
- Actual initial static RED: changing only go.mod exposed the obsolete
  security guard (`/tmp/bfp-go1273-static-red.log`). Updated guard and
  generic image-digest mutant now pass; added explicit old-vulnerable-
  toolchain refusal. `scripts/tests/test-security.sh`:11/11 PASS,
  including live `govulncheck ./...` on Go1.27.3, zero vulnerabilities
  reported (`/tmp/bfp-go1273-security.log`). No scanner suppression.
  Independent saved symbol-scan JSON confirms `go_version=go1.27.3`,
  scanner1.8.0, database modified2026-10-21 and zero finding events:
  `/tmp/bfp-go1273-govuln.json` (successful exit).
- `go test -race -count=1 ./...` PASS across all backend unit packages,
  `/tmp/bfp-go1273-unit-race.log`. `go vet ./...`, existing pinned
  staticcheck workaround, `sqlc vet` and `sqlc diff` PASS; individual logs
  `/tmp/bfp-go1273-{vet,staticcheck,sqlc-vet,sqlc-diff}.log`.
- Full real integration executed with a unique tmpfs PostGIS/S3 Compose
  project, production migrations and all four real Rust preparers:
  `go test -json -race -count=1 -p 2 -timeout 10m -tags=integration ./...`.
  112 packages passed; only cmd/worker failed because Cargo followed the
  operator umask and emitted mode775 CLIs. The bridge correctly refused
  those group-writable executables. After mode755 normalization, the entire
  worker integration package passed (6.382s) with the same Go1.27.3 code;
  `/tmp/bfp-go1273-worker-recheck.log`. This was an environment correction,
  not a retry of unchanged failing inputs or a weakened trust gate.
  No unresolved executed-test failure remains in this scoped matrix.
- Corrected the existing full-local harness to build/export all four
  preparers in its own temp directory and set their trusted executable
  modes; it previously built only registry and could inherit missing/writable
  quality/fine CLI inputs. Those exact preparation commands and actual
  native worker paths were exercised above; the unrelated full load/restore/
  soak script is not claimed as re-executed in its entirety.
- Integration report: `/tmp/bfp-go1273-integration.sQOGIT/`, with failed
  first-attempt results retained, not overwritten. Four explicit opt-ins
  remain separate: evidence soak, vector regeneration, Android consent
  cross-runtime harness and real PMQC methodology. Missing opt-ins are not
  claimed as proof;26 packages contain no tagged tests. The fixture project
  was removed by its cleanup trap; no shared dev/VPS database was selected.
- API, worker, migrate and station-load images all built PASS. Extracted
  API build metadata reports `go1.27.3`; runtime user is65532:65532. Actual
  migration image and API live/ready returned200 against another new tmpfs
  PostGIS fixture (`/tmp/bfp-go1273-image-smoke.log`). All owned containers
  were removed. Image build logs and binary metadata remain under
  `/tmp/bfp-go1273-image-*`; no registry push/deploy occurred.
- Infrastructure mutation harness7/7, CI cadence2/2 and OpenAPI lint PASS.
  Shell syntax, diff/secret review PASS. Android code/config/APK inputs are
  unchanged; prior feed JVM/native evidence is reused only for those inputs.

Integration remains pending at the new commit. Publish to existing dev PR140,
require current-head Quick verification/fast/integration/test and review/base/
merge-ref guards, then guarded finish. No local aggregate was run ahead of
pending CI. Wiki publication follows actual merge SHA once; no release,
provider, source or production readiness is inferred. The formerly paused
heartbeat stays paused while this human-requested audit proceeds.
