# CS05B — Review annotation and adversarial pass

Date: 2026-10-11. Status: LOCAL_DONE. Scope: review lookup,
annotation rendering, adversarial script — no migration, no VPS/APK.
Head: maintained `dev`.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md),
[CS05A target](CS05A_CONTRIBUTION_MODERATION.md). Rules: B-BR-CS05.

## Added

- `ReviewLookup` port in community application plus
  `TallyResult.UnderReview`. `TallyFor` consults the open
  CONTRIBUTION review (spelling pinned by test, no cross-module
  domain import); nil wiring reads clean for older compositions;
  lookup failure fails closed with `ErrReviewUnavailable`.
- Moderation `Store.OpenReview` over the existing open-case query
  (unknown targets read clean, never errors).
- HTTP renders `under_review` + frozen `review_notice`
  ("contribution under review", literal pinned); lookup failure maps
  to 503 `community.review-unavailable`. Cast receipts stay
  write-acks without annotation; GET tally and removal responses
  annotate (the public tally surfaces).
- Wiring in `cmd/api` and the e2e fixture; OpenAPI fields on
  `HelpfulnessTally` + contract vector.
- `scripts/helpfulness-adversarial.sh`: staging-only stateless
  probes (anon writes, malformed/oversized/UTF-8 bodies, unknown
  target/key, tampered proof, wrong method) with loopback-only
  local mode, https + confirm for staging, and refusal guards.
  Authenticated adversarial coverage stays in the Go e2e suite
  (replay, self-vote, stale, quota), cited in the script header.

## Validation (actual)

- Unit application: annotated/clean/nil-lookup/lookup-failure
  cases green. Unit HTTP: frozen wording + 503 mapping green.
- E2E: clean tally → open case → annotated exact counts → dismiss →
  annotation cleared (RED observed before fixture wiring, GREEN
  after). Full HTTP integration package green.
- Contract: annotated-tally vector validates; spec self-check ok.
- Script: `bash -n` + `shellcheck` clean; guard paths exercised
  (`--help`, bad target, non-loopback local refusal); chi 405
  behavior verified empirically before freezing the probe. Staging
  execution awaits the authorized staging window (not run here).
- `go build`, `go vet` (incl. integration tags), `gofmt -l`,
  `git diff --check` clean; `scan-secrets.sh` passed.

## Limits / owed

- 15-minute cursor decisions hold via the feed cursor TTL (cited,
  unchanged).
- Staging run of the adversarial script: authorized window.
- Erasure fan-out: CS09. Android surfaces: CS08/CS11.

Next microtask: CS06 — projections and consensus.
