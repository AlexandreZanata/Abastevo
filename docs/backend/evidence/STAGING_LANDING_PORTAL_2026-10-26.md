# Staging landing, portal search and ingestion — 2026-10-26

Status: STAGING_DONE / INTEGRATION_PENDING (PR to main). Not a public
release: noindex everywhere, robots disallow, no sitemap; G09 untouched.
Scope authorized by the owner: `https://teste.abastevo.com.br`, namespace
`abastevo-temp` only. Owner decisions recorded this day: B-BR-002a float-noise
admission approved; ANP price/registry search exposed on staging noindex.

## Deployed

- Source: `main` 4eeccfbf (PR #153 landing) + this branch; final images
  `abastevo-{api,worker}:stg-e3887d5c`. No new migration.
- Web tier `abastevo-temp-web` (Caddy 2.10.2, NodePort 30251,
  [`infra/caddy/staging-web.Caddyfile`](../../../infra/caddy/staging-web.Caddyfile)):
  landing at `/`, portal routes from the worker's verified release.
- Edge nginx: `/v1/`, `/health/`, `/abastevo-dev-photos/` and the upload regex
  stay on the API; everything else goes to the web tier through a 60 s
  micro-cache (`proxy_cache_lock`, stale on error). Backups and rollback
  files live privately under `/opt/abastevo-temp/stg-landing-20261026/`.
- Worker: ANP discovery on, enterprise export + portal refresh on, landing
  URL and search JS set; registry CronJob moved to `17 3 * * *` with the
  new `--budget-wait`.

## Live results

- Rust registry ingestion (manual run of the CronJob): 45,808 input,
  45,688 accepted, 120 quarantined; prepare 4.8 s / 113 MB RSS; load+publish
  164 s; peak container memory 197 MB.
- ANP week 2026-10-18..24: anp-v1 run held at `needs-review` (891
  over-precision = 4.3%); anp-v2 run published 20,926 prices, 0 quarantined,
  891 admitted as `float-noise`.
- Portal release `sha256:b446478a…`: 52,578 pages; search shards per UF;
  7 CPF-shaped station names minimized by policy esp-02 before export.
- Browser check: `/consultar/?uf=MT&q=sorriso` lists real stations with
  prices; only console error is the Cloudflare analytics beacon blocked by
  the CSP (zone setting, not loaded by the site).

## Gaps found and fixed (each with tests)

| Gap | Evidence | Fix | Result |
| --- | --- | --- | --- |
| Registry refresh failed 3 days at the top-of-hour load spike | `refresh_failed` load1 > 0.65 x CPU | bounded `--budget-wait`, cron off :00 | live run complete |
| Refresh dropped the landing chrome | no LandingURL in job | `ANPFUEL_ENTERPRISE_PORTAL_LANDING_URL` | pages link the landing |
| Real ANP week never published | 891 IEEE-754 noise cells | B-BR-002a, parser anp-v2 | 20,926 published |
| Portal exceeded page/shard budgets | 52,578 pages; SP shard 461 KB gzip | measured defaults 100k pages / 512 KiB | release promoted |
| Budget failures regenerated ~460 MB every 15 min x3 | 10 dead refresh cycles | typed BudgetError + budget key coalescing | fail fast |
| CPF in MEI station names | portalcheck refusal | esp-02 name minimization | check passes |
| Stale snapshot reused after policy change | state ignored policy | policy-bound state validity | rebuilt |
| Verification 71 s CPU per pass | pprof: regex 61%, gzip alloc 23% | linear scanner, prefilters, reused gzip | 15.4 s, same output |
| Promotion 25+ min | per-file fsync of 52k files | hardlink + one syncfs | 181 s -> 46 s |
| Web tier OOM at 150 req/s | Caddy OOMKilled x3, 54% errors | gzip-only + edge micro-cache + 256 MiB | 0 errors |
| `/v1/stations` seq scan + sort per page | EXPLAIN: 48,750 rows | primary-key keyset | p95 5.95 s -> 69 ms |

Load (k6 on the VPS, 2 pinned cores, real TLS edge, read-only mix of landing,
assets, portal pages, SP/MT shards, station pages, nearby, list, health):

| Metric | Before @150 | After @150 | After @300 |
| --- | --- | --- | --- |
| Errors | 53.84% | 0% | 0% (21,919 req) |
| p95 all | 2.67 s | 41.8 ms | 10.2 ms |
| Landing p95 | 2.23 s | 23.5 ms | 4.7 ms |
| SP shard p95 | 4.21 s | 56.8 ms | 15.0 ms |
| `/v1/stations` p95 | 5.95 s | 69 ms | 19.7 ms |
| nearby p95 | 2.45 s | 90 ms | 13.3 ms |

Other namespaces were snapshotted before and after; Sorrimobi health stayed
200 during every run.

## Limits and follow-ups

- Measured, not changed: registry `load_publish` stages one row per round
  trip (164 s/day, inside its 18 min bound).
- ~~Public API returns 7 CPF-shaped legal names~~ fixed below (B-BR-017).
- SP search shard stays 461 KB gzip; reducing it needs a shard redesign.
- Indexable national sitemaps would exceed 50,000 URLs (production only).
- Nearby still returns no pins: national coordinates belong to the separate
  national map plan (`docs/planning/MAP_NATIONAL_STATIONS_PLAN.md`, own PR).
- Rollback: restore the private nginx/deployment/cronjob backups, delete
  `abastevo-temp-web`, restore the DB dump only if data must be reverted;
  migrations untouched.

## Addendum — search v2, CPF in public reads, incident (same day)

Owner request: smart search without UF/city selects, national search by
name/CNPJ, search on fiscalizacao and a fuel/order explorer on precos, plus
the CPF fix in the public API. Delivered on `codex/portal-smart-search`:

- B-BR-017 `kernel.PublicStationName` on every public read (Directory
  detail/search/by-CNPJ/official anchors, community feed, station profile,
  quality) and the export; PostGIS test pins detail, by-CNPJ and search.
- Search v2 files (index with IBGE city names, prefix shards, city, fuel,
  fiscal) and one combobox client; city names replace "Município 2100055"
  on every portal page. Real build: largest prefix shard 92 KB gzip,
  gasoline 230 KB, index 88 KB; local browser checks of the three pages.
- Refresh receipts carry `portalgen.GenerationKey`, so generator, asset or
  search-script changes regenerate even with unchanged data.

Incident (staging only): the search-v2 images came from a branch rebased on
`main` bbd96568, which added migration 000132; the images were rolled out
before migrating. With `maxSurge=0` the old pod stopped and the new one
refused the older schema: API unavailable ~6 min until `rollout undo`.
Recovery: verified `pg_dump` (`db-before-000132.dump`), migration Job
`applied [000132]` (additive), immediate roll forward (old image unready
~1 min after the migration). Lesson for automated delivery: migrate (and
check schema compatibility) before rolling app images, keep a surge pod
or a readiness gate, and roll back automatically on failed readiness.
