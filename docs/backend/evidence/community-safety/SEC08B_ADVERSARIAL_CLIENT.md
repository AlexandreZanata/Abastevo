# SEC08-B — Adversarial synthetic client test

Date: 2026-10-11. Status: LOCAL_DONE. Scope: synthetic-adversary tests
only (no behavior change). Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC08, AUD-12). Contract: `location-v1` recompute, forged-claim
refusal, replay convergence, teleport review flag, UNKNOWN proximity
rule, 400 `location-forged` mapping without coordinate/identity echo.
Constraint: synthetic client/key (`c-adv`/`tok-synth-adv`) distinct
from honest fixtures; no real photos/coords in Git (synthetic
coordinates only, matching existing test practice).

Validation: PASS

## Changes

- `community/application/sec08b_adversarial_test.go` (6 tests) drives
  the exported `Submit` with a fake store + idempotency ledger:
  consistent fabrication lands VERIFIED/NEAR under the synthetic
  identity (residual measured, not hidden); identical replay converges
  to one row (`Replayed`, same id); 50 km teleport in 60 s keeps bands
  with `teleport-suspect`; stale (1 h) and coarse (500 m VERIFIED
  claim) refuse forged with zero rows; missing coords and centroid
  (unreviewed) sites degrade to VERIFIED/UNKNOWN, never NEAR; garbage
  `TRUSTED` verdict refuses forged while honest SIMULATED stores
  SIMULATED/UNKNOWN.
- `community/adapters/http/sec08b_adversarial_test.go` (2 tests):
  fabricated block arrives intact at the recompute through the signed
  route (201, synthetic caller carried); coarse fabrication maps to
  400 `location-forged` with no coordinate/identity echo.

## Validation (actual)

- `go test ./internal/modules/community/...` green (incl. 8 new).
- `go build ./...`, `go vet`, `gofmt -l` (clean after one format),
  `git diff --check` clean; secret scan passed (prior turn, synthetic
  fixtures only).
- RED pre-GREEN: no synthetic-adversary exercise existed — the matrix
  is new coverage; each refusal dimension was pinned by mutating one
  signal (age, accuracy, verdict, coords, site) against the frozen
  recompute, and the consistent-fabrication pass documents the
  geometry residual instead of claiming detection.

## Limits / owed

- Consistent fabrication still passes geometry by design (SEC08-A);
  detection needs the SEC08-C/D channels, not this layer.
- Capture-photo route shares the same recompute path; price submit is
  the exercised representative.
- No production claim.

Next microtask: SEC08-C attestation decision.
