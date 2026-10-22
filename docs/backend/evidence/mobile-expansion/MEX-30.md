# MEX-30 — Deterministic question interpretation and suggestions

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `f56aeb51` (maintained dev, PR #145 head). Date: 2026-10-22.
Depends MEX-02/23/29 (typed query vocabulary, reads, ranking),
accepted. Rules B-BR-MEX-08–13; BUC-MEX-06/07. Risk: query
meaning/adversarial input (critical: free text must never execute,
guess or fabricate).

Before behavior: supported Portuguese questions resolve deterministically
to frozen typed queries (price ranking or official records), explicit
alternatives, or explicit unsupported reasons — never SQL, scripts,
remote URLs, a model or a search service. Fuel, geography, metric,
source week and condition extract from allowlisted accent-free
vocabulary; ambiguous metrics, fuels, sources and periods ask with
labelled alternatives; unknown fuels, conditions and overlong or
adversarial text are unsupported without evaluation. Suggestions are a
frozen chip set whose every entry resolves. Recovery: fall back to
structured filters and suggested questions.

## Delivered

- `backend/internal/modules/exploration/domain/` (stdlib-only):
  `Interpret` over normalized text with longest-match fuel mapping
  (DIESEL_S10/S500 asked explicitly, legacy premium spelling mapped
  on input only), exact UF/country/city geography, observation vs
  area-mean metrics with direction, ANP-weekly reference Sundays from
  the caller date, exact conditions with unrecognized qualifiers
  refused, and official source routing (enforcement/pmqc/sanctions)
  with unbounded full-range date filters. Outputs validate against
  the frozen exploration schema.
- `backend/internal/modules/exploration/application/suggestions.go`:
  six frozen chips (price + official) with reviewed labels, each
  resolving.
- Twelve tests: paraphrases to one vector, accents/UF spellings,
  explicit metric/diesel/source choices, unknown fuel/condition
  refusals, adversarial literal handling with length cap, official
  source separation, unit/condition exactness, frozen-schema
  validation of queries and alternatives, and suggestion resolvability.

## Actual checks

From this worktree (2026-10-22, parent `f56aeb51`):

- `go test -count=1 ./internal/modules/exploration/...` → PASS, 12
  tests (10 domain + 1 application + 1 schema-driven).
- Resolved `PriceQuery` and `OfficialQuery` plus NEEDS_CHOICE
  alternatives validate against
  `contracts/openapi/mobile-exploration-planned.json`
  (`PriceQuery`, `OfficialQuery`) via kin-openapi; no invented
  fields, enums or shapes.
- `go vet ./internal/modules/exploration/...` PASS;
  `GOTOOLCHAIN=go1.27.1 staticcheck
  ./internal/modules/exploration/...` PASS.
- `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS.
- No DB, migration, HTTP, OpenAPI, worker or Android changes; no
  source activation, no PII in Git/logs/fixtures.

## Limits

- This unit interprets questions only. It executes nothing, reads no
  source, and serves no endpoint; MEX-31 (Android) consumes these
  vectors once authorized. A resolved query still needs capability
  resolution before execution (unsupported combinations rejected
  there, not here).
- Geography without a catalog: UF/country exact, city passed through
  for API-side exact matching (unknown cities page empty). No IBGE
  codes invented, no proximity or name matching.
- Dates: ANP weeks derive from the caller reference date (latest
  Sunday); official queries without explicit dates cover the full
  range. No staleness or availability claim is made here.
- Bare "diesel", bare "mais caro/barato/média" and unclear official
  sources ask explicitly; the interpreter never picks silently.
- No device, no production, no deploy and no release acceptance is
  claimed.

Next: MEX-31 (Android discovery journeys) needs its own authorization;
it consumes these tested vectors.
