# SEC03-B — Transfer-boundary decision

Date: 2026-10-11. Status: LOCAL_DONE. Scope: boundary selection +
unit pins — no provider change, no new transport (SEC03-C owns the
provider-compatible matrix). Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC03, AUD-02). Constraint: no S3 POST policy assumed for R2 (R2
documents POST form uploads as unsupported); no exact-length PUT
claimed without provider/transport proof.

Validation: PASS

## Decision

Keep direct presigned PUT and enforce at verification (selected):

- Transport cannot cap: PUT signs `content-type;host` with
  `UNSIGNED-PAYLOAD` (`storage/presign.go`; proven by
  `TestPresignDoesNotEnforceSize` — one-byte and 3 MiB ceilings
  mint the same URL). Exact-length signed PUT is NOT selected:
  no provider proof exists in this environment that R2 honors a
  signed length bound, and inventing one would be a false ceiling.
- Enforcement lands where bytes are actually read, exactly once:
  worker `Verify` downloads `MaxBytes+1` (overflow detectable,
  never silently truncated), `media.Snapshot` caps, the claimed
  SHA-256 must match constant-time, dimensions/inspection and
  sanitization gate promotion, and exactly one download binds the
  outcome (TOCTOU-safe). Oversize/substituted/missing objects
  never reach the server-only final key.
- Admission ahead of transfer (SEC03-A): count + live-byte budgets
  admit atomically, so parallel fresh reservations cannot exceed
  10 rows / 30 MiB per contributor/day.
- Least privilege around transfer: 5-minute URL TTL (1..15 min
  bounds), quarantine-only keys (`q/`, traversal/control refused),
  single-purpose methods, private bucket (no ACL/grant emitted),
  no-redirect transport, 8/16/64 KiB API bodies elsewhere.
- Incomplete writes: S3 PUT is atomic (whole object or nothing);
  interrupted transfers surface as absent objects — transient,
  never promoted, reconciled by expiry/sweep (24 h session TTL,
  quarantine/final cleanup). Repeated PUTs inside the TTL
  overwrite the key but verify binds the reservation claim: same
  bytes converge, divergent bytes mismatch-reject.

## Changes

- `evidence/application/verify_boundary_test.go` (new, 3 pins):
  declared 1 KiB + actual MaxBytes+1 rejects oversize with zero
  final upload; substituted bytes reject checksum-mismatch with
  zero final upload; absent object stays transient (never
  permanent, never promoted).

## Validation (actual)

- `go test ./internal/modules/evidence/...` green (incl. 3 new
  boundary pins; oversize/mismatch/transient/swap/TTL/namespace/
  redirect suites re-green).
- `go vet`, `gofmt -l`, `git diff --check` clean,
  `scan-secrets.sh` passed (synthetic hashes only).
- RED pre-GREEN: boundary pins written against the flaw model
  (declared-widens, substitution-promotes, absence-promotes) and
  observed GREEN on first run — the composition was already
  correct; RED was the missing pin, as recorded (no behavior
  change in this task by design).

## Limits / owed

- Provider-compatible matrix (declared/actual mismatch,
  missing/chunked length, max/max+1, URL reuse, expired/revoked
  session, wrong owner/key/MIME/hash, interrupted transfer,
  concurrent admission over the actual protocol): SEC03-C, small
  synthetic payloads against a real local S3-compatible service.
- Namespace/monitoring (public-prefix exclusion, worker caps,
  failed-object reconciliation, byte metrics): SEC03-D.
- R2 production behavior unverified here; no production claim.

Next microtask: SEC03-C provider-compatible matrix.
