# MEX-37 — Accessibility, compatibility and final scoped union

Status: LOCAL_DONE (source union; device accessibility OWED)
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `4c8f86eb` (maintained dev, MEX-36). Date: 2026-10-23.
Depends MEX-33–36. Tested code head: `887d5a71`; the only later
commits are evidence/PROGRESS Markdown (`54e8a021`, `4c8f86eb`), so the
executed code union is unchanged.

## Union checks

- JVM matrix at `887d5a71`: domain 674, application 454, data 542,
  app 460 = **2,130 tests, 0 failures/errors, 2 pre-existing skips**
  (2,128 executed). Skips are the opt-in live ANP
  listing and training-consent interop tests, not EV paths.
- Lint (`app`, `data`) zero errors; debug APK assembled.
- Go: race unit matrix, vet, staticcheck, sqlc vet, gofmt; real PostGIS
  integration matrix with documented trusted preparers; OpenAPI/contract
  validation and `scripts/check-compat.sh` PASS.
- Rust: 177 tests, fmt and clippy `-D warnings` PASS.
- Locale parity: all 8 locales carry the same new keys; the only
  pre-existing differences are `app_release_apk_url` and
  `home_reference_source`, unrelated to EV.
- Navigation: charging community route is reachable from place detail;
  master charging opens an existing vehicle context; no duplicate tab.
- Compatibility: PostgreSQL append-only chain (derived ≥000109
  expectation, replay no-op) and SQLite 24→25 (6/6) PASS.
- Feature inventory: occurrence UI flag remains build-scoped; paid
  acquisition OFF; charging/official source import and discovery flags
  remain default OFF; reads keep their explicit gates.

## Limits

- Physical TalkBack/font-scale/offline/device navigation and Room
  reopen were not executed; the standing deferral keeps them OWED.
- No production/staging proof, deployment or source activation.
- This is a bounded local union, not G18/G24 or release certification.

Next: EE-30/MEX-38 operational handoff manifest.
