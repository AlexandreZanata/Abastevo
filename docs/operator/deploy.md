# Release deploy and rollback runbook (P08-T02)

Versioned monolith releases across migrate → api/worker/caddy
(INFRASTRUCTURE_PLAN deploy sequence). The orchestrator is
`infra/scripts/deploy.sh`; this runbook owns the decisions around it.
Production traffic additionally requires G09 certification: this
procedure alone never authorizes a production rollout.

## One release

```sh
ANPFUEL_RELEASE=2026.09.30-p08t02 \
ANPFUEL_PREVIOUS_RELEASE=2026.09.28-p08t01 \
ANPFUEL_ENV_FILE=/srv/anpfuel/staging.env \
ANPFUEL_COMPOSE_FILE=infra/compose.staging.yml \
ANPFUEL_BACKUP_MANIFEST=/srv/anpfuel/backups/latest.sha256 \
ANPFUEL_RECEIPT_FILE=infra/releases/2026.09.30-p08t02.md \
  bash infra/scripts/deploy.sh
```

1. Prechecks (refuse before any mutation): immutable non-`latest`
   release, previous release known (or explicit `none`), env file
   present, compose renders, clean tree behind the revision label
   (or explicit `ANPFUEL_REVISION`), fresh backup manifest unless an
   explicit reason sets `ANPFUEL_ALLOW_NO_BACKUP=1`.
2. Build all three roles from the pinned builder with
   `REVISION=<release commit>`; same revision label is the release
   provenance (skip with `ANPFUEL_NO_BUILD=1` on digest-pinned hosts).
3. One-shot `migrate` under the migration lock/checksum ledger.
4. Rollout `api worker caddy`, then the readiness gate
   (`/health/ready` 200 within `ANPFUEL_READINESS_TIMEOUT_SECONDS`),
   then smoke (`/health/live` 200 plus one public v1 read 200).
5. Any gate failure returns to `ANPFUEL_PREVIOUS_RELEASE` and
   re-verifies its readiness: exit 1 with `RESULT=rolled-back`.
   Rollback is a forward redeploy of the prior compatible binary;
   destructive down migrations never exist.

The receipt records release, previous, revision and result: no
secrets, digests or DSNs. Record commit, image digests (`docker
images --digests`), schema version (`migrate` output), config/policy
versions and operator alongside it.

## Rollback decisions

- Failed migration/readiness/smoke: automatic via the orchestrator.
- Degraded service after a green deploy: re-run the orchestrator with
  `ANPFUEL_RELEASE=<previous>` and `ANPFUEL_PREVIOUS_RELEASE=none`.
- Corruption requiring restoration: disable writes, preserve a
  forensic snapshot, restore into a new volume/instance, replay the
  deletion ledger, then switch traffic (see [recovery.md](recovery.md)).
- Pause the affected worker type when jobs pile behind a bad release;
  retained accepted jobs replay through the durable queue.

## CI publish design (not yet implemented)

CI_PLAN owns workflow cadence; no workflow changes land in this task.
The designed publish job, for a future reviewed change:

- Trigger: version tag push (`vYYYY.MM.DD-*`).
- Steps: quick-verify → build all roles with `REVISION=$GITHUB_SHA`
  → vulnerability/image scan → push by digest to the private registry
  → attach digest receipt to the release.
- Branch protection keeps requiring `Quick verification` on the tag's
  commit; the job never bypasses it and never publishes `latest`.
- Staging deploy remains an explicit operator invocation of
  `deploy.sh` with the published digests, not an automatic push.

## Staging drill (synthetic data)

`bash scripts/tests/test-deploy.sh` exercises every gate failure with
stubbed docker/curl (9/9 green, no containers). The live drill runs on
the staging host after provisioning: boot the topology, run the
orchestrator, then force each failure (broken migration image tag,
unhealthy release, failing smoke endpoint) and verify automatic return
to the previous release plus honest receipts. Record the drill
evidence before claiming P08-T02 acceptance on staging.
