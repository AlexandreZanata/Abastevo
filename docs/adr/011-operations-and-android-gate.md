# ADR-011: Recoverable VPS deployment and backend completion gate

Status: **Adopted for planning**. Date: 2026-09-28. Scope: planned platform; implementation evidence required by roadmap gates.

## Context

The user wants complete backend and infra before app improvements; “complete” needs a bounded measurable definition.

## Decision

Compose/Caddy with API/worker/PostGIS on one VPS, private storage and off-host encrypted backups. G09 requires tested deployment/rollback, restore, security/privacy, load/metrics, catalog/community/identity and contracts. Initial RPO 24 h/RTO 4 h are proposals to verify/accept. Only after that P10 starts; paid products are outside MVP completeness.

## Alternatives

Immediate Kubernetes; Android UI parallel to infra; treating compile success as release-ready.

## Consequences

Single host failure accepted within tested recovery objectives; hosting/domain/provider choices remain operational decisions. Gate cannot be marked by documentation alone.

## Validation and follow-up

Use the owning tasks in [ROADMAP](../../ROADMAP.md), specifications in [docs index](../README.md) and [decision log](../planning/DECISIONS.md). Record tested policy/tool versions before marking a release gate complete.
