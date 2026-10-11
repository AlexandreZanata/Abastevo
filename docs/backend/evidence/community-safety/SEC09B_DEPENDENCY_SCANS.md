# SEC09-B — Dependency and container advisory scans

Date: 2026-10-11. Status: LOCAL_DONE. Scope: scans + version records
only (no dependency change). Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC09). Contract: zero new dependencies in the SEC00–SEC08 batch
(verified by empty dep-file diff since `1b67367`); every image
digest-pinned; release keeps exactly one exported activity.
Constraint: findings would block SEC09-E; scans run at this head.

Validation: PASS

## Results (actual, head `70748d9`)

- **Go** (`backend/`, toolchain `go1.27.2`): `govulncheck ./...` →
  **No vulnerabilities found.** Resolved set recorded via
  `go list -m all` (chi v5.3.2, pgx v5.11.0, testify v1.11.1,
  x/sync v0.23.0, x/text v0.42.0, full list reproducible from
  `backend/go.mod` + `go.sum`).
- **Rust** (`tools/station-prep`, 25 crates): `cargo audit` → exit 0,
  no advisories (1296-advisory DB loaded, lockfile pinned).
- **Android** (`gradle/libs.versions.toml`, no audit plugin vendored):
  resolved catalog pinned — AGP 8.7.3, Kotlin 2.0.21, coroutines
  1.9.0, compose BOM 2024.12.01, room 2.6.1, hilt 2.53.1, okhttp
  4.12.0, jsoup 1.18.3, work 2.10.0, poi 5.3.0, SDK 35/min 26.
  Dep-file diff since `1b67367` empty: no new dependency needs a
  license/security record in this batch.
- **Containers**: postgis `18-3.6@sha256:60f6ad…`, caddy
  `2.10.2-alpine@sha256:4c6e91c6…`, golang
  `1.27.2-bookworm@sha256:5cf287a7…`, distroless
  `static-debian12:nonroot@sha256:afa5c872…`, minio + mc digest-pinned
  in `compose.validation.yml`; staging images never `latest`
  (`ANPFUEL_RELEASE` required).
- **Release surfaces**: no `pprof`/debug route in `cmd/` or
  `platform/httpserver`; release `minify+shrink`, single exported
  `MainActivity` (launcher + `anpfuel://auth/callback` deep link),
  FileProvider non-exported, cleartext off; Room DB excluded from
  backup/transfer (SEC05-D).

## Validation (actual)

- Scan-only: `govulncheck`, `cargo audit`, manifest/diff reads above.
- `git diff --check` clean; no code/test/contract touched, no
  secrets/PII.

## Limits / owed

- Android has no vendored advisory gate (Gradle plugin absent);
  catalog is pinned and unchanged — a dependency-bump task re-scans.
- SBOM-by-record (mod/sum/lock/catalog/digests above); no syft
  artifact produced in this scope.
- No production claim.

Next microtask: SEC09-C integration and fault evidence.
