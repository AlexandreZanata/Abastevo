# Staging k3s source of truth (R2)

These files are the captured live objects in namespace `abastevo-temp` as of
2026-10-27. They are review artifacts and the baseline for the executor; they
are not applied by this directory's tooling.

The reviewed executor RBAC lives in `rbac/deploy-agent-rbac.yaml`; it is not
part of the captured-object diff and is not applied here.

Captured objects:

- `api-deployment.yaml` — Deployment `abastevo-temp-api` (api, worker and
  private-storage containers; one rollout updates api and worker together).
- `web-deployment.yaml` — Deployment `abastevo-temp-web` (Caddy).
- `services.yaml` — NodePort/ClusterIP Services for api, db and web.
- `db-statefulset.yaml` — StatefulSet `abastevo-temp-db`.
- `resourcequota.yaml` — ResourceQuota `abastevo-temp-quota`.
- `registry-sync-cronjob.yaml` — CronJob `abastevo-registry-sync`.
- `web-caddy-configmap.yaml` — ConfigMap `abastevo-temp-web-caddyfile`.

The StatefulSet references the pre-existing PVC `abastevo-temp-postgres-data`.
The PVC/PV are not templated here.

Secrets are referenced only through `secretRef`/`configMapRef`. No secret
value, host, IP, NodePort, image tag or operator path is committed.

## Placeholders

The render file is operator-owned, outside Git, and mode 600. Keys:

```text
API_IMAGE WORKER_IMAGE PRIVATE_STORAGE_IMAGE CADDY_IMAGE
RST_COMPATIBLE_BINARIES_PATH PRIVATE_MEDIA_PATH ENTERPRISE_ROOT
LANDING_ROOT PORTAL_MOUNT_PATH RST_TOOLS_MOUNT LANDING_MOUNT
EXPORT_ROOT PORTAL_ROOT PORTAL_SEARCH_JS
STAGING_SITE_URL STAGING_LANDING_URL WORKER_METRICS_ADDR
API_CLUSTER_IP DB_CLUSTER_IP WEB_CLUSTER_IP
API_NODE_PORT WEB_NODE_PORT
API_DEPLOYMENT_REVISION WEB_DEPLOYMENT_REVISION
API_POD_RESTARTED_AT WEB_POD_RESTARTED_AT
REGISTRY_SYNC_RELEASES_PATH REGISTRY_SYNC_STATE_PATH
MAP_DIST_ROOT
```

`render.py` replaces only these `${NAME}` placeholders; pod shell variables
such as `$POSTGRES_USER` remain untouched.

## Zero-diff proof

Render locally:

```sh
STAGING_RENDER_ENV=/path/outside/git/staging-render.env \
  python3 infra/k8s/staging/render.py infra/k8s/staging/*.yaml \
  > /tmp/staging-rendered.yaml
```

Copy `normalize-diff.py` to the staging host's allowed temporary location and
run the semantic diff. The normalizer removes only server-managed metadata and
client-apply bookkeeping (creation timestamp, generation, resource version,
UID, managed fields, status, last-applied annotation, deployment revision and
manual restart timestamp); spec, labels, desired annotations, data and image
pull policies are compared.

```sh
ssh <staging-host> \
  'KUBECTL_EXTERNAL_DIFF=<host-temp>/normalize-diff.py \
   kubectl diff -n abastevo-temp -f -' \
  < /tmp/staging-rendered.yaml
```

The proof run exited 0 with empty output. Raw `kubectl diff` without the
normalizer reports only those server-managed fields.

No `kubectl apply` is run by this directory. The executor later patches images
and source annotations; it does not change the namespace, quota, database,
Services or CronJob.
