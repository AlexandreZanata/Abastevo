# SEC08-C — Platform assurance decision (measured, introduces nothing)

Date: 2026-10-11. Status: LOCAL_DONE. Scope: decision record only —
no code, dependency, permission, or data collection change. Head:
maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC08, AUD-12). Contract: `location-v1` stays the only location
signal path; client boundary stays OS-flag + fix-metadata reduction
(`AndroidLocationSignals`, P16-T02); server recompute stays the only
verifier. Constraint: NOTHING introduced silently — no device
fingerprint, no developer-settings read, no blacklist, no precise-GPS
retention, no new permission, no new dependency.

Validation: PASS

## Decision: no platform attestation adopted in this scope

Additional platform assurance (Play Integrity, App Attest, SafetyNet
successors) is **declined for the current construction batch**, for
measured reasons — not deferred silently:

- **Proves the wrong thing.** Attestation proves app/binary integrity
  on a genuine device; it does not prove GPS genuineness. A genuine
  device running Fake GPS attests fine. Adopting it would manufacture
  exactly the false HIGH-trust signal SEC08-A forbids.
- **Offline behavior unsupported.** Captures are offline-first with a
  24 h decision envelope; attestation verdicts need connectivity at
  capture time and expire fast. An offline capture can never carry a
  fresh verdict, so the path would bifurcate into attested/online vs
  unattested/offline tiers with no fraud benefit (attackers pick the
  offline tier).
- **Device exclusion cost.** Attestation fails on rooted/custom-ROM/
  no-GMS devices regardless of contributor honesty, and verdicts need
  a Google/Apple backend round-trip per decision window (quota, latency
  and availability coupling on the contribution hot path).
- **Privacy/support cost.** Verdict payloads carry device integrity
  signals the project committed not to collect; support would inherit
  an opaque third-party refusal oracle ("your device failed") with no
  contributor-actionable reason, contradicting the frozen stable-reason
  vocabulary.

## What stays instead (already proven)

- OS mock flag + fix metadata reduced at the Android boundary, server
  recompute as the only verifier, forged-claim refusal, UNKNOWN-first
  degradation, teleport review routing, 150 m geometry at reviewed
  stations (SEC08-A/B evidence).
- Fake GPS handling: normal Fake GPS sets the OS mock flag and refuses as
  SIMULATED; flag-stripping clients face the recompute (freshness/
  accuracy/source-presence gates), not a blacklist.

## Explicitly NOT introduced (verified by grep, 2026-10-11)

- No `Play Integrity`/`SafetyNet`/`App Attest` dependency, API call,
  or verdict field in `application/`, `data/`, `domain/`, `app/` or
  `backend/` (only hit: `LocationModule` KDoc naming, no API).
- No developer-settings read, no mock-app blacklist, no device
  fingerprint, no precise-GPS column/retention, no new Android
  permission (client keeps one-shot last-known reads, no tracking).
- Any future proposal must arrive as its own versioned contract task
  with fixtures, privacy review, offline analysis, and honest
  presence-claim limits — never as a silent addition.

## Validation (actual)

- Documentation-only: verified the absence claims by scoped search
  over client + backend sources; re-read the Android boundary KDoc
  (one-shot reads, no settings/blacklist) and the server recompute.
- `git diff --check` clean; no code/test/contract touched, no
  secrets/PII/GPS in this doc.

## Limits / owed

- This decision binds the construction batch; a future explicit user
  request re-opens it as a new versioned task (presence claims stay
  forbidden until then).
- Real-phone behavior matrix is SEC08-D.

Next microtask: SEC08-D real-phone staging matrix.
