# P10-T09 — temporary staging smoke (NOT a launch, NOT certification)

Date: 2026-10-05. Temporary isolated backend on the existing shared VPS
(new namespace only; no other app touched, all peers re-verified healthy).
Public edge: `https://teste.abastevo.com.br` (Cloudflare-proxied, valid
edge cert, HTTP/2). This proves deploy/TLS/smoke paths only. Launch,
pilot results and G09 certification stay BLOCKED/NOT RUN.

## What ran (synthetic data only, fresh empty DB)

- Images built from project HEAD (`api` + `migrate` targets, pinned
  Go/distroless bases, no secrets in layers) and imported into k3s.
- Migrations applied from empty: 30/30 (`000001`–`000030`).
- Probes: `/health/live` → ok, `/health/ready` → ready (k8s + edge).
- `GET /v1/stations` → 200, honest empty page (`items: []`).
- `GET /v1/stations/nearby` (real PostGIS path) → 200, empty, request
  point not echoed; `cache-control: no-store` on the wire.
- Negatives: `lat=999` → 400; unknown station UUID → 404.

## Boundaries (not claimed)

- Staging env (`ANPFUEL_ENV=staging`), canonical host = public test
  domain; R2/media not attached (photo flows untested here).
- No ANP import, no contributors, no observations, no pilot cohort,
  no results in `pilot-results.md` (correctly absent).
- All deploy configs, secrets, keys and internal topology stay
  local-only and out of git by explicit owner order; this file holds
  outcomes only. Full backup/restore/load/provider proofs still owed
  for any real G09 claim.
