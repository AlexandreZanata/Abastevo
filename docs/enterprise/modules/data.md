# Abastevo Data

Status: **PLANNED / RECONSTRUCTED**, 2026-11-11.
Recovered from the surviving enterprise requirements, shared rules and backlog.
The accepted F0 decisions and existing release gates govern any implementation.

## 1. Purpose

Serve reproducible admitted datasets, methodology and analysis to an organization through scoped product access while preserving public basic access and excluding private company records.

## 2. Scope

F8 includes catalog, regional series, watchlists, comparisons, reports, API keys and metering. Commercial redistribution requires recorded rights per source under GOV-016. Private receipt, employee, agreement and track data are excluded from market products.

## 3. Entities and ownership

Dataset definition records source, coverage, method and version. The organization owns its API keys, usage, watchlists and report requests. Key secrets are protected and only safe metadata is redisplayed. A dataset is not the organization's private operational database.

## 4. Business rules

- **BR-DATA-001 — Recorded admission.** Availability on a public site is not recorded redistribution authority. Each dataset requires its source-specific rights decision.
- **BR-DATA-002 — Exclude private inputs.** Company, employee, document, agreement and location records never enter a market dataset by default (BR-SRC-005).
- **BR-DATA-003 — Reproducible basis.** Series, comparisons and reports include origin, period, coverage and methodology revision (BR-TRUTH-004).
- **BR-DATA-004 — Current key scope.** Issue, rotation, expiry, revocation and quotas preserve the organization boundary and server authorization.

## 5. Use cases

**UC-DATA-01 — Query an admitted series.** A currently authorized key requests an enabled dataset and regional period. The service checks rights admission, key scope and quota, then returns version-bound paginated data and basis. Missing input remains explicit.

**UC-DATA-02 — Rotate a key.** A fresh permitted organization actor issues an approved replacement. The recorded overlap ends at its stated time and the old key is refused afterwards. Audit contains no secret.

**UC-DATA-03 — Request a comparison.** A member selects an admitted report and permitted scope. Comparable sources produce a method-labelled result. Otherwise the result explains insufficient or unsupported data rather than guessing.

## 6. States and transitions

Keys follow active, rotating, revoked and expired states in [the state catalog](../08-state-model.md). Revocation applies to the next request. Report files use the existing export lifecycle and private expiry. Rights-pending admission remains a hold on the affected dataset regardless of subscription state.

## 7. Roles and permissions

The [Data permissions](../07-permissions.md) separate key management, consumption and report requests. Key scope cannot widen the organization's admitted capability. Staff account status and personal Premium give no Data key-management rights. Full private organization exports follow the distinct reporting contract.

## 8. Offline, concurrency, idempotency and corrections

API requests, key changes and report generation require server confirmation. Stable query basis and cursors prevent page mixing. Usage and retry identity are explicit. Notification subscriptions and report links recheck current recipient scope.

## 9. Events, notifications and integrations

Reuse enterpriseexport source preparation and typed admitted read ports. Usage and report jobs use the existing queue. External source updates never rewrite a stored method snapshot. Watchlist notices contain public-source metadata and safe scoped links.

## 10. Data, formulas and examples

A response states product and units, region, observation period, source coverage, method version and missing-data reasons. No unsupported aggregation fills gaps or mixes price conditions. An aggregation threshold needs a recorded review before use. No private row becomes market-safe merely by removing its name.

## 11. Screens

| ID | Surface | Objective | Actor | Data and actions | Validation |
|---|---|---|---|---|---|
| SCR-DATA-01 | PANEL | Manage authorized keys | Permitted organization actor | Safe metadata, issue, rotate and revoke | Fresh authority, one-time secret and visible expiry |
| SCR-DATA-02 | PANEL | Inspect usage and methodology | Permitted member | Quota period, datasets and coverage | No rights-pending sale claim, scoped totals and five shared states |

## 12. Acceptance, dependencies, risks, phase and microtasks

[EP17](../backlog/ep17-data.md) owns tasks and source gates. VAL-36 tests private-data exclusion across API, jobs and exports. Exercise revoked or foreign key, pagination scope, quota races, unavailable source and rights-pending dataset before activating any Data surface.
