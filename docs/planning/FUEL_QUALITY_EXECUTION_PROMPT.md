# Fuel quality implementation continuation prompt

Status: local planning handoff, 2026-10-14. Implementation is PLANNED.
Read the [implementation plan](FUEL_QUALITY_IMPLEMENTATION_PLAN.md) and
[target contract](../product/FUEL_QUALITY_TARGET_CONTRACT.md). This prompt does
not select implementation, publish or authorize live imports by itself.

## Copyable implementation prompt

> Implement the next explicitly selected FQ task from the fuel-quality plan,
> starting with FQ-00 if no task evidence exists. Read AGENTS.md, FAST_EXECUTION,
> current PROGRESS, its ROADMAP entry, DELIVERY_WORKFLOW at opening and only the
> relevant source/tests. Inspect actual Git history/status and source/dependency
> evidence; preserve occupied work and isolate when necessary. Work sequentially
> on maintained dev synchronized with main; do not resume an old national
> importer plan or treat registry/location evidence as fuel-quality acceptance.
>
> Document/adopt the task's B-BR-FQ/BUC-FQ contracts before behavior. Reuse existing
> Rust station-prep, Directory UUID/full-CNPJ ownership, Go modular monolith,
> worker, proof, privacy and private media boundaries. Preserve the existing
> coordinate stream; add separate lossless quality artifacts with explicit
> source/edition/hash, sample/assay identity and quarantine. No fuzzy negative
> event linkage, invented source fields, unverified purchase labels, arbitrary
> score thresholds, public documents or implicit station/location writes.
>
> Apply deterministic RED → GREEN → REFACTOR and immediate affected negative,
> failure, replay, concurrency, real-PostGIS/S3 and migration recovery checks.
> Record exact executable commands when new packages/tests exist. A profile or
> compile success is not data coverage, score calibration or live acceptance.
> Keep official/community facts and payment/trust meanings separate.
>
> Stop at one tested, documented atomic task checkpoint, or advance only through
> the already authorized construction scope. Update concise PROGRESS/evidence
> with tested revision and actual results/owed obligations. No bulk source fetch,
> VPS/device mutation, issue creation, deployment, PR/merge/wiki or numeric
> publication follows from this planning handoff. Reuse explicit session scope
> when those actions are actually authorized; protect main and retain dev.

## First deliverable

FQ-00 produces a source/contract gap inventory and current-head integration map,
including actual PMQC sample/assay identifiers and the repeated month-resource
link. It does not start a new crate, migrate a database, download all years or
choose a score formula before the owning contract/calibration tasks.
