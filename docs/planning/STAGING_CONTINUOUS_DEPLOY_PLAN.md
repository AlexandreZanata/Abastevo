# Staging continuous deployment plan (public repository)

Status: PLANNED (2026-10-26). Goal: every commit on `main` whose required
checks pass reaches `https://teste.abastevo.com.br` automatically, with no
secret, host detail or private configuration in this public repository or
its public Actions logs. Written for execution by a less capable agent:
each phase lists inputs, exact outputs, checks and STOP conditions.

Scope limits (from [CI_PLAN](CI_PLAN.md) and [DELIVERY_WORKFLOW](DELIVERY_WORKFLOW.md)):
automatic deployment targets **staging only**. Production stays a manual,
approved release behind G09; nothing here deploys production or tags a
release. One app (Abastevo, namespace `abastevo-temp`) on a shared VPS: the
agent never touches another namespace, nginx site or host service.

## 1. Design decision: signed pull, not push

| | Push (Actions SSH into the VPS) | **Pull (chosen)** |
| --- | --- | --- |
| Secrets in GitHub | SSH key/host in environment secrets | **none** beyond the automatic `GITHUB_TOKEN` and OIDC |
| Inbound access to VPS | GitHub runners need SSH | **none**; the VPS only makes outbound HTTPS |
| Public log exposure | host/IP/commands risk | Actions logs only show build/sign steps |
| Fork/PR abuse | must guard `pull_request_target` | builds run only on `push` to `main` |
| Trust anchor | whoever holds the key | Sigstore keyless signature bound to this repo, this workflow and `refs/heads/main` |

Flow: `main` push → required checks green → **release workflow** builds
immutable images (api, worker, migrate) and the landing bundle, publishes
them by digest to GHCR, attaches SBOM + build provenance, writes a signed
**release manifest** → a **deploy agent on the VPS** (systemd timer) polls,
verifies signature and identity, then runs the staged rollout with
automatic rollback.

Images contain only public source and no secrets, so GHCR packages can be
public: the VPS pulls without any token. Runtime secrets stay in the
existing k8s Secret on the VPS.

## 2. Lessons the design must encode (staging incident 2026-10-26)

- Migrate **before** rolling app images; refuse a rollout when the target
  image needs a schema newer than the database.
- Keep one surge pod (or a readiness gate) so a bad image never takes the
  only pod down; roll back automatically on failed readiness.
- An old image must tolerate an additive newer schema for the rollback
  window (expand/contract), or rollback is impossible after migrating.
- Back up (verified `pg_dump -Fc` + TOC check) before every migration.

## 3. Phases

### Phase 1 — Signed release artifacts from `main` (CI only, no VPS)

Deliverables (one PR):
- `.github/workflows/release-staging.yml`:
  - Trigger `workflow_run` on the required workflows (`quick`, `CI`,
    `backend`) for `main`, proceeding only when **all** concluded `success`
    for the same `head_sha` (a small gate job queries the check runs via
    `GITHUB_TOKEN`); also `workflow_dispatch` with an explicit `sha` input
    restricted to commits reachable from `main`.
  - `permissions:` minimal per job: `contents: read`; build job adds
    `packages: write`, `id-token: write`, `attestations: write`. No
    `pull_request_target`, no secrets, no `environment` secrets.
  - Every third-party action pinned by full commit SHA with a version
    comment; `concurrency: release-staging` (cancel-in-progress false).
  - Build `infra/docker/Dockerfile` targets `api`, `worker`, `migrate` with
    `REVISION=<sha>`; push to `ghcr.io/<owner>/abastevo-<role>:sha-<sha>`;
    record digests. Build `landing` with
    `--origin https://teste.abastevo.com.br` and upload `landing-dist.tar.gz`.
  - SBOM (syft or `docker buildx --sbom`) and
    `actions/attest-build-provenance` per image digest.
  - `release-manifest.json` (schema `abastevo-release-manifest-v1`): `sha`,
    `created_at`, per-role `image@digest`, `landing_sha256`,
    `migrations_max` (highest file in `backend/db/migrations`),
    `schema_min_supported` (see Phase 3), `workflow_run_url`. Signed with
    `cosign sign-blob` keyless; publish manifest + `.sig`/bundle as a
    workflow artifact **and** as an OCI artifact
    `ghcr.io/<owner>/abastevo-release:sha-<sha>` plus moving tag `staging`.
  - A final step prints the exact verification command
    (`cosign verify-blob --certificate-identity-regexp
    'https://github.com/<owner>/Abastevo/.github/workflows/release-staging.yml@refs/heads/main'
    --certificate-oidc-issuer https://token.actions.githubusercontent.com`).
- `scripts/check-release-workflow.sh` (run in Quick): fails if the release
  workflow uses unpinned actions, `pull_request_target`, `secrets.` other
  than `GITHUB_TOKEN`, `set -x`, `env` dumps, or echoes of variables.
- `.github/CODEOWNERS` entry requiring owner review for `.github/workflows/`
  and `infra/deploy/`.
- Docs: this plan's Phase 1 evidence in `docs/backend/evidence/cd/CD-01.md`.

Checks: `actionlint` (pinned), the new script, a dry run on a branch with
`workflow_dispatch` disabled for non-main refs, then one real run on `main`
producing verifiable artifacts. STOP if GHCR publishing needs a personal
token (it must not), if any step would need a secret, or if the required
check names differ from the ones above (record and ask).

Done when: for a `main` SHA, `cosign verify-blob` and
`gh attestation verify` pass for every image and the manifest; images are
pullable anonymously; Actions logs contain no host, IP, path under `/opt`,
or variable dump.

### Phase 2 — VPS deploy agent, dry run

- `infra/deploy/agent/abastevo-deploy-agent` (bash or Go, no secrets in
  source) + `infra/deploy/agent/README.md`; host config only in
  `/etc/abastevo-deploy/agent.env` (mode 600, never committed): namespace,
  deployment names, landing root, backup dir, receipt dir.
- systemd `abastevo-deploy-agent.timer` (every 2 min) + service with
  `flock`, `ProtectSystem=strict`, `NoNewPrivileges=yes`.
- Each run: fetch `staging` manifest, verify signature + certificate
  identity + `sha` reachable from `main` (GitHub API, anonymous), compare
  with `/var/lib/abastevo-deploy/current.json`, and write a **plan receipt**
  (what would change) without mutating anything.

Done when: the agent detects a new `main` release, verifies it, refuses a
tampered/unsigned/foreign-identity manifest (tested), and changes nothing.

### Phase 3 — Automated rollout with rollback (staging)

Order per release (any failure stops and rolls back what changed):
1. Schema gate: refuse if `schema_min_supported` of the running image is
   above the DB, or if the new image needs migrations not yet applied and
   the previous image cannot tolerate them (expand/contract rule).
2. Verified backup (`pg_dump -Fc`, TOC check, retention N=7).
3. Migration Job with the `migrate` digest; wait `complete`; record versions.
4. Roll api/worker by digest with `maxSurge: 1` (raise the namespace quota
   only if the owner approves; otherwise `maxUnavailable: 0` with a
   pre-pull) and readiness gate; on failure `rollout undo`.
5. Landing: unpack to `releases/<sha>`, switch `current` atomically, update
   the worker search-script env, enqueue one portal refresh.
6. Smoke: `/health/ready`, `/`, `/consultar/`, one nearby/list call, one
   search shard; on failure roll back images and landing pointer.
7. Write the receipt (no secrets) and keep the previous release for rollback.

Code change required first: the API/worker schema check accepts a newer
**additive** schema within a declared window (`schema_min_supported` ..
current), so rollback after a migration works.

Done when: two consecutive `main` merges deploy without a human, a
deliberately broken image is rolled back automatically with the site
staying up, and a rollback drill is recorded.

### Phase 4 — Operations

Status page/receipt endpoint (static JSON with release SHA and time),
failure notification to the owner (email/Telegram via a VPS-local token),
disk retention for images/releases/backups, and a runbook for pausing the
agent (`systemctl stop abastevo-deploy-agent.timer`).

### Phase 5 — Production (separate decision, not automatic)

Same artifacts, a manual `workflow_dispatch` approval with GitHub
environment `production` reviewers, and the G09 gates. Out of scope until
the owner opens it.

## 4. Never

- Secrets, tokens, SSH keys, host names/IPs, `/opt` paths or kubeconfig in
  the repository, workflow files, Actions logs or release manifests.
- `pull_request_target`, workflows triggered by forks with write tokens,
  unpinned actions, `curl | bash` installers, `latest` tags.
- Deploying a SHA whose required checks did not all pass, or any commit not
  on `main`.
- Touching other namespaces, the shared nginx config beyond the Abastevo
  vhost, or host packages from the agent.
