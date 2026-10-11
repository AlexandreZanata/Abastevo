# CS06 — Projections isolation

Date: 2026-10-11. Status: LOCAL_DONE. Scope: math pins, isolation
proof, static guards — no route/schema/migration change, no VPS/APK.
Head: maintained `dev`.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md).
Rules: B-BR-CS06.

## Proofs (actual)

- Display math: floor basis-point vectors incl. unanimous, splits,
  1/9999 edges and 2^40 counts — shares never exceed the ratio,
  never 0% with votes, always within 0..10000.
- Dynamic isolation: full helpfulness lifecycle (cast, change,
  second voter, remove, rebuild) leaves `community_current_prices`,
  `feedback_rating_stats` and `community_projection_inputs`
  byte-identical (snapshot comparison around the lifecycle).
- Static isolation (build-failing pins): consensus/projection/feed/
  vote/signal queries reference no helpfulness tables;
  helpfulness.sql references no rating/consensus/projection tables.
- 35-day audit freeze: no DELETE/TRUNCATE path against disputes,
  confirmations, helpfulness votes or observations in owned queries
  (media 24h lifecycle excepted, as designed). The 35-day horizon
  bounds ledger/media retention, never fact audit.

## Checks

Unit module + helpfulness integration green; `go build`, `go vet`
(incl. integration tags), `gofmt -l`, `git diff --check` clean;
`scan-secrets.sh` passed.

## Limits / owed

- Official star methodology stays independent (existing feedrating
  proof cited); social votes remain audit-only review inputs.
- Weighted-tally display work for mobile surfaces: CS08/CS11.

Next microtask: CS07 — benchmark and guardrails.
