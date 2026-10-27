#!/usr/bin/env bash
# Kubernetes runtime adapter for the staging deploy executor.
# shellcheck disable=SC2034
#
# All cluster mutation lives behind these functions. The dry-run path never
# sources this file. Jobs carry ttlSecondsAfterFinished=86400 and backoffLimit=0.

runtime_kubectl() {
    "${ABASTEVO_KUBECTL:-kubectl}" "$@"
}

runtime_apply_job() {
    runtime_kubectl apply -f - >/dev/null
}

runtime_wait_job() {
    local namespace="$1" job="$2" timeout="$3"
    runtime_kubectl -n "$namespace" wait \
        --for=condition=complete --timeout="${timeout}s" "job/$job" >/dev/null
}

# Informational only. The executor never uses this annotation as the gate.
runtime_schema_annotation() {
    local namespace="$1" deployment="$2"
    runtime_kubectl -n "$namespace" get deployment "$deployment" -o json \
        | jq -r '.metadata.annotations["abastevo.com/schema-version"] // empty'
}

runtime_schema_version() {
    local namespace="$1" job="$2" image="$3" secret="$4" sha="$5" timeout="$6"
    if ! runtime_kubectl apply -f - >/dev/null <<YAML
apiVersion: batch/v1
kind: Job
metadata:
  name: $job
  namespace: $namespace
  labels:
    app.kubernetes.io/managed-by: abastevo-deploy-agent
    abastevo.com/release-sha: "$sha"
    abastevo.com/role: schema
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 86400
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: abastevo-deploy-agent
        abastevo.com/role: schema
    spec:
      automountServiceAccountToken: false
      restartPolicy: Never
      containers:
        - name: schema
          image: $image
          imagePullPolicy: IfNotPresent
          envFrom:
            - secretRef:
                name: $secret
          command: ["sh", "-c", "psql \"\$ANPFUEL_DATABASE_URL\" -tAc 'SELECT max(version) FROM schema_migrations'"]
YAML
    then
        return 1
    fi
    runtime_wait_job "$namespace" "$job" "$timeout" || return 1
    runtime_kubectl -n "$namespace" logs "job/$job" \
        | grep -E '^[0-9]{6}$' | tail -n 1
}

runtime_backup() {
    local namespace="$1" job="$2" image="$3" secret="$4" backup_dir="$5" backup_name="$6" sha="$7" timeout="$8"
    if ! runtime_kubectl apply -f - >/dev/null <<YAML
apiVersion: batch/v1
kind: Job
metadata:
  name: $job
  namespace: $namespace
  labels:
    app.kubernetes.io/managed-by: abastevo-deploy-agent
    abastevo.com/release-sha: "$sha"
    abastevo.com/role: backup
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 86400
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: abastevo-deploy-agent
        abastevo.com/role: backup
    spec:
      automountServiceAccountToken: false
      restartPolicy: Never
      containers:
        - name: backup
          image: $image
          imagePullPolicy: IfNotPresent
          envFrom:
            - secretRef:
                name: $secret
          command:
            - sh
            - -c
            - |
              set -euo pipefail
              out="/backup/$backup_name.dump"
              pg_dump "\$ANPFUEL_DATABASE_URL" -Fc -f "\$out"
              pg_restore --list "\$out" >/dev/null
              sha256sum "\$out" | cut -d' ' -f1 > "\$out.sha256"
          volumeMounts:
            - name: backup
              mountPath: /backup
      volumes:
        - name: backup
          hostPath:
            path: $backup_dir
            type: Directory
YAML
    then
        return 1
    fi
    runtime_wait_job "$namespace" "$job" "$timeout" || return 1
}

runtime_migrate() {
    local namespace="$1" job="$2" image="$3" secret="$4" sha="$5" timeout="$6"
    if ! runtime_kubectl apply -f - >/dev/null <<YAML
apiVersion: batch/v1
kind: Job
metadata:
  name: $job
  namespace: $namespace
  labels:
    app.kubernetes.io/managed-by: abastevo-deploy-agent
    abastevo.com/release-sha: "$sha"
    abastevo.com/role: migrate
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 86400
  template:
    metadata:
      labels:
        app.kubernetes.io/managed-by: abastevo-deploy-agent
        abastevo.com/role: migrate
    spec:
      automountServiceAccountToken: false
      restartPolicy: Never
      containers:
        - name: migrate
          image: $image
          imagePullPolicy: IfNotPresent
          envFrom:
            - secretRef:
                name: $secret
YAML
    then
        return 1
    fi
    runtime_wait_job "$namespace" "$job" "$timeout" || return 1
    runtime_kubectl -n "$namespace" logs "job/$job"
}

runtime_quota_cpu_headroom_milli() {
    local namespace="$1" quota="$2" hard used hard_milli used_milli
    hard="$(runtime_kubectl -n "$namespace" get resourcequota "$quota" -o json \
        | jq -r '.status.hard["limits.cpu"] // .spec.hard["limits.cpu"] // empty')"
    used="$(runtime_kubectl -n "$namespace" get resourcequota "$quota" -o json \
        | jq -r '.status.used["limits.cpu"] // "0"')"
    [[ -n "$hard" ]] || return 1
    hard_milli="$(cpu_to_milli "$hard")" || return 1
    used_milli="$(cpu_to_milli "$used")" || return 1
    if (( hard_milli > used_milli )); then
        printf '%d\n' $((hard_milli - used_milli))
    else
        printf '0\n'
    fi
}

cpu_to_milli() {
    local value="$1"
    case "$value" in
        *m) printf '%d\n' "${value%m}" ;;
        *.*) awk -v x="$value" 'BEGIN { printf "%d\n", x * 1000 }' ;;
        ''|*[!0-9]*) return 1 ;;
        *) printf '%d\n' $((value * 1000)) ;;
    esac
}

runtime_deployment_images() {
    local namespace="$1" deployment="$2"
    runtime_kubectl -n "$namespace" get deployment "$deployment" -o json \
        | jq -c '[.spec.template.spec.containers[] | {name, image, imagePullPolicy}]'
}

runtime_deployment_strategy() {
    local namespace="$1" deployment="$2"
    runtime_kubectl -n "$namespace" get deployment "$deployment" -o json \
        | jq -c '.spec.strategy.rollingUpdate // {}'
}

runtime_patch_strategy() {
    local namespace="$1" deployment="$2" max_surge="$3" max_unavailable="$4"
    runtime_kubectl -n "$namespace" patch deployment "$deployment" --type=merge \
        -p "{\"spec\":{\"strategy\":{\"type\":\"RollingUpdate\",\"rollingUpdate\":{\"maxSurge\":$max_surge,\"maxUnavailable\":$max_unavailable}}}}" \
        >/dev/null
}

runtime_set_image() {
    local namespace="$1" deployment="$2" container="$3" image="$4"
    runtime_kubectl -n "$namespace" set image "deployment/$deployment" \
        "$container=$image" >/dev/null || return 1
    RUNTIME_IMAGE_SET=1
    runtime_kubectl -n "$namespace" patch deployment "$deployment" --type=strategic \
        -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"$container\",\"imagePullPolicy\":\"IfNotPresent\"}]}}}}" \
        >/dev/null || return 1
}

runtime_set_images() {
    local namespace="$1" deployment="$2" api_container="$3" api_image="$4" worker_container="$5" worker_image="$6"
    runtime_kubectl -n "$namespace" set image "deployment/$deployment" \
        "$api_container=$api_image" "$worker_container=$worker_image" >/dev/null || return 1
    RUNTIME_IMAGE_SET=1
    runtime_kubectl -n "$namespace" patch deployment "$deployment" --type=strategic \
        -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"$api_container\",\"imagePullPolicy\":\"IfNotPresent\"},{\"name\":\"$worker_container\",\"imagePullPolicy\":\"IfNotPresent\"}]}}}}" \
        >/dev/null || return 1
}

runtime_annotate_source() {
    local namespace="$1" deployment="$2" sha="$3" schema="$4"
    runtime_kubectl -n "$namespace" patch deployment "$deployment" --type=merge \
        -p "{\"metadata\":{\"annotations\":{\"abastevo.com/source-revision\":\"$sha\",\"abastevo.com/api-source-revision\":\"$sha\",\"abastevo.com/worker-source-revision\":\"$sha\",\"abastevo.com/schema-floor\":\"$schema\"}}}" \
        >/dev/null
}

runtime_rollout_status() {
    local namespace="$1" deployment="$2" timeout="$3"
    runtime_kubectl -n "$namespace" rollout status "deployment/$deployment" \
        --timeout="${timeout}s" >/dev/null
}

runtime_rollout_undo() {
    local namespace="$1" deployment="$2"
    runtime_kubectl -n "$namespace" rollout undo "deployment/$deployment" >/dev/null
}
