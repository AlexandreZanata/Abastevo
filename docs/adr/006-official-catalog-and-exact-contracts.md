# ADR-006: Official revisions, canonical stations and exact shared contracts

Status: **Adopted for planning**. Date: 2026-09-28. Scope: planned platform; implementation evidence required by roadmap gates.

## Context

Android preserves first import, rounds money to two decimals, uses numeric CNPJ and legacy PREMIUM naming. Backend must ingest corrections safely.

## Decision

Server-side revisioned ANP ingestion; station UUID separate from CNPJ; accept verified numeric/alphanumeric formats. Integer milli-BRL and explicit unit; preserve raw official decimals. GASOLINE_ADDITIVED is target terminology, legacy mapping explicit. Shared versioned fixtures carry intentional Android differences until P10.

## Alternatives

Copy Android parser semantics blindly; overwrite previous official prices; discard local ANP import.

## Consequences

Extra revision/provenance storage; fixtures prevent silent divergence. Precision beyond contract quarantines instead of rounding. No Android source/schema rewrite before G09.

## Validation and follow-up

Use the owning tasks in [ROADMAP](../../ROADMAP.md), specifications in [docs index](../README.md) and [decision log](../planning/DECISIONS.md). Record tested policy/tool versions before marking a release gate complete.
