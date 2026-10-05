# Station profile contract freeze (P30-T01)

Status: FROZEN for P30–P33 implementation, 2026-10-05. Authority:
[representation target](STATION_PROFILE_REPRESENTATION.md) (B-BR-P01–P16,
BUC-P01–P10), [profile plan](../planning/STATION_PROFILE_PLAN.md),
[ADR-017](../adr/017-station-profile-and-verified-representation.md).
Semantics freeze here; wire schemas/migrations/handlers land in
P30-T02…P33 only after this policy. Items marked `[OPEN]` are
explicitly unresolved and block their owning task — never silently
defaulted. No `OWNER` role exists anywhere in this scope.

## Claim ceremony bounds

- Challenge: 256-bit server entropy per declaration; exact declaration
  binds station UUID, full operating CNPJ/revision, claim + account
  reference, requested scopes, purpose, policy version, server
  issued/expiry times. TTL 30 minutes; max 5 proof attempts per claim;
  one active declaration per claim (reissue versions and invalidates
  earlier unused challenges); max 3 open claims per account.
- Declaration template fields (exact, in this order): `station_id`,
  `operator_cnpj`, `operator_revision`, `claim_id`, `account_ref`,
  `scopes[]`, `purpose`, `policy_version`, `challenge`,
  `issued_at`, `expires_at`. Consent text states the station, role,
  scopes and the 24 h photo cap; signing it authorizes verification
  checks, not data sale or marketing.
- Proof intake: PDF only (`application/pdf` by magic bytes), ≤ 5 MB,
  no active content/encryption/embedded executables; original bytes
  preserved verbatim (rasterization/compression destroys evidence and
  is refused). Never request private keys, passwords, PFX/P12 or
  remote signing credentials.

## Roles, scopes and authentication

- Roles: `administrator` {`profile.edit`, `reply.official`,
  `invite.propose`, `revoke.request`}, `manager` (accepted subset of
  the above, never broader than the mandate). No generic `OWNER`, no
  moderator powers, no community privilege.
- Sensitive writes (claim/proof/admin/transfer) require a fresh
  session (access token issued < 15 min ago) plus existing
  contributor-key proof. Hardware attestation: deferred explicitly
  (no client-side role flags ever decide).
- Invitations: scoped, 7-day expiry, recipient account/key acceptance;
  transfer needs recipient-bound proof + fresh confirmations + review.

## Business fields (P11 boundary)

- Editable: `opening_hours` (structured), `services[]` (closed enum:
  fuel, convenience, carwash, tire-service, oil-change, restaurant,
  atm, restroom, wifi, parking), `phone`, `website`, `description`
  (≤ 280 Unicode scalars). No gallery, no sponsored ranking, no price
  authority; canonical name/address/location/operator edits route to
  Directory review; official replies reuse 280-scalar feedback rules
  with current-grant checks.

## Retention, backup and privacy

- Granted cases: signed authorization retained while the grant is
  active + 2 years audit `[CALIBRATE: legal review]`; denied/
  withdrawn/appeal-superseded: 90 days; expired unused challenges:
  24 h; embedded photos/scans: all-copy 24 h cap (packaging as PDF
  never evades it). Backups inherit retention and purge expired proof
  before restore; restore replays deletion/revocation ledgers first.
- Public DTOs/logs/jobs/fixtures carry: badge state, business fields,
  at-time reply attribution. Never: CPF, private documents, keys,
  account/provider identity, contributor GPS, access URLs.
- Account deletion revokes its grants first (background cleanup
  after); other representatives' documents never enter an owner's
  export; station/community history survives.

## States and transitions (wire-frozen)

- Claims: `DRAFT`, `AWAITING_PROOF`, `CHECKING`, `NEEDS_INFORMATION`,
  `IN_REVIEW`, `APPROVED`, `DENIED`, `CANCELLED`, `EXPIRED`. Terminal:
  approved/denied/cancelled/expired (appeal opens a linked new
  claim). Source/verifier failure stays pending with a safe reason.
- Grants: `ACTIVE`, `SUSPENDED`, `REVOKED`, `EXPIRED` with query-time
  validity/account/operator checks; suspension/restoration need fresh
  audited decisions; jobs/replays/offline commands never restore
  expired or revoked scope.

## Explicitly open (blocking, not defaulted)

- `[OPEN/P31-T01]` permitted signature verifier (maintained tool vs
  contracted adapter vs documented manual process) after access/
  license/security verification; gov.br signing-API eligibility for
  this private app; VALIDAR automation suitability (public interface
  is not assumed to be an app API).
- `[OPEN/P31-T02]` Receita/QSA access path and corporate-act
  retrieval procedure; joint-signature and matrix/filial rules
  source; ANP Consulta Posto Web automation rights.
- `[OPEN/P31]` review staffing/coverage and impersonation-triage
  capacity evidence (objective: 24 h triage, measured later).
- `[OPEN/P30-T02]` moderation claim target/command enum extension
  (designed in schema task, not invented as strings).

## Source/provider decisions

- No new service/broker/tenant DB; Go monolith + `stationprofile`
  module + PostgreSQL/PostGIS + existing jobs. No dependency is added
  by this freeze; the verifier decision above carries its own
  license/security review in P31-T01.
