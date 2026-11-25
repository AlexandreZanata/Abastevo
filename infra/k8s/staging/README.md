# Staging k3s manifests

Reviewed desired state of the staging namespace `abastevo-temp` on the k3s
cluster of VPS 72.60.147.2 (https://abastevo.com.br). Nothing in this directory
applies them: operators apply reviewed changes by hand, and the deploy agent
only patches images and switches the landing release
([agent](../../deploy/agent/README.md)).

## Files

| File | Object |
|------|--------|
| `api-deployment.yaml` | Deployment `abastevo-temp-api`: `api` and `worker` containers, rolled out together. |
| `storage-deployment.yaml`, `storage-service.yaml`, `storage-configmap.yaml` | Singleton private S3 storage `abastevo-temp-storage` (`Recreate`, ClusterIP only, start script). |
| `web-deployment.yaml`, `web-caddy-configmap.yaml` | Web Caddy serving the landing, portal, `/admin/` and `/empresas/`; the ConfigMap mirrors `infra/caddy/staging-web.Caddyfile`. |
| `db-statefulset.yaml` | StatefulSet `abastevo-temp-db` on the pre-existing PVC `abastevo-temp-postgres-data` (not templated). |
| `services.yaml`, `resourcequota.yaml` | Services and ResourceQuota `abastevo-temp-quota`. |
| `registry-sync-cronjob.yaml` | CronJob `abastevo-registry-sync` (daily official registry refresh). |
| `rbac/deploy-agent-rbac.yaml` | Namespace-scoped ServiceAccount/Role/RoleBinding of the deploy agent. |
| `render.py`, `normalize-diff.py`, `test-manifests.py` | Placeholder renderer, semantic diff normalizer, structural tests. |

## Render and compare

Host-specific values (images, host paths, cluster IPs, NodePorts, portal and
landing roots, site URLs) are `${NAME}` placeholders filled from an
operator-owned env file outside Git (mode 600); `render.py` replaces only
those, leaving pod shell variables alone.

```sh
make check-infra    # includes python3 infra/k8s/staging/test-manifests.py
STAGING_RENDER_ENV=/path/outside/git/staging-render.env \
  python3 infra/k8s/staging/render.py infra/k8s/staging/*.yaml > /tmp/staging-rendered.yaml
# on the staging host, with normalize-diff.py copied to a temporary path:
KUBECTL_EXTERNAL_DIFF=<tmp>/normalize-diff.py kubectl diff -n abastevo-temp -f - < staging-rendered.yaml
```

The normalizer drops only server-managed metadata (timestamps, UIDs, resource
versions, managed fields, status, revision and restart annotations). An empty
diff means live state equals this directory; check it before assuming a
manifest change is live.

## Operator-managed settings

- Secrets come only through `secretRef`/`configMapRef` of `abastevo-temp-secrets`;
  no value is committed. `api-deployment.yaml` declares that API and worker
  read `ANPFUEL_DATABASE_URL` from the secret key `ANPFUEL_RUNTIME_DATABASE_URL`
  (the ordinary role provisioned by `infra/db/runtime-role.sql`) and set
  `ANPFUEL_ENTERPRISE_ENABLED=true`; the secret's own `ANPFUEL_DATABASE_URL`
  stays the owner DSN used by the agent's backup/migration Jobs and the
  registry CronJob. Enterprise routes must never run on the owner DSN.
- Admin API: patch `ANPFUEL_ADMIN_KEY_SHA256` and
  `ANPFUEL_ADMIN_ORIGIN=https://admin.abastevo.com.br` into the Secret (key
  from `go run ./cmd/ops admin keygen`; rotate via
  `ANPFUEL_ADMIN_KEY_SHA256_NEXT`) and restart the API. The panel is
  `https://admin.abastevo.com.br/admin/login/`; the apex `/admin/` redirects
  there and the edge answers `/v1/admin/*` only on that host. The optional
  `ANPFUEL_ADMIN_ALLOWED_CIDRS` fence holds the operator's own network (kept
  out of Git); when that address changes, patch it and restart the API.
- Client IPs: Caddy reaches the API through the loopback NodePort, which k3s
  source-NATs to the CNI gateway, so the API sees every request from that one
  address (default `10.42.0.1`). Set `ANPFUEL_TRUSTED_PROXIES` to exactly that
  address (`10.42.0.1/32`) so per-IP quotas and the admin fence use the
  client IP Caddy forwards. Never trust a wider range: the API would accept
  forwarded headers from in-cluster callers.
- Seeds (`backend/db/seeds/*.sql`) are idempotent dev/staging data, applied by
  hand with `psql -v ON_ERROR_STOP=1` inside the db pod after the migrations
  they depend on.
- The registry CronJob needs the verified offline OpenCNPJ source directory
  (`REGISTRY_RFB_SOURCE_PATH`, mounted read-only) before it is enabled; see
  [registry refresh](../../deploy/registry/README.md).
- Private storage is a single writer: stop the old writer before starting a
  new one; never run two at once.

## Rules

- No secret value, private host or operator path in these files.
- Keep `terminationGracePeriodSeconds` above shutdown drain delay + timeout +
  5 s (see `backend/README.md`).
- Do not change the namespace, quota, database or Services as a side effect of
  an image change.

See [operations runbooks](../../../docs/operations/) and
[registry daily sync](../../../docs/operations/registry-daily-sync.md).
