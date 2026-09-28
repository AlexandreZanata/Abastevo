# Infrastructure, release and recovery plan

Status: production/staging not provisioned; P01 local Compose/database and container foundations exist. Initial deployment target is one Linux VPS with 16 GB RAM, Docker Compose, Caddy, API, worker and PostgreSQL/PostGIS; private R2-compatible storage and Cloudflare are external. No infrastructure purchases, credentials, domain registration or production changes occur during planning.

## Environments and delivery

- Local: disposable Compose database/storage emulator with synthetic fixtures; bind local ports to loopback and isolated volumes. Explicit local reset command only for disposable data.
- CI: short required checks for phase integration; ephemeral real PostGIS and synthetic objects for selected critical checks and full release certification. No production secret access on fork PRs. Current path-aware workflows remain active until G01-FLOW; [CI_PLAN](../planning/CI_PLAN.md) defines the safe transition.
- Staging: separate DB, storage bucket/prefix, keys and hostname. Test actual R2 URL/ACL behavior, TLS/edge/proxy signature behavior and deploy/rollback here. Do not restore raw production data into ordinary staging.
- Production: private DB network, persistent volume, controlled SSH, non-root containers/read-only filesystem where possible, resource/health limits, versioned images by digest, off-host backup. API/worker from one reviewed release commit; migrator invoked once with lock.

Release cadence: P01–P08 phase merges integrate code and docs; they do not automatically deploy production or certify the entire backend. P09 selects an immutable candidate after those merges, runs the complete matrix and records G09 acceptance before Android work. P08 provisioning/restore/load checks still run when needed to validate that phase; reuse only demonstrably matching evidence. See [delivery workflow](../planning/DELIVERY_WORKFLOW.md).

Deploy sequence for a release candidate: build/test/scan → immutable image → staging → verify recent backup and schema compatibility → run additive migration under timeout → start release API/worker → readiness and synthetic smoke → switch traffic → monitor. Record commit, image digests, schema version, config/policy versions and operator. No secrets in release evidence.

Rollback: first stop rollout, route to previous compatible binary, pause affected worker type, retain accepted jobs. Prefer forward migration fix over destructive down migration. If corruption requires restoration, disable writes, preserve forensic snapshot, restore last verified backup to a new volume/instance, replay allowed changes if supported, apply deletion/retention ledger, check consistency and only then switch traffic. Never `docker compose down -v` against persistent environments.

## Backup and restore acceptance

Initial proposed objectives: RPO ≤24 h and RTO ≤4 h; explicit pilot acceptance needed. Daily encrypted logical backup, roles/extensions/schema recorded, SHA-256 manifest, off-host destination using separate credentials; 7 daily +4 weekly copies with 35-day maximum. Exclude transient precise-location/raw-OCR data, auth challenges and IP rate windows from backup data (retain schemas); otherwise a 35-day backup would violate their 24-hour retention cap. Restored pending observations without required transient evidence must reject/request resubmission, not fabricate validation. Snapshot-only recovery is insufficient. Keys must be recoverable outside the VPS without entering Git. Monitor backup age and failure.

Before G09 and monthly afterward, restore to an isolated environment from downloaded backup using documented commands, verify schema/counts/known record hashes, sample official/community read paths, rebuild current projections, reconcile jobs and evidence references, replay deletion ledger, apply retention and record duration. Restore must meet target RPO/RTO, not merely produce a readable dump file. Evidence objects have independent lifecycle: expired photos remain unavailable after DB restore; missing objects must yield deleted/missing states, not broken public URLs.

For lower RPO later, add verified WAL/PITR through an ADR and restore test; do not advertise sub-day recovery while using daily dumps alone.

## Health, observability and budgets

`/health/live`: process event loop alive; no external dependency check. `/health/ready`: essential DB/schema compatible and request admission available; no sensitive details. R2/geocoder health is a degraded capability metric rather than a reason to stop all public official reads. Job age/backlog has its own alarm.

slog JSON with UTC timestamp, severity, service role, request/job ID, route template, duration, status, stable error code. Do not log raw query/body, IP/GPS, auth headers or signed media URL. Metrics labels are bounded (route, status class, job type), never contributor/station/observation IDs.

Initial **acceptance workload**, not production capacity claim: 50 read requests/s and 5 writes/s for 30 min on 16 GB staging-equivalent VPS, with 100k station fixtures, 1M official rows, 1M community facts, 10 concurrent media jobs capped by worker config. Include a dense-city radius and 20% cache misses; also run origin-only reads. Targets: read p95 ≤300 ms, p99 ≤1 s; observation admission p95 ≤500 ms excluding object upload; unexpected 5xx <1%; projection p95 ≤30 s; memory <75% sustained, no growing queue at steady input. Measure p50/p95/p99, RPS, query time, pool wait, CPU/RAM/disk IO, cache hit ratio and object operations. If targets fail, tune measured bottlenecks or reduce documented pilot capacity before launch.

Alerts initially: DB unavailable/5xx spike; disk >75% warning/>85% critical; backup age >26 h; failed restore drill; oldest critical job >5 min; dead jobs; rejected ANP revision/layout change; media cost/quota anomalies. Establish one tested operator notification route during provisioning; do not send real notifications without a configured operational recipient.

## Failure-mode runbook

- PostgreSQL down: reject writes with 503, readiness false, keep process alive; restore connectivity/check disk before retrying. Never pretend writes were stored.
- R2 down: stop issuing costly authorizations if persistently unavailable; uploads/validation report pending/degraded; official reads continue. Retry boundedly and reconcile orphans.
- Cloudflare down: investigate DNS/edge and use only an explicitly secured origin recovery path; never bypass TLS/origin authentication to regain availability.
- Geocoder down: keep location UNKNOWN and last verified location; textual lookup works; queue retries within quota.
- ANP layout change: quarantine failed import, alert, preserve last published revision and freshness; new parser via fixtures.
- Upload succeeds, complete fails: client retries complete with same command; orphan grace avoids deletion during retry. Complete twice returns same state; no second object binding.
- Worker dies: lease expiry + fencing reclaims job; exactly-once business effect enforced by unique key/version, not assumed delivery.
- Mobile offline/clock wrong: stable queued command, new signature challenge on reconnect; preserve claimed capture age and never claim server-receipt time is capture proof.
- Duplicate evidence/observation/confirmation: deduplicate effects; record risk/409 where relevant; same request retry remains safe.
- Concurrent confirmations: DB uniqueness and single-vote reduction prevent double support; recompute price key serially.
- Old client/new API: compatibility tests and retained v1 semantics; unsupported writes receive stable error, never reinterpret fuel/units.
- Migration fails: stop deploy, retain prior compatible binary and backup; checksum/lock diagnostics; no manual schema patch outside a new migration.
- Storage fills: reject new work before exhaustion, pause noncritical imports, alert, clean only known expired temp/orphan data. Do not delete price history to free space ad hoc.
- Backup fails to restore: G09 fails; repair backup pipeline and repeat drill before opening production writes.

## Cost model and scale triggers

No exact supplier quote or capacity promise is implied. Cost = VPS+disk+backup+DNS/security plan+object retained bytes+object operations+processing/bandwidth+operator time. Photo upload rate, retention and decode work dominate differently from cached reads. [R2 pricing](https://developers.cloudflare.com/r2/pricing/) separates storage/operation charges and describes egress terms; verify plan/product details before procurement.

Scenario assumptions: 20 station-read requests per monthly active user, 10% monthly contributors, four photos/contributor/month, average sanitized photo 0.5 MiB, 14-day retention. Under these assumptions:

- 1k MAU: 20k reads/month, 400 photos/month, ~93 MiB retained. Fixed VPS/backup/operator cost dominates; pilot can test single-host hypothesis.
- 10k MAU: 200k reads, 4k photos, ~0.91 GiB retained. Quotas, restore size and import/worker contention matter more than nominal user count.
- 100k MAU: 2M reads, 40k photos, ~9.1 GiB retained. Measure peaks, DB index/vacuum/WAL and cache-hit distribution; separate DB if evidence warrants.
- 1M MAU: 20M reads, 400k photos, ~91 GiB retained. Multiple API replicas/dedicated DB may be justified, but cannot be inferred from MAU alone; campaign peaks and contribution ratios can change demand by orders of magnitude.

Retention estimate = photos/month ×0.5 MiB×14/30; excludes originals, retries, orphans, snapshots and replication. CDN helps cacheable station reads; exact nearby queries remain origin-bound in v1. Direct object upload avoids API proxy traffic but worker validation still consumes bounded processing/network resources.

Scale out of the VPS when sustained CPU >70%, RAM >75%, pool wait p95 >50 ms, DB latency/IO dominates endpoint budget or queue delay exceeds 30 s under normal load after reasonable query/index/pool tuning. Collect at least representative peak windows and document measurements/alternatives/cost. Stage 2 dedicated DB; Stage 3 stateless API replicas/load balancing plus worker scaling. No infrastructure for hypothetical million-user load in v1.
