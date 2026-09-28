# ADR-010: Private object storage and frozen validated evidence

Status: **Adopted for planning**. Date: 2026-09-28. Scope: planned platform; implementation evidence required by roadmap gates.

## Context

Photos can be large, private and hostile; a presigned upload URL can be reused before expiry.

## Decision

Private S3-compatible storage/R2; direct bounded-quota upload to quarantine; worker validates exact downloaded snapshot and writes sanitized bytes to a different server-only key. DB stores metadata. Short admin download authorization, lifecycle and orphan sweep.

## Alternatives

Store blobs in PostgreSQL; API proxy all photos; public bucket; trust client Content-Type/HEAD alone.

## Consequences

Object/DB reconciliation is necessary; bandwidth abuse remains bounded by issuance quotas. Server validation processing still consumes resources. Emulators do not replace actual R2 integration tests.

## Validation and follow-up

Use the owning tasks in [ROADMAP](../../ROADMAP.md), specifications in [docs index](../README.md) and [decision log](../planning/DECISIONS.md). Record tested policy/tool versions before marking a release gate complete.
