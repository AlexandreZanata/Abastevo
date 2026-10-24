# PAY-00 — baseline and parallel ownership reconciliation

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Worktree: `/home/iiii/.codex/worktrees/abastevo-payments/brazil-fuel-prices`
Branch: `codex/payments-stripe-asaas`
Base: `origin/dev` at `721582b8` (`docs(maps): diagnose the checker gate and
comaps integration blockers`).

Scope: read-only baseline/ownership inventory, an exclusive payment worktree and
the versioned threat/test matrix. No provider account, credential, charge, refund,
mandate, catalog, webhook, runtime composition, migration or deployment was
created or changed. Real sales and live money remain disabled.

## Baseline reconciled before editing

| Ref | SHA | Date | State at inspection |
|---|---|---|---|
| `origin/dev` | `721582b8` | 2026-10-24 | Maintained dev selected for this branch; includes MAP work already merged through `5cf02a73`/`721582b8`. MAP's `000122` overlay migration is only on its own branch, not dev. |
| `origin/main` | `c82823cb` | 2026-10-23 | Protected main after PR #147 merge. No payment planning commit is present. |
| local `dev` | `5c9f5204` | 2026-10-24 | Diverged from fetched `origin/dev` (3 local / 20 remote); not used as the new base. |
| local `main` | `a349d7c0` | 2026-10-09 | Stale local ref (0 ahead / 567 behind `origin/main`); not used. |
| planning commit | `ee8d316b` | 2026-10-24 | Complete payment plan; parent `b1a2bde6` (ESP planning), parent source local `5c9f5204`. Not an ancestor of `origin/dev`/`origin/main`. |

`git fetch origin --prune` was run before selecting the base; the refs above are
the fetched actual remote refs. The planning worktree remains clean and untouched
at `/home/iiii/.codex/worktrees/stripe-asaas-plan/brazil-fuel-prices`.

Because `ee8d316b` is based on a divergent local dev, its payment content was
replayed onto the reconciled `origin/dev` head instead of merging the old worktree
or importing the unrelated ESP planning commit:

- the five payment plan/contract/registry/prompt documents were added unchanged;
- the scoped `ROADMAP.md`, `docs/README.md`, `PREMIUM_BACKEND_APP_PLAN.md` and
  `PROGRESS.md` additions were re-applied to the current files;
- no ESP planning/code was copied or merged.

The original `ee8d316b` object and planning worktree are preserved. Final
integration may need to reconcile additive `ROADMAP.md`/`PROGRESS.md` entries from
MAP/ESP; Git conflict resolution must keep all concurrent entries.

## Source versus deployment

- The current dev source head is `721582b8`; it includes backend changes after the
  deployed source, including migration `000121` and charging identity/link code.
- The named test environment (`https://teste.abastevo.com.br`) runs API/worker
  source `53719336` with schema `000120`, per
  [the 2026-10-24 test update](../TEST_ENV_UPDATE_2026-10-24.md). That deployment is
  not current dev and contains no payment provider runtime.
- Payment composition is intentionally absent: `backend/cmd/api/main.go` and
  `backend/cmd/worker/main.go` leave the subscription provider/codec/binding ports
  nil. `Service.Configured()` is false in both processes.
- No payment code, migration, route, webhook, product, customer or provider
  resource exists in production or test deployment as a result of this phase.

## Parallel worktree and branch inventory

| Owner | Worktree | Branch / head | Working tree | Relevant shared paths observed |
|---|---|---|---|---|
| Payment (this phase) | `/home/iiii/.codex/worktrees/abastevo-payments/brazil-fuel-prices` | `codex/payments-stripe-asaas` / base `721582b8` | clean before PAY-00 edits | payment docs/evidence only in PAY-00 |
| MAP | `/data/dev/worktrees/abastevo-map-ev` | `codex/map-ev-implementation` / `606f3b08`, 4 ahead of `origin/dev`; advanced to `be0543bc` during PAY-00 | clean at inspection | `ROADMAP.md`, `docs/planning/PROGRESS.md`, `backend/cmd/api/main.go`, `backend/db/migrations/000122_map_overlays.sql`, `backend/internal/platform/config/`, `contracts/openapi/v1.yaml` |
| ESP | `.worktrees/enterprise-static-portal` | `codex/enterprise-static-portal` / `46fee9e0`, 7 ahead of `origin/dev` | modified `backend/db/queries/{directory,official}/*`, `backend/internal/modules/enterpriseexport/export.go` | `ROADMAP.md`, `docs/planning/PROGRESS.md`, `docs/README.md`, `backend/db/queries/`, Directory/Official export code, product/planning docs |
| PRM | `.worktrees/premium-planning` | `codex/premium-backend` / `a104ed80` | clean; head is an ancestor of `origin/dev` (0 unique commits) | existing `subscription` source and PRM evidence; no active PRM branch conflict |
| Primary checkout | repository root | detached `e22824db` | foreign modifications in Android Home/Survey, localization, `backend/internal/modules/evidence/adapters/store.go` and untracked `ManualSyncVerificationStore.kt` | preserve; not touched |

Other worktrees exist for unrelated app/landing/mobile work. They were not moved,
stashed, reset or edited. The exclusive payment branch and worktree were created
for this effort; no occupied branch was switched.

## Existing subscription/PRM owner to reuse

The existing Premium backend is already `LOCAL_DONE` source integrated into dev;
the stale `codex/premium-backend` worktree is an ancestor of `origin/dev`, so it is
not a parallel implementation to recreate.

- **Domain:** `backend/internal/modules/subscription/domain/policy.go` owns
  policy version 1, lifecycle states, 15-minute proof freshness and the fixed
  capability list. It is Play-neutral in shape but has no web settlement/order/
  mandate concepts.
- **Application:** `application/service.go` `Configured()` requires a Play
  verifier, token codec, account binding, test/production environment and package
  name. `Status()` removes all capabilities when unconfigured. `Admit()` uses a
  Play token digest and provider verification. `Export()` is deliberately
  independent of provider configuration.
- **Adapters:** `adapters/play/client.go` is the existing Google Play verifier;
  `adapters/http/http.go` exposes private `/v1/premium/*` status/verify/restore/
  export; `adapters/http/provider.go` is a Google Pub/Sub-style notification
  endpoint. `adapters/capability.go` checks capability inside the caller
  transaction.
- **Persistence:** migrations `000075`–`000077` own verification slots/operations,
  purchases, retired tokens, provider notifications and reconciliation schedules;
  account deletion has an in-transaction purge/retire trigger. `000121` is the
  current dev charging migration; no payment migration is allocated here.
- **Composition/jobs:** API and worker construct `Store`/`Service` but leave
  provider ports nil. Worker retains `subscription-refresh` and
  `subscription-acknowledge`; reconciliation only runs when configured. Existing
  jobs safety loops retain priority.
- **Evidence:** [PRM first ten](../premium/PRM_FIRST_TEN.md),
  [PRM next ten](../premium/PRM_NEXT_TEN.md) and
  [PRM08 gate matrix](../premium/PRM08_T05_GATE_MATRIX.md). Play sandbox remains
  PROOF_OWED; no web payment acceptance is inferred from them.

Payment ownership therefore remains: Account/Identity owns caller proof;
subscription owns access/capability; jobs owns durable dispatch; the future
payments module owns only web order/provider orchestration. It must not create a
second account, subscription, user ledger, tenant, team, station-management or
Enterprise-privilege owner. Free status/export and local data must remain usable
when web sales are disabled.

## Shared-path coordination record

No repository-wide lock exists; a clean file or worktree is not treated as a lock.
Coordination is branch/worktree ownership plus maintainer coordination, and any
shared mutation is serialized at its phase.

- **PAY-00 shared docs:** `ROADMAP.md`, `docs/planning/PROGRESS.md`,
  `docs/README.md` and `docs/planning/PREMIUM_BACKEND_APP_PLAN.md` received only
  additive planning/status text. MAP was clean at inspection; ESP's uncommitted
  files did not include these docs. Their future additive entries must be retained
  when integrating.
- **PAY-02:** `backend/go.mod`/`go.sum` require dependency/license/security review.
- **PAY-04:** migration number must be reserved at a coordinated fresh head after
  MAP's `000122`; no number is reserved now.
- **PAY-05:** `backend/internal/platform/config/` and `infra/env.staging.example`
  are shared with MAP/config work.
- **PAY-10/14/16:** `backend/internal/modules/subscription/` is shared with PRM;
  web sources extend, not replace, the existing owner.
- **PAY-13/18:** `backend/internal/platform/jobs/` is shared with safety/map work.
- **PAY-19:** `backend/cmd/api/main.go`, `backend/cmd/worker/main.go` and
  `contracts/openapi/` are shared with MAP/ESP.
- **PAY-20/23:** `infra/compose.staging.yml` and `infra/caddy/Caddyfile` require
  an explicit deployment window and scoped topology review.
- **PAY-22:** `ROADMAP.md` and `PROGRESS.md` require final additive reconciliation.

No shared code, migration, OpenAPI, CI, Compose/Caddy, Go module or runtime
composition file was edited in PAY-00.

## Threat and test matrix

The versioned matrix is [PAY_THREAT_MATRIX.md](PAY_THREAT_MATRIX.md), version 1.
It covers auth/IDOR/CSRF/replay, catalog and price integrity, ambiguous remote
writes, Stripe/Asaas webhook intake, settlement/access, lifecycle/switching,
recovery, privacy/boundaries and existing PRM reuse candidates. At PAY-00 every
row is OPEN or REUSE_OPEN; no row is closed and no provider proof is claimed.

## Validation performed

Commands are run from the payment worktree unless noted.

```sh
git status --short --branch
git worktree list --porcelain
git log -1 --format='%h %ad %s' --date=iso-strict <ref>   # dev/main/origin refs
git merge-base --is-ancestor <ref> <ref>                  # ancestry checks
git diff --check
python3 <scoped markdown-link/registry/DAG check>
bash scripts/scan-secrets.sh
```

Results:

- Worktree/status/branch inspection matched the table above; no foreign worktree
  head or working-tree modification was changed. MAP advanced independently from
  `606f3b08` to `be0543bc` during this phase; it was not touched.
- `git diff --check` PASS.
- Added/changed Markdown link check PASS: 57 links on added/changed content across
  8 changed Markdown files. (Pre-existing `PROGRESS.md` links to MAP-branch
  evidence are not on dev and were not introduced here; MAP owns them.)
- Payment registry check PASS: 24 ordered unique PAY-00–23 IDs and every
  `depends_on` points to an earlier phase.
- Threat matrix check PASS: 52 unique rows — 46 `OPEN`, 6 `REUSE_OPEN`, zero closed
  or provider-proven. No row was silently marked accepted.
- `bash scripts/scan-secrets.sh` PASS; the changed docs contain no credential/PII
  fixture, real provider key, charge URL or QR payload.
- No Go, SQL, provider or runtime test is required or claimed for a documentation-
  only phase. No provider account was contacted.

## Remaining obligations and next action

- PAY-01 freezes Premium/Enterprise offers, BRL values/terms, lifecycle/refund
  policy and the two independent Pix channel gates. Missing real prices/features
  must keep that sale disabled; synthetic fixtures may be used for generic code.
- No provider credentials/account approval, Pix Automático eligibility, fiscal
  decision, distribution policy or sandbox proof exists.
- Next action: PAY-01 only after the maintainer authorizes the next phase.
