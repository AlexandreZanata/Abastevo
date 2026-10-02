# P24 / G24 — user acceptance by explicit decision (evidence waived, not proven)

- Date / decision: 2026-10-02, explicit user request. Manual + emulator/device
  validation happens only after ALL P phases finish; until then only code (JVM/unit)
  tests run. The user directed that phases advance treating that future manual
  validation as valid.
- Scope: P24-T01 (issue #96), P24-T02 (#97), P24-T03 (#98), P24-T04 (#99) advance
  on code slices already committed (`1c4ebd7`, `05b3fec`, `0d960e9`, `011dcca`).
  Issues stay OPEN until the phase PR merges, per delivery workflow.
- What this is NOT: no device/emulator/novice/provider/signature proof exists for
  G24. G24-ANDROID-COMMERCIAL is accepted BY USER DECISION, not by evidence. The
  manual batch (R3.1.3/R3.1.9, FreshInstall networked, low-end row, novice T1–T4,
  cold-start/perf, provider callbacks, signature matrix) is OWED at the end and
  must be recorded before any release claim.
- Downstream: G09 real production stays UNCERTIFIED (needs real deployment +
  provider/TLS/restore/load evidence, separately authorized). P10-T09 pilot and
  P11 remain blocked on certified G09. No deployment/tag/pilot authorized here.
