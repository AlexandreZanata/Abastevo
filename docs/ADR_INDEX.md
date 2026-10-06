# ADR index

Existing Android decisions remain applicable to the imported release. New statuses distinguish planning choices from a shipped/verified system. See [decision log](planning/DECISIONS.md) for unresolved implementation choices.

## Existing

- [ADR-001 Kotlin/Compose stack](adr/001-kotlin-compose-stack.md) — retained Android baseline.
- [ADR-002 Import immutability](adr/002-price-import-immutability.md) — first-import-wins; backend revisions intentionally differ.
- [ADR-003 Nominatim](adr/003-nominatim-reverse-geocode.md) — Android reverse-geocoding scope; not bulk-server provider authorization.

## Added planning decisions

- [ADR-004 Backend scope and inherited engineering rules](adr/004-backend-scope-and-governance.md) — Adopted for planning.
- [ADR-005 One Go monolith and one PostgreSQL/PostGIS database](adr/005-modular-monolith-and-postgresql.md) — Adopted for planning.
- [ADR-006 Official revisions, canonical stations and exact shared contracts](adr/006-official-catalog-and-exact-contracts.md) — Adopted for planning.
- [ADR-007 Anonymous proof and mobile key compatibility](adr/007-anonymous-proof-of-possession.md) — Proposed protocol; freeze in P03.
- [ADR-008 Data minimization and privacy-aware immutable history](adr/008-privacy-and-retention.md) — Proposed policy; review before G09.
- [ADR-009 Immutable observations and conservative current-price projection](adr/009-community-consensus-and-projections.md) — Adopted algorithm proposal; calibrate before pilot.
- [ADR-010 Private object storage and frozen validated evidence](adr/010-private-object-evidence.md) — Adopted for planning.
- [ADR-011 Recoverable VPS deployment and backend completion gate](adr/011-operations-and-android-gate.md) — Adopted for planning.
- [ADR-012 Preserve MIT and separate hosted convenience from community trust](adr/012-licensing-and-hosted-business.md) — MIT preservation adopted; future license undecided.

- [ADR-013 Fast phase delivery and separate release certification](adr/013-fast-phase-delivery.md) — Policy adopted; G01-FLOW automation/activation pending.

## Future ADRs when triggered

- Actual provider/tool version selections when alternatives have material tradeoffs (P01/P02).
- Major money/CNPJ Room compatibility change (P10), with upgrade evidence.
- Account recovery, billing and sync conflict strategy (P11).
- Any licensing change, new public brand/application ID, public API major version or dependency outside the accepted stack.
- Dedicated DB, partitions, replicas, new cache/broker or orchestration only after measured bottleneck/cost evidence.

- [ADR-014 — Functional app before real production release](adr/014-functional-app-before-production-release.md): G09-LOCAL app entry; real G09 deferred until G18.
- [ADR-015 — Shared KMP domain and native Swift adapters](adr/015-kotlin-multiplatform-shared-domain.md): toolchain audit and platform evidence before readiness.

- [ADR-018 — Project batch delivery](adr/018-project-batch-delivery.md): tested local phase checkpoints; CI/PR merge/wiki at final project construction closure.

- [ADR-017 — Station profiles and verified representation](adr/017-station-profile-and-verified-representation.md): private claims, independent corporate authority and scoped permissions.
- [ADR-019 — Android/VPS first](adr/019-android-vps-integration-first.md): P34–P38 live app source integration before catalog/profile expansion; final project batch delivery.

- [ADR-020 — Maintained dev and protected main](adr/020-maintained-dev-main-delivery.md): consolidate historical construction branches, maintain dev → main, preserve incomplete acceptance obligations.
