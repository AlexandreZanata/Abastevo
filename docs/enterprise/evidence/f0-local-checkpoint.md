# Enterprise F0 local checkpoint

Status: LOCAL_DONE
Validation: PASS

Date: 2026-11-11. Base revision: `d8bdc642411820bbd0a2357c143f8fa7c92cbdd4`,
the synchronized fetched main/dev head. This record travels with the tested
F0 source tree. Branch: `codex/enterprise-f0-f1`, isolated because the local
dev checkout retains three unrelated commits. Delivery to maintained dev
and its protected main PR remains pending.

## Accepted scope and completed local work

The owner accepted the organization boundary only for private Abastevo
Empresas data, the panel at `/empresas/`, Android partitioning by account,
origin and organization, synthetic C1 receipts, and local F1 after M0
closure and integration of the current ADR-019 source batch.

All 21 backlog files now define the original 348 tasks. Ten missing design
documents were reconstructed from surviving requirements, shared rules,
domain/state models and acceptance scenarios. Eighty-seven traceability
rows were reconciled. The original identifiers and dependency graph are
preserved. The generated screen-index defect was reproduced by a failing
test and fixed without suppressing unknown references or canonical duplicates.

## Executed validation

- `python3 docs/enterprise/tools/check_plan.py --against "$task_skeleton" --files ...`
  matched all 348 original task metadata records. The original skeleton
  was recovered from the project planning scratchpad and checked against
  the pre-generation index.
- `python3 -m unittest discover -s docs/enterprise/tools -p test_check_plan.py`
  passed all three regression tests after reproducing two failures.
- `python3 docs/enterprise/tools/check_plan.py --write` followed by check-only
  passed with 348 tasks, 545 business rules, 167 use cases, 145 screens
  and zero problems.
- A second generation produced identical Markdown hashes. Local links
  and heading anchors, required task fields, graph, traceability,
  new-file whitespace and added-content secret patterns passed review.
- `git diff --check` passed.

## Verified F1 source entry

Fetched main and dev are equal at the named base. P38 checkpoint
`f005f9f8`, catalog checkpoint `de3d207a`, profile handoff `cbddfc45`
and merged integration `8078fae7` are ancestors of that head.

GitHub reports PR177 merged to main as `8078fae7`. Its head
`f443a6c73583917465675f039f3ec28364810110` has successful
`Quick verification`, `fast`, `integration` and `test` checks.
Main protection has strict `Quick verification` and admin enforcement.
[Integration PR](https://github.com/AlexandreZanata/Abastevo/pull/177)

## Retained obligations

The legal package is prepared, with real-receipt intake and category-specific
retention still requiring review. Managed credentials, Stage D, source
rights, billing/provider activation, actual device acceptance and G09
retain their owning gates. The integrated P38 acceptance record remains
partial and names owed device evidence. F0 acceptance and source ancestry
do not replace those results.

Next bounded task: EMP-PLAT-001, the pure organization identifier and explicit
scope contract plus repository/module registration conventions. It introduces
no database, route, provider or live-data activation.
