# ADR-004: Backend scope and inherited engineering rules

Status: **Adopted for planning**. Date: 2026-09-28. Scope: planned platform; implementation evidence required by roadmap gates.

## Context

The imported app is offline and generic rules demand tenants/auth on every request and events-only interaction. The user explicitly requires backend/infra before Android improvement.

## Decision

P01–P09 backend/infra, G09 gates Android work. Public reads are anonymous/cacheable where inputs are nonsensitive; contributor proof on writes/owner reads; restricted admin. No tenant concept. Explicit application interfaces allowed; domain unit tests pure, real DB integration tests required. Business append-only rules permit legally required privacy deletion.

## Alternatives

Apply generic tenant/auth rules literally; start Android and backend together; replace old modules.

## Consequences

Avoids needless identity/tenant coupling and user-scope violations. Current Android behavior is preserved; Kotlin fixture implementation waits until P10. Old stack/architecture rules govern Android only; new backend docs govern Go.

## Validation and follow-up

Use the owning tasks in [ROADMAP](../../ROADMAP.md), specifications in [docs index](../README.md) and [decision log](../planning/DECISIONS.md). Record tested policy/tool versions before marking a release gate complete.
