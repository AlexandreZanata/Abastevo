# ADR-005: One Go monolith and one PostgreSQL/PostGIS database

Status: **Adopted for planning**. Date: 2026-09-28. Scope: planned platform; implementation evidence required by roadmap gates.

## Context

A small team needs clear ownership and dependable jobs without multiple operational systems.

## Decision

Go net/http/chi/pgx/sqlc/slog; API/worker/migrator from one codebase/release. PostgreSQL/PostGIS is authoritative. Explicit module ports/SQL ownership; queue rows inserted with business facts and claimed with leases/fencing. No external broker/cache initially.

## Alternatives

Microservices; managed proprietary backend; Redis/broker queues; generic ORM.

## Consequences

Simple deploy and transactions, but one DB/VPS creates a failure domain. Dedicated database/replicas later require measured justification. Queue correctness still requires idempotent effects and real concurrency tests.

## Validation and follow-up

Use the owning tasks in [ROADMAP](../../ROADMAP.md), specifications in [docs index](../README.md) and [decision log](../planning/DECISIONS.md). Record tested policy/tool versions before marking a release gate complete.
