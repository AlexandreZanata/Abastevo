# SEC00-B — Route/capability inventory by access class (planning only)

Date: 2026-10-11. Status: PLANNING — access map, not a fix.
Unpublished by policy (planning-only work does not publish).
Verified against route registrations (`RegisterRoutes`), handler
doc comments and `cmd/api` wiring on maintained `dev`. The
OpenAPI `security: []` marker means proof-based auth (Signature
headers), NOT anonymous — classes below come from handler code.

## 1. Anonymous reads (no proof)

- `GET /health/live`, `/health/ready` (ops surface).
- Directory: `GET /v1/stations`, `/nearby`, `/by-cnpj/{cnpj}`,
  `/{station_id}`, `/{station_id}/prices|official-prices|profile`
  ("serves anonymous directory reads").
- Official: `GET /v1/community/feed` is dated public facts
  ("exposes only dated public price facts"); official reads
  anonymous ("serves anonymous official reads").
- Feedback reads: `GET /v1/feedback/comments`,
  `/comments/{id}/replies|tally`, `/ratings/stats`
  ("reads stay public and anonymous").

## 2. Anonymous signed price writes (proof, no email account)

Intentional by design — not an auth bypass (plan rule). Proof
required (unsigned refuses); unbound contributors keep the
anonymous baseline (`checkAccountGate`: only bound+suspended
FREE accounts refuse).

- `POST /v1/observations` (+ confirmations/disputes),
  `POST /v1/photo-captures`, `POST /v1/contributors`
  (registration ceremony), `POST /v1/identity/challenges`.
- `GET /v1/observations/{id}/helpfulness` (optional auth:
  anonymous OK, owner additionally gets `my_choice`).

## 3. Account-only social writes (proof + live account session)

- Feedback: `POST /v1/feedback/comments|ratings`,
  `/comments/{id}/edit|remove|replies|report|votes(+/remove)`,
  `/ratings/remove` ("only active accounts write"; dead
  account → forbidden, bad proof → invalid).
- Accounts: email codes/consume, keys, sessions
  refresh/revoke, providers link/unlink, deletion request
  (session/token or registration proof per route).
- Station suggestions (`POST /v1/stations/suggestions/...`)
  and profile claims (`POST /v1/profile/claims/...`):
  live account session required — "anonymous contributor
  proof alone is insufficient on every route" (verified doc
  comments in both handlers).
- Community social writes fall back to class 2 for unbound
  contributors (verified in `checkAccountGate`, not assumed).

## 4. Private owner reads (proof + ownership, shared not-found shape)

- `GET /v1/observations/{id}` (status),
  `GET /v1/contributors/me/observations` (history),
  `GET /v1/privacy/training-consent` (CS12A: unsigned → 401,
  cross-owner reads as default OFF).
- Evidence: `GET /v1/uploads/{upload_id}` (status),
  session/object reads owner-scoped (`Authenticate` first).
- Export download: owner-only key download (privacy P07).
- Key rotation (`POST /v1/contributors/me/keys/rotate`):
  proof-based owner action (old+new key proofs per rotation
  contract).
- Consent/promotion/erasure rows: owner-keyed throughout, no
  cross-owner oracle (CS09–CS10).

## 5. Upload grants (owner proof → short-lived presigned URLs)

- `POST /v1/uploads` (reserve → presigned PUT),
  `POST /v1/uploads/{id}/complete` (verify → finalize).
- Training namespace: presigned PUT/GET/DELETE under
  `training/v1/` via server storage identity, 5-min TTL
  (CS10C; MinIO/R2 S3-compatible).
- Grants are single-purpose (PUT/GET/DELETE), namespace-bound
  (`validObjectKey`), never public.

## 6. Restricted operator actions (no HTTP; host-secret CLI)

- `cmd/ops`: erasure replay, evidence/feedback exports and
  erasure, privacy erase/replay. Gated by host secrets
  (`CursorSecret`, R2 creds) + accountable
  `ANPFUEL_OPERATOR_ID`; unconfigured storage warns and marks
  rows only (orphaned bytes need a worker pass). No operator
  HTTP route exists — verify by absence in the route table.

## Explicitly NOT claimed

Quota/budget ceilings per class ride SEC00-C (numbers not
frozen here). Turning intentional anonymous price writes into
an alleged bypass is out of scope by plan rule. No live
readiness inference; no VPS/phone mutation.
