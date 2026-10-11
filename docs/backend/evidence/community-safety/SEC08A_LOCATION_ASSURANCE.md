# SEC08-A — Location assurance policy

Date: 2026-10-11. Status: LOCAL_DONE. Scope: policy document only —
no code, contract, or threshold change. Head: maintained `dev` + this
task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC08, AUD-12). Contract: `location-v1` frozen verdicts/reasons/bands
(`backend/internal/modules/community/application/location.go`);
server recompute (`location_verify.go`); 150 m capture geometry
(`PhotoCaptureRadiusM`); proximity bands (`signals.go`/`derive.go`).
Constraint: self-declared metadata alone never manufactures an
independent HIGH-trust signal; user-facing geometry and honest
unknown/manual outcomes preserved; any future contract change versions
the policy and ships fixtures first.

Validation: PASS

## The five assurance layers (what each proves)

1. **Request/key authenticity** — proves *which account* speaks (email
   session, key proof, OIDC subject binding). Proves nothing about
   where the device is. A stolen valid session submits from anywhere.
2. **OS mock detection** — the platform-marked simulation flag
   (`isMock`/`isSimulatedBySoftware`) proves the OS *caught*
   simulation, never that absence of the flag means genuine GPS. Root,
   hooks, and flag-stripping clients bypass it silently.
3. **Coordinate geometry** — proves the claimed point lies within 150 m
   of a station point *if both numbers are true*. Geometry cannot
   authenticate its own inputs; fabricated coordinates pass geometry
   exactly like genuine ones.
4. **Station-point quality** — proves the station side is a reviewed
   projection (not TEST_ONLY/synthetic). A genuine fix at an
   unreviewed point still cannot enter public consensus.
5. **Physical presence** — unproven by any combination above. Presence
   needs an independent channel (attended review, corroborating
   observations); SEC08-C records the measured attestation decision.

## Why self-attestation cannot yield HIGH trust alone

- The server recomputes the verdict from claimed metadata on the
  server receipt clock (`VerifyDeviceFix`): freshness derives from
  receipt-minus-captured, never client age. A forged `VERIFIED` over
  inconsistent/stale/missing metadata refuses with
  `forged-location-claim` instead of downgrading.
- Missing OS source info caps at UNKNOWN (`source-info-missing`): a
  client-claimed "not simulated" cannot promote itself.
- Only VERIFIED fixes carry proximity (`ClaimForFix` returns nil
  otherwise); honest states store UNKNOWN proximity. MANUAL/DENIED/
  SIMULATED/UNKNOWN preserve browsing, manual lookup, and offline use
  without granting proximity.
- Teleport-speed anomaly routes to review; it never auto-rejects a
  structurally valid observation (no false-presence claim either way).

## Preserved behavior (explicit non-changes)

- 150 m capture geometry unchanged; 2-minute freshness, ±5-minute
  skew, 100 m accuracy tunables unchanged (`location-v1`).
- Honest UNKNOWN/MANUAL outcomes unchanged and reachable in tests.
- No device fingerprint, blacklist, or precise-GPS retention
  introduced (see SEC08-C for the measured decision).
- Contract versioning rule: policy changes bump `LocationV1`,
  update `contracts/testdata/location/risk-v1.json` fixtures, and
  never silently reinterpret old rows.

## Validation (actual)

- Documentation-only: re-read `location.go`, `location_verify.go`,
  `submit_location.go`, `signals.go`, `photo_capture.go`; verified
  the claims above against current source (precedence order,
  recompute-before-trust, nil-claim rule, review-only teleport).
- `git diff --check` clean; no code/test/contract touched, no
  secrets/PII/GPS in this doc.
- RED pre-GREEN: not applicable (policy freeze, no behavior delta);
  adversarial teeth arrive in SEC08-B.

## Limits / owed

- Geometry alone still cannot detect Fake GPS — stated, not solved,
  here. Real-phone matrix is SEC08-D (OWED without a device+window).
- Attestation support/privacy/cost decision is SEC08-C.

Next microtask: SEC08-B adversarial synthetic client test.
