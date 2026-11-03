# EVC-10 staging acceptance

## EVC-10A publication recovery rehearsal

B-BR-EVC-10A. An owner can hold or resume national publication without
changing the visible generation. An explicitly approved rollback restores
the active generation's recorded predecessor, or the pre-catalog state
for the first generation, and leaves publication held. The operation
checks both digests under the publication lock. Preserve past activation
records, source data and licence offers. Audit each effective control
change in the same transaction.

BUC-EVC-10A. In disposable PostGIS, activate two synthetic generations,
hold and resume publication, reject stale control commands, and roll
back each generation in order. Verify the current pointer, hold state
and immutable audit. Use the same database target restrictions as the
national activation command.

Local result: the RED build failed on the missing control contract. With
migration 155 applied to disposable PostGIS, the affected Go suites with
race detection passed. The control test covered a held generation, stale
resume, incorrect restore digest, two concurrent rollback attempts with
one committed result, and restoration to the pre-catalog state. Activation
history stayed intact and the four effective control events were retained.
Three existing optional Rust/prepared-batch parity tests skipped because
their fixture paths were unavailable. Go vet, staticcheck, whitespace and
secret review passed. Staging acceptance remains pending.

Operator interface: `charging-catalog control --action hold|resume|rollback
--expected-sha256 DIGEST --actor OWNER --reason REASON`. Rollback also
requires `--restore-sha256 PREDECESSOR`; use `none` for the pre-catalog
state. A lost response requires checking the pointer and audit before
issuing another command.

## EVC-10B signed operator executable

B-BR-EVC-10B. Ship the national catalog operator executable in the
existing signed worker image. Keep its worker entrypoint and non-root
runtime. Catalog execution remains an explicit owner action.

BUC-EVC-10B. Verify the worker target includes a static catalog operator
built from the same source revision as the worker. It is available for
a separately authorized run with private configuration and pinned files.

Local result: the worker target lacked the catalog executable before the
change. The exact Go 1.27.3 static build flags produced a Linux/amd64
executable, which refused a production target when invoked. All 11
security harness checks passed. The release workflow builds and signs
the worker target. Full image build and digest verification are pending
the GitHub release and staging acceptance.
