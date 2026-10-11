# CS02B — Helpfulness HTTP and OpenAPI

Date: 2026-10-11. Status: LOCAL_DONE. Scope: HTTP routes, composition
wiring, OpenAPI schemas and contract vectors only — no migration/store
change (CS02A), no VPS/APK/dependency change. Head: maintained `dev`.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md),
[CS02A store](CS02A_HELPFULNESS_STORE.md). Rules: B-BR-CS02, BUC-CS02.

## Routes (`community/adapters/http`, same proof/envelope conventions)

- `POST /v1/observations/{id}/helpfulness` → 201 with vote receipt,
  exact tally and owner overlay; auth required.
- `POST /v1/observations/{id}/helpfulness/remove` → 200 with tally
  after tombstone; auth required.
- `GET /v1/observations/{id}/helpfulness?decision_sequence=N` →
  200 public counts; `my_choice` only on the authenticated read path,
  never public caches.
- Bodies: 64 KiB cap, strict decode (unknown/duplicate/trailing
  rejected), UTF-8 enforced, canonical integer sequence ≥ 1, frozen
  choice checked at the wire and again in domain/application.
- Errors: `community.self-vote` 403, `community.stale-revision` 409,
  `community.invalid-vote` 400, plus the existing auth/blocked/
  not-found/ineligible/bad-body codes. Responses carry no voter,
  account, key, device or cursor internals and are `no-store`.
- Wiring: `cmd/api/main.go` mounts real `Cast`/`Remove`/`TallyFor`
  over `HelpfulnessPorts` (same account gate as votes).

## Binding correction (found by the e2e test, fixed here)

Observations carry `caller.Token` as contributor reference, so the
application vote identity now binds `caller.Token` — exactly like
community confirmations — instead of the contributor ID; the account
gate keeps using the ID. Without this, self-votes would never refuse
(the e2e author-vote case returned 201 before the fix, 403 after).
CS01 unit fakes updated to per-caller tokens with contributor-scoped
submission keys. CS01 freeze wording is superseded accordingly:
one live vote per attribution+target+revision.

## Validation (actual)

- Unit `adapters/http`: 12 helpfulness tests green (happy paths,
  anonymous writes 401, 10 malformed bodies, UTF-8, 5 error mappings,
  remove zeroing + overlay cleared, anonymous tally without overlay or
  leaks, bad sequences, 404).
- E2E `adapters/http` (full-stack fixture with real signed proof and
  pg store): cast 201 + same-id crash replay, author self-vote 403,
  anonymous tally without overlay, stale 409, removal zeroes;
  tampered signature 401, unknown key 401, oversized 400, unknown
  target 404. Full package green (existing e2e unaffected).
- CS02A store suite re-green after the token change (10/10,
  concurrent trio under `-race`).
- `apicontract`: 4 new vectors validate against the new
  `HelpfulnessSubmit/Tally` schemas (spec self-check + local refs ok).
- `go build ./...`, `go vet`, `gofmt -l`, `git diff --check` clean;
  `scan-secrets.sh` passed. Only disposable local test databases used.

## Limits / owed

- `CONTRIBUTION` moderation target + hide/delete writers: CS05.
- Erasure footprint wiring (account → refs fan-out): CS09 path.
- Restricted DB roles: P08 provisioning.
- Android helpfulness cards/overlay: CS08/CS11 (strings.xml VALID/
  INVALID collision already noted).

Next microtask: CS03 — admission and abuse protection.
