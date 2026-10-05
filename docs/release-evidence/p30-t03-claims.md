# P30-T03 — Account-bound claim and one-use declaration

Status: LOCAL_DONE on `codex/phase-30-station-profiles`. Task: P30-T03.
Binds B-BR-P03/P04/P07/P09/P16 and BUC-P02/P03. Private requests with
server-bound exact authorization content; one active declaration;
competing cases stay private; no grant from submission.

## Behavior (TDD, critical-first)

- Migration `000037_profile_claims.sql` (append-only): private claims
  (account/station/operator/role/scopes/policy/state, per-owner
  idempotency key) + versioned declarations (nonce/expected digests,
  exact text, TTL, attempts, states) with nonce uniqueness.
- Domain: open-claim states, single-active-declaration rule,
  frozen roles/scopes matrix (no OWNER), ceremony limits (256-bit,
  30-min, 5 attempts, 3 open claims).
- Application: `OpenClaim` (operator link recorded when known,
  unknown left pending-never-inferred; replay returns the identical
  declaration so idempotency survives; changed request conflicts;
  quota enforced pre-write), `ReissueDeclaration` (supersede in one
  guarded step), owner-only `ClaimStatus`/`CancelClaim`/`ListMine`
  (foreign → not-found, terminal → closed).
- Adapters: PG stores, claim HTTP routes (open/mine/status/reissue/
  cancel, session-in-body, `no-store`, 404-shared), `cmd/api` wiring
  with session + operator closures (modules never cross-read).
- OpenAPI 5 claim paths + `RepresentationClaim` schema (`vacuum`
  PASS after fixing a merged `post:` line, `apicontract` +
  `check-compat.sh` PASS).

## Validation

- Unit (`-race`): domain states/roles, application open/replay/
  conflict/quota/auth/role + reissue versioning + owner isolation —
  PASS.
- Integration (real PostGIS, `-race`): open→reissue(v2, new nonce)→
  status→cancel→cancelled with competing-claim privacy, parallel
  same-key convergence (1 created) and owner isolation. Fresh
  disposable DBs (migrations incl. 000037).
- Fixed from real failures (not weakened): partial-conflict index
  predicate, replay JSONB normalization (semantic compare), missing
  policy default, non-UUID test ids, scope plumbing, YAML merge.
- Regression: full `stationprofile` + `directory` unit + integration
  PASS (zero failures); `go vet` clean; `go build ./...` PASS.
- `git diff --check` PASS; `scan-secrets.sh` PASS (no nonces/keys/
  documents in Git — digests only).

## Limits and next

- Proof intake/verification/decision/grant flows are P30-T04…P33;
  review tooling consumes the frozen ports.
- Next: P30-T04 private immutable signed-document intake and expiry.
