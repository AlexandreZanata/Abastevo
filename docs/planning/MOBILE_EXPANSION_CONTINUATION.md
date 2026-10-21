# Resume mobile expansion after MEX-00–03 / EE-00–02

The foundation [MEX-00 audit](../backend/evidence/mobile-expansion/MEX-00.md) and
the subsequently authorized [MEX-01 registration contract](../backend/evidence/mobile-expansion/MEX-01.md)
and [MEX-02 exploration contract](../backend/evidence/mobile-expansion/MEX-02.md)
are LOCAL_DONE / INTEGRATION_PENDING. The next four-unit request selected
[MEX-03 baselines](../backend/evidence/mobile-expansion/MEX-03.md),
[EE-00 inventory](../backend/evidence/ev-support/EE-00.md),
[EE-01 qualification](../backend/evidence/ev-support/EE-01.md) and
[EE-02 types/numerics](../backend/evidence/ev-support/EE-02.md), now locally complete.
Qualification is not source admission: PBE rights and full-source/runtime/privacy
gates remain pending. No product runtime implementation is accepted here.
This file is continuation guidance, not additional execution authorization.

## Starting point for the next authorized agent

1. Read applicable AGENTS, FAST_EXECUTION, current PROGRESS and DELIVERY_WORKFLOW,
   then the [MEX-00 evidence](../backend/evidence/mobile-expansion/MEX-00.md) and
   [combined execution prompt](EV_SUPPORT_EXECUTION_PROMPT.md).
2. Inspect `.worktrees/mobile-expansion-plan`, `codex/mobile-expansion-plan` and
   actual maintained dev/main before acting. The foundation reconciles original
   plans `47fded8c` / `35accb9e` with dev `a2f8925c`, preserving both histories.
   Do not check out the old plan base as the implementation base. Do not reset,
   stash, cherry-pick an already inherited change or include another agent's edits.
3. Refresh remote refs when available. The foundation's remote check failed DNS;
   no remote current-head/CI acceptance was established. Work on maintained dev
   when available; otherwise isolate and reconcile normally under ADR-020.
4. Next in the combined order is **EE-03**, source-specific preparation/census
   manifests; it must respect the actual EE-01 rights/profile/admission limits.
   **MEX-04** place batching is independently ready. Select one bounded task,
   complete actual acceptance and commit before the next. No schema/UI changes
   before their owning dependencies; required source gates cannot be mocked away.
5. Read the selected unit and linked rules; do not rerun the entire project for
   this audit. Relevant code/config/base changes invalidate affected evidence.
   The [dependency JSON](EV_MEX_DEPENDENCIES.json) has no completion flags.

## Reuse and guardrails that must survive the handoff

- One free vehicle; extra vehicles Premium. Current technical ceiling3 and
  existing professional feature matrix remain until an explicit amendment.
  Preserve old vehicles/history and expired-account data-exit rights.
  [Registration v2](../product/VEHICLE_REGISTRATION_POLICY.md) and its23 worked
  examples are frozen; actual vehicles.multiple grants/quota remain MEX-07/08.
- [Exploration v1](../product/MOBILE_EXPLORATION_CONTRACT.md) has54 shape and
  three counting examples in a separate schema without live endpoints. Reuse it
  for typed intent/source/clock/coverage/company filters and Android wording;
  actual semantics/SQL/HTTP/screens and source acceptance are still unimplemented.
- Reuse PRM cloud/ledger/metrics/subscription and Kotlin ports. Source coverage
  does not close Android journeys or real provider proofs.
- Keep EE capability/unit/tariff/source contracts ahead of dependent migrations
  and Android. PHEV is one vehicle; no fake electricity FuelProduct or zero cost.
  [EE-02 frozen contract](../product/EV_TYPED_DATA_CONTRACT.md) has46 shape and24
  exact Go/Kotlin/Rust examples; numerical oracles are not the runtime EE-11 engine.
- [MEX-03 actual baseline](../backend/evidence/mobile-expansion/MEX-03.md) distinguishes
  the in-memory picker from unused global search; target the real consumer and
  compare pinned workloads. Host SQLite/JVM does not establish phone latency.
- Reconcile OPT commits `f60bb3e2`, `8de22c24`, `6e8b306a` before repeating that
  work. Their persistence/no-store and real SQLite acceptance require review;
  don't blindly accept their phase3 declaration. Allocate Room versions from
  the actual combined database. Preserve Home commit `a2f8925c`.
- Close missing FQ scoped-read acceptance through MEX-18; MEX-20a–f are six
  real-source enforcement units. Current-company association is not historical
  site attribution. No source-global data is suppressed for lack of association.
- D receipts/business portal stays future-only until extreme consolidation and
  thousands of real clients, unless expressly requested earlier for that scope.
  iOS, charging control/payment, provider registration and activation stay held.
- Local source implementation and commits are distinct from publication,
  deployment, device changes and release certification. Follow the human's
  active authorization; this foundation does not publish or authorize those steps.

Deliver each task with actual targeted/risk tests, evidence, concise PROGRESS
and one atomic English commit. Report in Portuguese. Never convert an absent
test, provider permission, source sample or live access into PASS.
