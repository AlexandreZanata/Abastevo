# EV planning and research validation

Date: 2026-10-20, America/Cuiaba.
Status: LOCAL_DONE
Validation: PASS
Scope: source research and documentation amendment only. No runtime implementation,
source import, account/contact/purchase, provider proof or activation/deployment.

Prepared on the clean owned codex/mobile-expansion-plan worktree from 47fded8c.
Existing other worktrees/dirty dev/primary changes were not edited, stashed or
included. This amends the earlier MEX plan rather than replacing it or publishing
a parallel implementation. Existing PRM/FQ/OPT IDs and user scope holds remain.

Deliverables: 25 source/data families (23 external, two first-party), 24 EV rules,
12 journeys, 12 proposed financial vector groups, 32 EE execution units, combined
MEX dependency splice and prompt. Source rights/access levels are not silently
approved by an indexed webpage or a protocol. No claim of an exhaustive worldwide
source list, full Brazilian inventory or live operator coverage was made.

Research checked primary official/publisher pages, selected PBE PDF text/screenshot
and relevant provider docs/coverage/license metadata. Findings include Brazil
static-only marking in the inspected TomTom matrix, OCM mixed licensing/Brazil
experimental export, ODbL obligations including ANEEL tariff metadata, Google
storage restrictions, commercially gated PlugShare, and differing PBE label/URL/
internal-update semantics. Raw bytes/hashes/full-source oracles/API credentials
were not validated here; those are mandatory implementation gates where applicable.

## Executed documentation checks

- Scoped Python audit PASS: 32 unique ordered EE task IDs, mandatory dependency/
  rule/journey/risk/deliverable/acceptance/recovery fields, 24 unique business rules,
  12 journeys, 25 source entries and 12 financial vector groups.
- Dependency metadata PASS: 78 unique nodes, 77 execution units plus aggregate
  MEX-20; every reference resolves, one recorded order includes every node once
  after its prerequisites, and no cycle exists. All required EE units reach
  MEX-39; optional EE-17 is absent from core-acceptance ancestry. EV model gates
  precede local vehicle work, EE ranking precedes master UI, and all six actual
  enforcement children precede MEX-20 acceptance. External FQ/PRM/OPT gates still
  require actual evidence; this graph does not accept their runtime state.
- Relative-link audit PASS: 51 local link occurrences across seven new files and
  introduced amendment/navigation lines; all targets exist. No local anchor links
  were added. External URLs are research references with explicit access limits,
  not automated current-feed or licensing acceptance.
- Independent Decimal arithmetic checks PASS: Wh/kWh/MJ conversion, energy/session/
  parking totals, valid-window ratios, time/grace fee example and PHEV monetary
  aggregation. This validates worked document arithmetic, not runtime engines.
- New-file whitespace/high-confidence secret-signature review and git diff --check
  PASS. Manual changed-surface review: no raw customer/source rows, CPF, account
  secrets, live keys, production photos or private charging/home coordinates.
- Manual architecture/scope review PASS: retained fuel enums/IDs, pure local-file
  Rust, explicit source/license/census/clock boundaries, real Go admission, scoped
  local generations, no fake live data or lossless-battery inference, source/
  community/private separation, preserved Premium/free/data-exit rules and D/iOS/
  source/provider/publication holds. MEX identifiers were not renumbered.

No runtime suite is appropriate for documentation alone; future test descriptions
are obligations, not tests run by this planning task. No runtime source, machine
policy, dependency, migration, database, phone or provider account was changed.
