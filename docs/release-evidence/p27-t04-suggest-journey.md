# P27-T04 — Lightweight suggest-correct-status Android journey

Status: LOCAL_DONE on `codex/phase-27-station-intake`. Task: P27-T04.
Binds B-BR-D06–D09 and BUC-D03/D04/D05. Free-account journey with
search-before-submit, stable offline retries, private status/cancel
and honest guest/denial states. No background permission, no photo
requirement.

## Behavior (TDD)

- `StationIntakeHttpClient` (session-in-body, `no-store` header,
  non-2xx/empty throw, no double `/v1`) + domain
  `StationIntakeGateway` port (DTOs parsed in the data adapter, so
  application stays JSON-free and module boundaries hold — fixed a
  real layering violation during implementation).
- Application intake use cases (submit/status/cancel/mine) mapping
  transport failures to explicit outcomes; `Unavailable` never
  invents acceptance.
- `SuggestStationViewModel` + `SuggestStationScreen`: structured form,
  duplicate-CNPJ hint from the cached catalog (review still dedups;
  corrections stay allowed), one stable id per form across offline
  retries (no duplicate visible station), private list + status +
  cancel, guest gating, revoked-session surfacing. Entry via
  `Routes.SUGGEST` from the Explore server section; legacy flows
  untouched. en/pt-BR strings +18.
- DI: `IntakeModule` (shared origin) + 4 use-case provisions; origin
  guard extended to gateway bindings.

## Validation

- NEW: client 3/3, application intake 3/3, `SuggestStationViewModel`
  4/4 (guest gate, duplicate hint, same-key retry, status/cancel
  refresh) — all PASS.
- Fixed from real failures (not weakened): application→data import
  (domain port extracted), org.json Android stubs under JVM tests
  (same vetted test artifact as `:data`), `coEvery` on non-suspend
  mocks, experimental-Material opt-in, gateway binding filter.
- Regression: full `:domain` + `:application` + `:data`
  (232 tests, 1 pre-existing skip) + `:app` suites + Hilt KSP +
  `:app:assembleDebug` PASS.
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits and next

- Device-level journey proof joins the end manual batch (no emulator
  per directive).
- Next: P27-T05 intake abuse, privacy and lifecycle acceptance.
