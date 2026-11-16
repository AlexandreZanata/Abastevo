# Abastevo Empresas activation on the VPS (runtime role, switch and rollback)

Scope: namespace `abastevo-temp` (`ANPFUEL_ENV=staging`, public at
`https://abastevo.com.br`). Authority: owner decision EDR-272 ("enable
enterprise on the VPS", a manual change after the automatic deploy, with a
written rollback and the owner's final confirmation). This runbook implements
B-BR-EMP-556..562, BUC-EMP-285..287 and EDR-363..368; the source evidence is
[f4-enterprise-activation](../enterprise/evidence/f4-enterprise-activation.md).
It is not a release certification, does not accept EMP-REL-001/002 or M4 and
does not authorize real-company data: verification uses synthetic
organizations only and tracking stays absent or `OFF` for every real
organization (EDR-362).

Never print, paste or commit a password, a DSN with credentials, an account
key or a session token. Every command below prints names, schemes, counts or
HTTP codes only.

## What changes

| Object | Before | After | Rollback |
|---|---|---|---|
| PostgreSQL role `abastevo_runtime` | absent | `LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION`, owns nothing, grants of `infra/db/runtime-role.sql` | may stay (additive) |
| Secret `abastevo-temp-secrets` | `ANPFUEL_DATABASE_URL` = owner DSN | same, plus `ANPFUEL_RUNTIME_DATABASE_URL` | may stay; `ANPFUEL_DATABASE_URL` is never touched |
| Deployment `abastevo-temp-api`, containers `api` and `worker` | owner DSN through `envFrom`; switch unset | `env` `ANPFUEL_DATABASE_URL` from key `ANPFUEL_RUNTIME_DATABASE_URL`; `ANPFUEL_ENTERPRISE_ENABLED=true` | delete both entries |
| Deploy agent Jobs (schema, backup, migrate), registry CronJob | owner DSN | unchanged (owner DSN) | — |
| Synthetic smoke organization | absent | created, capability enabled (revision 1), proven, disabled again (revision 2); two audit events | stays as synthetic staging data |

The deploy agent sets only images, pull policy, strategy and annotations, and
its `rollout undo` targets the revision it recorded before its own image
change, so the hand-set entries survive later releases. The desired state is
recorded in [api-deployment.yaml](../../infra/k8s/staging/api-deployment.yaml).

## Stop conditions

Do not start, or roll back at once, when any of these holds: the owner's final
confirmation is missing; the deployed release does not carry the runtime
script for the live schema version; `ANPFUEL_R2_ENDPOINT` is set and not
`https` (with the switch on, the API and worker refuse to start, see EDR-366);
the backup Job fails or its checksum differs; the script refuses; the rollout
is not ready within 300 s; a smoke answer differs; the database log shows
`permission denied` after the rollout.

## 0. Session (root on the VPS host)

```sh
set -euo pipefail
set +o history
umask 077
WORK="$(mktemp -d)"; cd "$WORK"
NS=abastevo-temp DEPLOY=abastevo-temp-api SECRET=abastevo-temp-secrets
DBPOD=abastevo-temp-db-0 ROLE=abastevo_runtime BASE=https://abastevo.com.br
SHA="$(jq -r .sha /var/lib/abastevo-deploy/current.json)"   # deployed release
OPERATOR_ID=<canonical UUID of the operator, kept in the coordinator ledger>
```

Hold the deploy agent for the whole operation; its timer runs then defer:

```sh
exec 9>/var/lib/abastevo-deploy/apply.lock
flock -n 9 || { echo "deploy agent busy: wait and retry"; exit 1; }
```

## 1. Pre-flight (read-only)

```sh
# The release must carry the script; read it from a clean checkout of $SHA.
# CHECKOUT is the coordinator's clean clone of the repository.
git -C "$CHECKOUT" fetch --quiet origin
git -C "$CHECKOUT" show "$SHA:infra/db/runtime-role.sql" > runtime-role.sql
SCRIPT_SCHEMA="$(sed -n 's/^-- Schema version: \([0-9]\{6\}\)\..*/\1/p' runtime-role.sql)"
OWNER_DSN_PARTS="$(kubectl -n "$NS" get secret "$SECRET" -o jsonpath='{.data.ANPFUEL_DATABASE_URL}' | base64 -d \
  | python3 -c 'import sys,urllib.parse as u; d=u.urlsplit(sys.stdin.read().strip()); print(d.username, d.path.lstrip("/"))')"
OWNER_USER="${OWNER_DSN_PARTS% *}" DB_NAME="${OWNER_DSN_PARTS#* }"
DB_SCHEMA="$(kubectl -n "$NS" exec "$DBPOD" -- psql -U "$OWNER_USER" -d "$DB_NAME" -tAc 'SELECT max(version) FROM schema_migrations')"
test "$SCRIPT_SCHEMA" = "$DB_SCHEMA" && echo "schema $DB_SCHEMA matches the script"
# Only the scheme is printed: it must be https (or the key absent).
kubectl -n "$NS" get secret "$SECRET" -o jsonpath='{.data.ANPFUEL_R2_ENDPOINT}' | base64 -d | cut -d: -f1; echo
# The owner login must be a superuser and PUBLIC must not create in schema public.
kubectl -n "$NS" exec "$DBPOD" -- psql -U "$OWNER_USER" -d "$DB_NAME" -tAc \
  "SELECT rolsuper, has_schema_privilege('public','public','CREATE'), current_setting('log_statement') FROM pg_roles WHERE rolname=current_user"
# Current serving revision and the env names of both containers (no values).
kubectl -n "$NS" get deploy "$DEPLOY" -o jsonpath='{.metadata.annotations.deployment\.kubernetes\.io/revision}'; echo
kubectl -n "$NS" get deploy "$DEPLOY" -o json | jq -c '.spec.template.spec.containers[] | {name, env: [.env[].name]}'
```

Expected: `schema … matches`, `https` (or an empty line), `t|f|none` (or
`mod`), and neither container lists `ANPFUEL_ENTERPRISE_ENABLED` yet. Record the
revision number in the receipt.

## 2. Verified backup (the deploy agent's backup Job)

```sh
BACKUP_DIR="$(sed -n 's/^ABASTEVO_BACKUP_DIR=//p' /etc/abastevo-deploy/agent.env)"
DB_IMAGE='postgis/postgis:18-3.6@sha256:60f6ad1d21ea86a67d47780b9a0d1e1d200500f62b19293fa834d0dea80b8677'
. /usr/local/lib/abastevo-deploy-agent/runtime/k8s.sh
NAME="activation-$(date -u +%Y%m%dT%H%M%SZ)"
runtime_backup "$NS" "backup-activation-$(date -u +%H%M%S)" "$DB_IMAGE" "$SECRET" "$BACKUP_DIR" "$NAME" "$SHA" 300
test "$(tr -d '[:space:]' < "$BACKUP_DIR/$NAME.dump.sha256")" = "$(sha256sum "$BACKUP_DIR/$NAME.dump" | cut -d' ' -f1)" \
  && echo "backup $NAME verified"
```

The `activation-` prefix keeps it out of the agent's `pre-*` retention.

## 3. Runtime role and grants (as the owner)

```sh
kubectl -n "$NS" exec -i "$DBPOD" -- psql -U "$OWNER_USER" -d "$DB_NAME" \
  -v ON_ERROR_STOP=1 -v runtime_role="$ROLE" -f - < runtime-role.sql
```

The script runs in one transaction, refuses another schema version, an owning
or privileged role and the owner itself, and ends with `COMMIT` only after its
own verification. Re-running it is safe.

## 4. Password and the new secret key

```sh
RUNTIME_PASSWORD="$(openssl rand -hex 32)"
printf "SET log_statement='none'; SET log_min_duration_statement=-1; SET log_min_error_statement=panic; ALTER ROLE %s PASSWORD '%s';\n" \
  "$ROLE" "$RUNTIME_PASSWORD" \
  | kubectl -n "$NS" exec -i "$DBPOD" -- psql -U "$OWNER_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 -q
cat > runtime-dsn.py <<'PY'
import base64, json, os, sys, urllib.parse as u
owner = u.urlsplit(sys.stdin.read().strip())
host = owner.hostname + (":%d" % owner.port if owner.port else "")
netloc = "%s:%s@%s" % (os.environ["ROLE"], os.environ["RUNTIME_PASSWORD"], host)
runtime = u.urlunsplit(owner._replace(netloc=netloc))
print(json.dumps({"data": {"ANPFUEL_RUNTIME_DATABASE_URL": base64.b64encode(runtime.encode()).decode()}}))
PY
kubectl -n "$NS" get secret "$SECRET" -o jsonpath='{.data.ANPFUEL_DATABASE_URL}' | base64 -d \
  | ROLE="$ROLE" RUNTIME_PASSWORD="$RUNTIME_PASSWORD" python3 runtime-dsn.py \
  | kubectl -n "$NS" patch secret "$SECRET" --type=merge --patch-file=/dev/stdin
unset RUNTIME_PASSWORD
kubectl -n "$NS" get secret "$SECRET" -o json | jq -r '.data | keys[]' | grep -E '^ANPFUEL_(RUNTIME_)?DATABASE_URL$'
```

Expected: both key names listed. The hexadecimal password needs no URL
escaping and the server stores only its SCRAM verifier.

## 5. Switch the serving containers

```sh
kubectl -n "$NS" patch deploy "$DEPLOY" --type=strategic -p '{"spec":{"template":{"spec":{"containers":[
 {"name":"api","env":[{"name":"ANPFUEL_DATABASE_URL","valueFrom":{"secretKeyRef":{"name":"abastevo-temp-secrets","key":"ANPFUEL_RUNTIME_DATABASE_URL"}}},{"name":"ANPFUEL_ENTERPRISE_ENABLED","value":"true"}]},
 {"name":"worker","env":[{"name":"ANPFUEL_DATABASE_URL","valueFrom":{"secretKeyRef":{"name":"abastevo-temp-secrets","key":"ANPFUEL_RUNTIME_DATABASE_URL"}}},{"name":"ANPFUEL_ENTERPRISE_ENABLED","value":"true"}]}]}}}}'
kubectl -n "$NS" rollout status deploy/"$DEPLOY" --timeout=300s
```

With `maxUnavailable: 0` the previous pod keeps serving until the new one is
ready; a new pod that cannot log in or read the schema ledger never becomes
ready. If the rollout does not finish, go to Rollback.

## 6. Smoke

```sh
code() { curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "$@"; }
for path in /health/ready '/v1/stations?limit=1' /v1/forum/categories \
  "/v1/map/assertions?min_lat=-16&min_lon=-48&max_lat=-15&max_lon=-47&source=ANP"; do
  echo "$path $(code "$BASE$path")"                                   # 200 each
done
post() { curl -sS --max-time 10 -b jar -c jar -H 'Content-Type: application/json' -H "Origin: $BASE" "$@"; }
echo "unauthenticated $(code -X POST -H 'Content-Type: application/json' -H "Origin: $BASE" --data '{}' "$BASE/v1/org-access/organizations/setups/list")"  # 401
# Synthetic organization (records stay as synthetic staging data).
USERNAME="synact$(date -u +%y%m%d%H%M)"
KEY="$(post --data "{\"username\":\"$USERNAME\"}" "$BASE/v1/accounts/keys" | jq -r .account_key)"
SESSION="$(post --data "{\"account_key\":\"$KEY\"}" "$BASE/v1/accounts/keys/login" | jq -c '.session | {family_id, access_token}')"
unset KEY
CSRF="$(post --data "$SESSION" "$BASE/v1/org-access/session/login" | jq -r .csrf_token)"
op() { python3 -c 'import uuid; print(uuid.uuid4())'; }
ORG="$(post -H "X-CSRF: $CSRF" --data "{\"schema_version\":1,\"operation_id\":\"$(op)\",\"name\":\"Synthetic activation probe\",\"time_zone\":\"America/Cuiaba\",\"kinds\":[\"OTHER\"],\"unit_name\":\"Synthetic main unit\",\"terms\":{\"version\":\"enterprise-synthetic-dpa-v1\",\"digest\":\"590cc4cf31e1909ac9baacb76092897f3dba07b95719953b0f4588b51348f8b3\",\"accepted\":true}}" \
  "$BASE/v1/org-access/organizations/create" | jq -r .organization_id)"
echo "organization $ORG"                                                  # a UUID
post -H "X-CSRF: $CSRF" --data '{}' "$BASE/v1/orgs/$ORG/settings/read" | jq -c '.context.capability_enabled'   # false
unit() { post -H "X-CSRF: $CSRF" --data "{\"schema_version\":1,\"operation_id\":\"$(op)\",\"name\":\"Synthetic unit $1\",\"code\":\"$1\"}" "$BASE/v1/orgs/$ORG/units/create"; }
unit REFUSED | jq -r .error.code                                          # ENTITLEMENT_REQUIRED
# Operator capability command (BUC-EMP-287): the api container's /ops runs
# with that container's runtime DSN; it refuses the owner login by design.
capability() { kubectl -n "$NS" exec deploy/"$DEPLOY" -c api -- /ops enterprise capability "$@" \
  --org "$ORG" --capability organizations --operator "$OPERATOR_ID"; }
capability show                                         # ... enabled=false revision=0 replayed=false
ENABLE_OP="$(op)"
capability set --enabled --operation-id "$ENABLE_OP" --expected-revision 0    # ... enabled=true revision=1 replayed=false
capability set --enabled --operation-id "$ENABLE_OP" --expected-revision 0    # same line with replayed=true (safe retry)
post -H "X-CSRF: $CSRF" --data "{\"schema_version\":1,\"operation_id\":\"$(op)\",\"revision\":1}" \
  "$BASE/v1/orgs/$ORG/organization/activate" | jq -r .state                 # ACTIVE
unit SECOND | jq -r .unit_id                                              # a UUID: the write now succeeds
DISABLE_OP="$(op)"
capability set --disabled --operation-id "$DISABLE_OP" --expected-revision 1  # ... enabled=false revision=2 replayed=false
unit THIRD | jq -r .error.code                                            # ENTITLEMENT_REQUIRED again
unset SESSION CSRF; rm -f jar
kubectl -n "$NS" exec "$DBPOD" -- psql -U "$OWNER_USER" -d "$DB_NAME" -tAc \
  "SELECT action, actor_id, reason_code FROM org_audit_events WHERE org_id='$ORG' AND action LIKE 'capability.%' ORDER BY recorded_at"
                                                        # capability.enabled|<operator>|operator.request, then capability.disabled
kubectl -n "$NS" logs "$DBPOD" --since=15m | grep -c 'permission denied' || true   # 0
```

The synthetic organization ends capability-off (revision 2), so no
organization is left enabled after the smoke (EDR-368). To enable it, or a
named pilot organization once approved, run `capability show` and then
`capability set --enabled` with a new operation ID and the revision shown. A
stale revision answers a conflict and changes nothing; reusing an operation ID
for another command is refused; the same command retried is a replay.

## 7. Release the agent and record

```sh
exec 9>&-
cd /; rm -rf "$WORK"
```

Record in the coordinator ledger (no secrets): date, `$SHA`, revision before
and after, backup name, the synthetic organization identifier, the operator
UUID, both capability operation IDs and revisions with the reason for the
change, every smoke code and the `permission denied` count.

## Rollback

Hold the agent lock as in step 0, then remove only the two entries (images and
everything else stay; never use `rollout undo`, which could revert a newer
image):

```sh
kubectl -n "$NS" patch deploy "$DEPLOY" --type=strategic -p '{"spec":{"template":{"spec":{"containers":[
 {"name":"api","env":[{"name":"ANPFUEL_DATABASE_URL","$patch":"delete"},{"name":"ANPFUEL_ENTERPRISE_ENABLED","$patch":"delete"}]},
 {"name":"worker","env":[{"name":"ANPFUEL_DATABASE_URL","$patch":"delete"},{"name":"ANPFUEL_ENTERPRISE_ENABLED","$patch":"delete"}]}]}}}}'
kubectl -n "$NS" rollout status deploy/"$DEPLOY" --timeout=300s
curl -sS -o /dev/null -w '%{http_code}\n' "$BASE/health/ready"            # 200
curl -sS -o /dev/null -w '%{http_code}\n' -X POST -H 'Content-Type: application/json' -H "Origin: $BASE" \
  --data '{}' "$BASE/v1/org-access/organizations/setups/list"            # 404: routes no longer served
exec 9>&-
```

Nothing stored is deleted by either switch. The role, its grants and the
secret key are additive and may stay. Remove them only when no pod uses them:
`kubectl -n "$NS" patch secret "$SECRET" --type=json -p '[{"op":"remove","path":"/data/ANPFUEL_RUNTIME_DATABASE_URL"}]'`
and, as the owner, `DROP OWNED BY abastevo_runtime; DROP ROLE abastevo_runtime;`.
Restoring the activation backup is a separate, explicitly approved recovery
([recovery](../operator/recovery.md)), never part of this rollback.

## After every later release

When `git diff <previous SHA> <new SHA> -- infra/db/runtime-role.sql` is not
empty, apply the new release's script (steps 0, 1 and 3 only, under the agent
lock) right after the agent has deployed it, then repeat the
`permission denied` check. Until then, tables that the release added already
have ordinary DML through the default privileges; a new privilege on an
existing table is missing and that path answers unavailable (EDR-364). The
password and the secret do not change.

## Owed before real organizations

Scheduling of `identity_rate_windows` cleanup (EMP-PLAT-014);
measured production quota classes; EMP-REL-001 and the VAL-24 switch
rehearsal; the M4 device matrix before any tracking (EDR-361/362).
