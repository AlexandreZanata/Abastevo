# ADR-008: Data minimization and privacy-aware immutable history

Status: **Proposed policy; review before G09**. Date: 2026-09-28. Scope: planned platform; implementation evidence required by roadmap gates.

## Context

Evidence/GPS aid abuse checks but immutable personal history conflicts with erasure/minimization.

## Decision

Separate immutable price facts from expiring private payload and contributor links. GPS/original/OCR cap 24 h, sanitized media 14 days (review extension ≤30), fraud hashes 90 days, observation history 24 months initially; implement rights workflow and restore deletion ledger. Review purposes/legal bases before collection.

## Alternatives

Keep every raw signal forever; delete all audit immediately; treat pseudonymous keys as anonymous data.

## Consequences

Reduced future replay/forensic ability is intentional; disclose limits. Legal review and provider retention settings remain required. Never use immutability to refuse valid privacy deletion.

## Validation and follow-up

Use the owning tasks in [ROADMAP](../../ROADMAP.md), specifications in [docs index](../README.md) and [decision log](../planning/DECISIONS.md). Record tested policy/tool versions before marking a release gate complete.
