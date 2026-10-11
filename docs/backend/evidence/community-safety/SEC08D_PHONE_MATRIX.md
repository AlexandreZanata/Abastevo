# SEC08-D — Real-phone staging matrix (OWED, no device/window)

Date: 2026-10-11. Status: OWED / PROOF_OWED. Scope: frozen matrix +
entry criteria only — no run performed, no behavior change. Head:
maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC08, AUD-12). Contract: `location-v1` verdicts, SEC08-A policy,
SEC08-B synthetic matrix (local proxy), SEC08-C attestation decline.
Constraint: no authorized device + staging window exists in this
environment; NOTHING is fabricated — the matrix below is the frozen
procedure, all verdict cells empty pending the run. No real
photos/coordinates ever enter Git/logs (record bands/reasons only).

Validation: NOT_RUN (owed)

## Frozen matrix (4 cases × expected verdict)

| # | Case | Setup | Expected |
|---|------|-------|----------|
| 1 | Normal Fake GPS refuses | Real phone, genuine GPS off, Fake GPS app mocking a station point, capture at reviewed station | SIMULATED / `simulated-source`, UNKNOWN proximity, no consensus entry |
| 2 | TEST_ONLY geometry excluded | Isolated TEST_ONLY coordinates submitted at a TEST_ONLY point | No public consensus entry regardless of fix quality (unreviewed site rule) |
| 3 | Genuine location passes | Real phone, genuine GPS, reviewed station, fresh accurate fix | VERIFIED / NEAR (or FAR by distance), consensus-eligible |
| 4 | Airplane/offline capture | Offline capture, reconnect within 24 h envelope | Freshness judged on server receipt math; stale refuses forged, fresh verifies |

## Entry criteria (all required before the run)

- Explicit user authorization naming the device + staging window.
- Staging origin only (`https://teste.abastevo.com.br`); reviewed +
  TEST_ONLY stations pre-identified; account/key fixtures pre-made.
- Evidence capture limited to verdicts/reasons/bands + timestamps;
  photo bodies, exact coordinates, and device identifiers excluded.

## Validation (actual)

- Procedure-only: no run, no device touched, no staging mutated.
- `git diff --check` clean; no code/test/contract touched, no
  secrets/PII/GPS in this doc.

## Limits / owed

- PROOF_OWED: the four verdict cells above. This task stays open
  until the authorized run fills them; SEC09 exit inherits the owe.
- Local synthetic coverage (SEC08-B) is not a substitute.

Next microtask: SEC09-A current-head review.
