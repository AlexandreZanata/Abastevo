# P31-T04 — Scoped business edits and official replies

Status: LOCAL_DONE on `codex/phase-31-verified-representation`. Task: P31-T04.
Binds B-BR-P02/P10–P12/P16 and BUC-P05. Server-enforced management
capabilities with at-time business attribution; canonical facts,
prices, votes and criticism stay outside authority.

## Behavior (TDD, critical-first)

- Migration `000040_comment_business_attribution.sql` (append-only
  nullable columns + index; FKs to stations/grants): personal history
  stays NULL (existing behavior unchanged); no backfill.
- Feedback: `AttributeBusinessComment` on the store (author match +
  null-guards → not-found, no oracle), service passthrough with
  session gate, `CommentView` + thread JSON carry
  `business_station_id/_grant_id` (at-time truth; reads never
  re-check liveness). MemStore mirrors with a side table. OpenAPI
  view shape extended (optional, compatible).
- stationprofile application `manage.go`: `liveGrant` (grant scope +
  account liveness + operator currency in one place),
  `EditBusinessFields` (frozen field/service enum, 280-scalar
  description, 256-char caps, optimistic revision), `PostOfficialReply`
  (grant scope + live rechecks + feedback transport + authorship-
  checked attribution). No price-trust/ranking/regulatory overwrite;
  no criticism suppression.
- Adapters: pg projection update (revision-guarded), manage HTTP
  routes (edit + replies, session-in-body, `no-store`, 400/401/403/
  409 mapping), `cmd/api` wiring via closures (modules never
  cross-import). OpenAPI edit/reply paths + schemas (`vacuum` PASS,
  `apicontract` + `check-compat.sh` PASS).

## Validation

- Unit (`-race`): scope/policy/version denials, 280/281 transport
  rule, manager-without-scope, stale operator, manage HTTP
  policy/version/no-store — PASS.
- Integration (real PostGIS, `-race`): edit→revision bump, reply→
  thread attribution, stale-operator denial, concurrent edits 1-0,
  attribution author-check/rebind refusal + view exposure. Fresh
  disposable DBs (migrations incl. 000040).
- Fixed from real failures (not weakened): mixed param names,
  unresolved store reference, non-UUID grant ids, missing FK seed
  chain, test-order staleness.
- Regression: full `stationprofile/feedback/directory/account/
  moderation` unit + integration PASS (zero failures); `go vet`
  clean; `sqlc vet` + `generate` clean; `go build ./...` PASS.
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits and next

- App-side badge/reply buttons consume these contracts in P32;
  abuse/lifecycle acceptance is P33.
- Next (other session): P31-T05 contestation/suspension/revocation.
