# ADR-009: Immutable observations and conservative current-price projection

Status: **Adopted algorithm proposal; calibrate before pilot**. Date: 2026-09-28. Scope: planned platform; implementation evidence required by roadmap gates.

## Context

Latest-write-wins and paid trust are vulnerable and misleading. Reads must remain cheap.

## Decision

Append observations/decisions, separate votes/disputes, one eligible vote/contributor/key, exact-price groups, deterministic versioned consensus with freshness. Query indexed projection and enforce expiry at read time. Payment excluded structurally. Separate validation state, availability, freshness and confidence.

## Alternatives

Mutable station price field; most recent request wins; paid weight; full event sourcing/CQRS infrastructure.

## Consequences

Eventual consistency and conservative unknown/disputed outputs. Sybil resistance requires quotas/evidence/review. Full policy and golden examples in COMMUNITY_PRICING_SPEC; tuning changes policy version.

## Validation and follow-up

Use the owning tasks in [ROADMAP](../../ROADMAP.md), specifications in [docs index](../README.md) and [decision log](../planning/DECISIONS.md). Record tested policy/tool versions before marking a release gate complete.
