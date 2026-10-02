# P24-T02 — Novice usability and accessibility (protocol + first remediation)

Status: IN_PROGRESS on `codex/phase-24-android-acceptance`. Issue: #97 OPEN (not done).
Entry: G19–G23 integrated (P23 PR #95 `b4f664e`); P24-T01 automated loop PAUSED for
user manual validation (`TEST_STAGE_CLOSED_USER_MANUAL`), so no device acceptance is
claimed here. Binds B-BR-C01–C06 / BUC-C01–C05. No migration, no backend change, no iOS work.

## Protocol (frozen before running, from P19-T02 baseline)

- Sample: 3–5 novice drivers (Android daily users, never used abastevo) + 1 maintainer
  observer. No PII collected beyond first name + consent; sessions not recorded.
- Tasks (predeclared, BUC-derived):
  - T1 (BUC-C01): guest selects city + fuel, reads recent community price or explicit
    absence, identifies source (community vs dated ANP) and recency.
  - T2 (B-BR-C01/C06): explains condition label on a loyalty/app-only price (no cheap
    ranking assumption).
  - T3 (BUC-C02): photo contribution review → confirm station/fuel/price/condition.
  - T4 (BUC-C03): rate/comment/reply/vote + report abuse path (280-char bound visible).
- Pass thresholds: T1 ≥ 4/5 first-try source/recency identification; T3 zero silent
  publishes of queued data; T4 zero PII/GPS exposure; screen-reader + 2x font-scale
  rows green (existing `AccessibilityUiTest` 4/4). Failures remediate presentation,
  never domain rules.
- Novice sessions: NOT RUN in this slice (no participants recruited). Protocol above is
  frozen so P24-T02 can execute without redefinition. Maintainer heuristic walkthrough
  below is early evidence only, not a substitute.

## Automated accessibility baseline (this tree)

- Existing instrumented `AccessibilityUiTest` 4/4 preserved (Loading TalkBack,
  Error/Home/Search at 2x font-scale). Device rerun stays P24-T01-row-owned while the
  user validates manually; not claimed here.
- pt-BR coverage verified: `source_badge_*`, `a11y_source_badge_*`,
  `a11y_station_price_row`, `stations_collected_at_label`, `a11y_nav_update_price`
  present in `values-pt-rBR`; `a11y_station_price_row` present in all 7 locales.

## Finding + remediation (this task)

- Finding: `StationPriceRow` announced only `a11y_station_navigate` ("Ir para X") —
  TalkBack lost price + recency, failing B-BR-C01 comprehension. The richer string
  `a11y_station_price_row` ("%1$s, preço %2$s") existed in all locales but was unused.
- Fix (`app/.../ui/components/StationPriceRow.kt`): description is now
  `a11y_station_price_row(displayName, priceFormatted)` + optional localized
  `stations_collected_at_label`, joined by pure `stationRowTalkBackDescription`
  (Role.Button preserved, so the row stays actionable). No new strings, no domain change.
- `SourceTimeBadge` needs no change: text + icon (never color-alone) + per-kind
  screen-reader description already covered by `SourceTimeBadgeTest` 3/3.

## Validation (this slice)

- RED → GREEN: new `StationPriceRowA11yTest` 3/0-fail (price-only, blank-recency,
  recency-appended).
- `:app:testDebugUnitTest` target + `:app:assembleDebug` (record outcome in commit
  evidence); `git diff --check` clean; scoped secret review (no secrets/PII/GPS/photo).
- Open for phase exit: novice sessions T1–T4, instrumented rerun on frozen P24-T01
  rows, low-end hardware row (P24-T03-owned).
