# P20-T01 — Bounded discovery contract and read model

Status: LOCAL_DONE on `codex/phase-20-community-discovery`. Issue: #76. Binds B-BR-C01/C06 and BUC-C01 to existing contracts. No app/backend query implementation in this task; domain query validation only.

## Reused projection (no new read model yet)

- Community-first list reuses `BackendPriceGroup` semantics (`domain/.../model/BackendPriceGroups.kt`): exact money (integer thousandths / `PriceAmount`), `unit`, verbatim `conditionKind`, dated ANP `BackendOfficialSection` as secondary reference only, community projection primary when present.
- Missing community coverage stays explicit (no ANP substitution into the primary price); stale/disputed/unknown follow server rules per `p19-t02-information-architecture.md`.
- Social signals stay separate (B-BR-C04); no votes means no percentage.

## Additive query surface (domain only)

`domain/.../discovery/DiscoveryQuery.kt` + `DiscoverySort.kt`:

- Required: `state` (`BrazilianState`), `municipality` (trimmed, >= 2 chars), `fuelProduct` (`FuelProduct`).
- Optional: `search` station text (blank → null; when present >= 2 chars), `sort` (`PRICE_ASC` default, `RECENCY_DESC`), `page` (0-based), `pageSize` (1..100, default 20). Offset = `page * pageSize`.
- Works without GPS/account/photo; query carries no precise location, account id, photo URL or private fields by construction.
- Distance sort is explicitly out of scope until P20-T02 proves location integrity.

## Index/measurement note

No new backend index or paginated endpoint is added here. Before P20-T02 adds any indexed query, benchmark realistic fixture scale and record the exact query plan per COMMERCIAL_COMMUNITY_PLAN P20-T01. Anonymous reads return only public station/price/source fields; owner-private reads stay behind proof.

## Validation

- `DiscoveryQueryTest` 6/0-fail (domain RED → GREEN).
- `./gradlew :domain:test --no-daemon` (affected module).
- `git diff --check` clean; scoped secret review (no secrets/PII/GPS/photo content).
