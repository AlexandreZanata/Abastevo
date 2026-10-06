# ADR-019 — Android integration with the existing VPS before catalog expansion

Date: 2026-10-05. Status: ADOPTED planning by explicit maintainer request; implementation and remote publication pending.

## Context

The maintainer prioritizes a functional Android app using `https://teste.abastevo.com.br` and rapid construction with final-batch CI/merge. Existing backend modules are integrated, but Android DI still points auth/community/identity/contribution clients at `https://api.anpfuel.example.invalid`. The station detail uses local CNPJ rows and a pending-community card; P22 evidence records missing canonical UUID wiring. The temporary VPS smoke record shows an empty database, no private media storage and no real user pilot. These gaps must be addressed before national sources or business claims can deliver useful Android journeys.

## Decision

- Add P34–P38 for the selected existing Android journeys, ahead of P25–P33. Preserve all phase/task IDs and previously integrated history. Phase numbers identify scope, not execution order.
- Sequence: P34 environment/configuration → P35 Directory/price screens → P36 free-account/social journeys → P37 contributions/private media → P38 source acceptance and end Android manual batch. Then the existing P25/P26/P27/P29 catalog expansion and P30/P31/P32/P33 verified business profiles, final project integration, G09 certification and P10 public pilot. P28/P11 remain optional; iOS remains archived.
- Reuse the deployed API and existing ports/contracts. Backend changes are bounded contract gaps or verified defects required by the current app task, with immediate security/PostGIS/migration tests before consumers. Do not require a new service or full national ingestion before testing app journeys on a small bounded catalog.
- Treat the domain as a staging origin, not production or a location endpoint base. All clients share one explicit origin configuration. Never append `/v1` twice, fix user coordinates to the sample URL, log precise request GPS, disable TLS verification or ship the test environment as production accidentally.
- Build and test modules locally immediately, with TDD/DDD and critical risk gates. Use bounded non-destructive HTTPS probes and synthetic staging fixture identities only when write/deployment scope is authorized. Credentials/VPS topology stay local-only; this planning request does not redeploy, populate or reconfigure the VPS.
- Device/emulator/manual validation stays deferred until all selected construction P phases complete, per the existing user directive. P38/P29/P33 record code-ready and live/device obligations separately. Reuse unchanged evidence and execute the union of affected Android/device acceptance once at the end; no fabricated provider/photo/device acceptance.
- Apply ADR-018: local phase checkpoints/dependency branches; no per-phase CI waits/PR merges/wiki publication. Required current remote checks/reviews, guarded cumulative merge and one wiki mirror happen at final project closure. Public pilot and G09 remain their separately certified gates.

## Consequences

App integration produces observable progress before national source expansion. Empty catalog, unavailable providers/private storage, TLS trust failure or unresolved contracts remain explicit blockers for the affected live flow, never mocked success. A health check is not social/media/auth/capacity certification. The planning state and eight-line continuation prompt are in [ANDROID_VPS_PLAN](../planning/ANDROID_VPS_PLAN.md).
