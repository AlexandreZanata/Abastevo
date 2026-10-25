# ESP-00 — Reconcile baseline and freeze selected scope

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Worktree: `.worktrees/enterprise-static-portal`
Base: maintained dev `721582b8a078b47c8d34de8371326b27decb1fdf`
Planning preserved from `b1a2bde6bf08f69012e4795abdf350579bcbd146`, replayed
onto the base as `1d9cf0d7`.

Rules: B-BR-ESP-01/16. No BUC execution at this stage.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-00--reconcile-baseline-and-freeze-selected-scope)
and [contract](../../../product/ENTERPRISE_DATA_STATIC_PORTAL_CONTRACT.md).

## Actual baseline

| Ref / checkout | Commit | Observed state |
| --- | --- | --- |
| `origin/main` | `c82823cb` | maintained release branch |
| `origin/dev` | `721582b8` | maintained construction head; map checkpoints MAP-00–07 plus MAP-08 diagnostics |
| primary checkout | `e22824db` (detached) | unrelated dirty Android and Evidence Store edits; not used |
| map worktree `/data/dev/worktrees/abastevo-map-ev` | `721582b8` on `codex/map-ev-implementation` | active, dirty with MAP-08–14 files and an untracked `000122_map_overlays.sql`; not touched |
| ESP worktree | `1d9cf0d7` on `codex/enterprise-static-portal` | isolated from `origin/dev`; planning overlay preserved |

The planning commit was based on an older local dev (`5c9f5204`). Replaying only
its six documentation paths avoided importing the three unrelated local Android
commits. The one content conflict (`docs/planning/PROGRESS.md`) was resolved by
keeping the current map record and adding the reconciled ESP entry; no map text
was removed. `git fetch origin --prune` left `origin/dev` and `origin/main` at the
commits above.

No ESP implementation existed at this base. A scoped search for `enterprise`,
`static portal` and `export manifest` under `backend/`, `landing/`, `contracts/`
and `scripts/` returned no product code, so there is no duplicate importer,
station registry, parser, database or portal to retire.

## Selected scope

Front A (shared data foundation, sequential): ESP-01 source profiles/rights/public
fields; ESP-02 bounded discovery/correction gaps; ESP-03 module-owned export
snapshot; ESP-04 deterministic analytics; ESP-05 one enterprise offering
boundary (specification only); ESP-06 durable build intents; ESP-07 local data
acceptance.

Front B (static public consultation): ESP-08 routes/templates; ESP-09 catalog,
price and bounded-history pages; ESP-10 regional static search; ESP-11
safely-associated official records only after the named FQ acceptance; ESP-12
insights/downloads; ESP-13 reproducible artifacts; ESP-14 scoped hosting;
ESP-15 refresh/correction recovery; ESP-16 capacity measurements; ESP-17
preview/operations; ESP-18 public publication after the declared operational
scope and protected integration.

V1 content is limited to public ANP prices, station identity, source clocks,
coverage and admitted official records. Excluded: Android map implementation,
tenant/team management, billing, private business participation, automatic
alerts, consumer reports/media/GPS, new risk scores, national history shipped to
every browser and any visitor-triggered import/build.

## Component split with the map effort

| Component | Priority during construction | ESP rule |
| --- | --- | --- |
| Directory identity and station reads | MAP | Consume `StationReader` (`Search`, `Detail`, `ByCNPJ`) and canonical UUID/CNPJ; no identity edits |
| Official/ANP source producers, parser and import jobs | MAP | Consume read ports and staged revisions; no second importer/parser |
| `tools/station-prep` and Rust preparers | MAP | Do not edit; reuse only through an existing prepared boundary |
| Quality/FQ enforcement, fines and reads | FQ | Consume accepted public read contracts; do not reimplement |
| `landing/` build and static assets | ESP | Extend the existing HTML/CSS/TypeScript stack; check again at ESP-08 |
| API/worker composition | MAP | ESP-06 may add its own bounded worker wiring only after coordination |
| OpenAPI, shared schemas and generated code | shared | Recheck current head before editing; no parallel contract |
| Migrations | shared | Last committed is `000121`; the map worktree has an untracked `000122` claim, so ESP must choose any number only against the then-current head |
| Caddy, Compose, deploy scripts | shared | ESP-14 is a narrowly scoped addition after topology review and explicit edge authorization |
| CI, dependencies, lockfiles | shared | Add none unless a recorded need justifies it |
| `ROADMAP.md`, `PROGRESS.md` | shared | Append/reconcile only; preserve map history |

If an ESP phase needs to change one of the MAP-priority producers, the work
stops on that component, records the exact interface or correction needed, and
continues only with independent tasks until coordination. A worktree does not
remove contract, database-numbering or runtime conflicts.

## Verified dependencies and gaps

- **Official prices:** XLSX parser, staged/published revisions, read port,
  current-price/ranking queries and import jobs exist. `adapters/jobs/discovery.go`
  probes only the current and previous survey weeks and documents that a source
  without ETag re-imports only through manual enqueue. This is the actual gap for
  ESP-02, not a reason to create another discovery service.
- **Directory:** canonical station UUID and bounded `Search`/`Detail`/`ByCNPJ`
  reads exist; reviewed coordinates remain distinct from absent coordinates.
- **FQ dependency:** FQ-06/07/08 are LOCAL_DONE / INTEGRATION_PENDING. There is
  no live Go enforcement stream, no activation of quality/fine imports, and safe
  Directory company/station association plus deployment acceptance remain OWED.
  Therefore ESP-11 cannot be accepted from the current evidence and its slice
  must stay blocked; price/catalog work is independent.
- **Landing:** existing static HTML/CSS, plain TypeScript, a Node build/check/test
  suite and fingerprinted assets are available for extension. Other landing
  worktrees exist outside maintained dev; ESP must recheck their merge state at
  ESP-08.
- **Hosting:** `infra/caddy/Caddyfile` proxies only `/v1/*` and `/health/*` and
  returns 404 otherwise. There is no static document root, release pointer,
  publisher or cache policy yet; ESP-14 owns that new scoped work.
- **Database:** PostgreSQL/PostGIS and append-only migrations are the established
  baseline. No ESP schema change is made in this phase.

## Map interaction and integration risk

The map worktree is actively dirty beyond maintained dev. Its current MAP-08 is
IN_PROGRESS/BLOCKED (native check/route not accepted). ESP does not include,
copy or validate those files. At the end of the ESP batch, the final integration
must compare the then-current dev head; unaccepted map work must not be pulled
into the ESP merge silently. Shared-file conflicts (`PROGRESS.md`, `ROADMAP.md`,
OpenAPI, migration numbering, Compose/Caddy) require a coordinated checkpoint
before the protected dev → main PR.

## Checks executed

- `git status --short --branch`, `git branch -a -vv`, `git worktree list` and
  `git log` in the primary checkout, ESP worktree and map worktree.
- `git fetch origin --prune`; `git rev-parse origin/dev origin/main`.
- `git rev-list`/`merge-base` comparison of the planning commit, local dev and
  maintained dev; the replay was cherry-picked and its conflict resolved by hand.
- Scoped inventory of `official`, `directory`, `quality`, `landing`, `infra/caddy`,
  migrations, FQ evidence/handoff and MAP phase registry.
- `git diff --check`; local Markdown path/link existence check; secret-pattern
  inspection of the changed documentation. The ESP-00 links and the new
  `PROGRESS.md` evidence link resolve. Pre-existing MAP evidence links in
  `PROGRESS.md` point to a non-existent `docs/planning/evidence/` at this base;
  they are map-owned and were left unchanged.

No backend, Rust, landing or Android runtime test was run: this phase changes
documentation only and enables no source, schema, job or route. No source file,
migration, API contract or hosting configuration was modified.

## Exit and next action

Exit satisfied for ESP-00: actual baseline, selected V1/V2 scope, MAP split,
shared-component rules, dependency gaps and outstanding gates are recorded; the
planning overlay is preserved on maintained dev. Rollback is to drop this
documentation commit; nothing was enabled.

Next: ESP-01 — admit bounded source profiles, rights, public fields, clocks,
retention and correction windows, reusing FQ/RST admission evidence.
