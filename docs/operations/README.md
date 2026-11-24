# Operations

## Current deployment

- **Host:** one VPS (shared with an unrelated service that must never be
  touched). Abastevo runs isolated in k3s, namespace `abastevo-temp`, with
  NodePorts bound to `127.0.0.1` only.
- **Edge:** Cloudflare (proxied, Full TLS) → host Caddy, which imports one
  Abastevo site file and serves `abastevo.com.br` (landing, portal, forum),
  `www.` (redirect), `api.` (`/v1`, `/health`) and `admin.`.
- **Data:** PostgreSQL + PostGIS StatefulSet and a private S3-compatible
  storage deployment, both inside the namespace.
- **Releases:** merge to `main` → CI → `release-staging` publishes a signed
  manifest `ghcr.io/alexandrezanata/abastevo-release:staging` (image digests,
  landing bundle, schema range) only when every main workflow passed. A
  merge that lands while the previous commit is still in CI cancels that
  commit's runs: the release gate stays closed for it without failing, and
  the newer commit carries its changes. The
  `abastevo-deploy-agent` timer on the VPS (every 2 minutes) verifies the
  signature, checks the schema gate, takes a verified backup, runs the migrate
  Job, rolls out images, switches the landing and smoke-tests, writing a receipt.
  Code: [`infra/deploy/agent`](../../infra/deploy/agent/), manifests:
  [`infra/k8s/staging`](../../infra/k8s/staging/).
- **Not managed by the agent:** worker environment variables set by hand, the
  web Caddy ConfigMap (`infra/k8s/staging/web-caddy-configmap.yaml`, applied
  manually) and the registry-sync tool releases on the host.
- **Android:** `android-release` builds, signs and publishes GitHub
  prereleases from `main`.

## Runbooks

| Topic | Runbook |
|-------|---------|
| Backups | [backup.md](backup.md) |
| Restore and recovery | [recovery.md](recovery.md) |
| Monitoring | [monitoring.md](monitoring.md) |
| Edge cache | [edge-cache.md](edge-cache.md) |
| Operator access | [operator-access.md](operator-access.md) |
| Daily station registry sync | [registry-daily-sync.md](registry-daily-sync.md), [registry-runs.md](registry-runs.md), [dou-editions.md](dou-editions.md) |
| Moderation | [moderation-feedback.md](moderation-feedback.md), [moderation-profiles.md](moderation-profiles.md) |
| Static map overlay | [map-static-overlay.md](map-static-overlay.md) |
| Payments (sales off) | [payments.md](payments.md), [payment-providers-guide.md](payment-providers-guide.md), [premium-billing.md](premium-billing.md) |
| Abastevo Empresas activation | [enterprise-activation.md](enterprise-activation.md) |
| Account sign-in | [account-sign-in.md](account-sign-in.md) |

## Rules

- Never run destructive tests against the staging database; use a disposable
  PostGIS locally.
- Check the shared host's other service before and after any Caddy reload.
- Secrets live only on the host and in GitHub secrets; never in the repository,
  logs or receipts.
