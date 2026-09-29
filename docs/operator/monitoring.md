# Monitoring operations (P08-T05)

Metrics, alerts and the operator notification route for the service
and data-quality budgets in [INFRASTRUCTURE_PLAN](../backend/INFRASTRUCTURE_PLAN.md).
Alerts fire on codes and counts, never on identifiers.

## Metrics endpoint

- Private listener only: `ANPFUEL_METRICS_ADDR` (default
  `127.0.0.1:9090`, empty disables). Staging/production topologies
  publish no metrics port; scrape from the host or a private
  collector. Responses carry `Cache-Control: no-store`.
- Series (all bounded; route is the chi template, never the raw path):

```text
http_requests_total{method,route,class}
http_request_seconds_total{method,route,class}
db_up db_pool_acquired db_pool_idle
jobs_queued jobs_dead jobs_oldest_queued_seconds
metrics_dropped_total
```

- Label policy: methods from a fixed verb set (`OTHER` folds the
  rest), classes `2xx/4xx/5xx/other`, routes from registered
  templates plus `unmatched`. Contributor, station and observation
  IDs cannot become label values (pinned by unit test); per-metric
  series cap 512 drops runaways loudly via `metrics_dropped_total`.
- Gauges report `NaN` (unknown) when a scrape-time read fails, never
  a stale value.

## PromQL starters

```promql
# 5xx spike (share of requests, 5m window)
sum(rate(http_requests_total{class="5xx"}[5m]))
  / sum(rate(http_requests_total[5m]))

# Admission latency (average seconds per class)
sum(rate(http_request_seconds_total[5m])) by (class)
  / sum(rate(http_requests_total[5m])) by (class)

# Pool saturation
db_pool_acquired / (db_pool_acquired + db_pool_idle)

# Queue backlog and dead letters
jobs_queued
jobs_oldest_queued_seconds
jobs_dead
```

## Alert catalog

| Rule | Severity | Signal | Response |
|---|---|---|---|
| `db_down` | critical | `db_up == 0` (pool ping fails) | Writes already 503 with readiness false; restore connectivity/disk per the failure-mode runbook; never pretend writes stored |
| `jobs_dead` | critical | `jobs_dead > 0` | Inspect dead payloads, fix the cause, `ReplayDead` with an audited reason; poison jobs park, never auto-delete |
| `jobs_stuck` | warning | oldest queued older than 5 min | Check worker liveness and slow handlers; lease expiry reclaims crashed work; scale or tune before queue delay exceeds 30 s |
| `backup_stale` | critical | newest manifest older than 26 h | Repair the backup pipeline and key custody before accepting production writes; repeat the restore drill |

Evaluation deduplicates per rule within 15 minutes (cooldown), so a
stuck firing rule pages once per window instead of flooding. Check
errors fail loudly: a blind rule never counts as green.

## Notification route

```sh
# One-shot evaluation (exit 0 quiet, exit 1 when anything fired)
ANPFUEL_DATABASE_URL=... ANPFUEL_BACKUP_MANIFEST=/srv/anpfuel/backups/latest.sha256.json \
  go run ./cmd/ops monitoring eval --webhook https://ops-relay.example.invalid/alerts
```

- Without `--webhook` (or `ANPFUEL_MONITORING_WEBHOOK`), alerts go to
  the service log under the 14-day operational retention.
- With a webhook, every firing alert POSTs once as fixed-field JSON
  (`name/severity/detail/fired_at`); non-2xx responses fail loudly
  for retry. Establish one tested operator recipient during
  provisioning; never send real notifications without one.
- Cron the evaluation every 5 minutes on the monitoring host (not on
  the API host): `*/5 * * * * ops monitoring eval ...`.

## Label leak response

If a personal label is ever observed in exposition: disable the
offending collector (empty `ANPFUEL_METRICS_ADDR` stops everything),
purge the collector series, fix the label source with a regression
test, and adjust alerts to code-based signals without hiding the
underlying failure. Roll back the insecure deployment first if the
leak reached shared storage.
