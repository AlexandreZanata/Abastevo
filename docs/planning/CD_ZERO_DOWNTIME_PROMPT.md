# Execution prompt: zero-downtime continuous deploy (ZD-00..ZD-09)

Paste the block below into a new agent session opened at the repository root.

---

You are executing the infrastructure work in
`docs/planning/CD_ZERO_DOWNTIME_PLAN.md` for the Abastevo / brazil-fuel-prices
repository. Goal: every update of GitHub `main` deploys the new images to the
staging VPS (`https://teste.abastevo.com.br`, k3s namespace `abastevo-temp`)
automatically and with zero downtime for app users, using the existing signed
pull pipeline (`release-staging.yml` → GHCR signed manifest →
`abastevo-deploy-agent` systemd timer). Production is out of scope (G09).

## Read first (in this order)

1. `AGENTS.md`, `docs/planning/FAST_EXECUTION.md`, current
   `docs/planning/PROGRESS.md` (top entries only), `docs/planning/DELIVERY_WORKFLOW.md`.
2. `docs/planning/CD_ZERO_DOWNTIME_PLAN.md` (this task's plan — the source of truth).
3. `docs/planning/STAGING_CONTINUOUS_DEPLOY_PLAN.md`,
   `docs/planning/pending/CD_STAGING_ACCEPTANCE_REMAINING.md`,
   `docs/backend/evidence/cd/CD-03.md`, `CD-04.md`.
4. Code: `infra/deploy/agent/` (agent, `apply.sh`, `runtime/k8s.sh`, systemd,
   `tests/test-agent.sh`), `infra/k8s/staging/` (manifests, `render.py`,
   RBAC), `.github/workflows/release-staging.yml`,
   `backend/internal/platform/httpserver/httpserver.go`, `backend/cmd/api/main.go`
   (signal handling), `backend/cmd/worker/main.go` and its overlap test.
5. Reference model only (read, never modify, never run its deploy commands):
   `/home/iiii/PESSOAL-PROJETOS-ALEXANDRE/TBG-Tupi-Battle-Grounds`, on its
   `origin/main`: `deploy/auto/README.md`, `pull.py`, `control.py`, `slots.py`,
   `.github/workflows/backend-release.yml`. Read with
   `git -C <tbg> show origin/main:<path>`; that checkout is on another branch
   with other people's work.

## Working rules

- The main checkout may hold another effort's uncommitted changes. Do not
  touch, stash or reset them. Create a worktree from `dev` synchronized with
  `origin/main` (ADR-020), branch `codex/cd-zero-downtime`. The plan and this
  prompt may exist only as untracked files in the main checkout: copy them
  into the worktree and commit them first (`docs(cd): plan zero-downtime
  continuous deploy`).
- One task at a time, in plan order: ZD-00 → ZD-01 → ZD-02 → ZD-03 → ZD-04 →
  ZD-06 → (ZD-05 only if ZD-06 shows edge errors) → ZD-07 → ZD-08; ZD-09 only if
  the owner opts in.
- Per code task: B-BR/BUC note before behaviour, RED → GREEN → REFACTOR,
  targeted tests (Go `-race` for touched packages, agent harness, shellcheck,
  actionlint, `make check-infra` / `make test-infra` when manifests change),
  `git diff --check`, secret-surface review, one atomic English commit
  ending with the session's attribution line, PROGRESS entry (state, branch,
  tested revision, next action) and evidence in
  `docs/backend/evidence/cd/ZD-xx.md`.
- Never weaken a test or acceptance to pass. A known failure is fixed now,
  not deferred.
- Python scripts use the stdlib only; any new dependency needs the recorded
  need/license/security review first.

## Live VPS access (strict)

- Ask the owner for the SSH alias already in their private SSH config. Never
  write host names, IPs, ports, usernames, paths under `/opt`, kubeconfig or
  tokens into Git, commits, evidence or logs; redact them in receipts.
- ZD-00 is read-only: `systemctl status/list-timers`, `journalctl -n`,
  `cat` of state files, `kubectl get/describe` in `abastevo-temp`, `nginx -T`
  grep of the Abastevo vhost only.
- Every live change (applying manifests, moving private-storage, nginx
  reload, enabling surge, drills, timer changes) needs an explicit "yes" from
  the owner in chat **for that step**, stated with what will change and the
  rollback. Approval for one step does not cover the next.
- Forbidden: production; other namespaces or nginx vhosts; `kubectl delete`
  of the namespace, PVC, PV or StatefulSet; volume deletion; `nginx` restart
  (reload only after `nginx -t`); disabling the deploy timer without telling
  the owner; editing `current.json`/receipts by hand; force push, admin
  bypass, `--no-verify`.

## Key findings you must not lose

- Downtime cause: API Deployment `replicas: 1`, `maxSurge: 0`,
  `maxUnavailable: 1`; surge path exists in the agent but is off.
- `private-storage` runs inside the API pod on a hostPath media directory;
  a surge would start a second storage process on the same data. Treat it as
  single-writer until ZD-00 proves otherwise; ZD-03 moves it to its own
  `Recreate` Deployment before surge is enabled.
- The API closes its listener at SIGTERM without first failing readiness; ZD-02
  adds the readiness-503 drain delay.
- The agent lacks TBG's deployment floor (candidate must descend from the
  deployed SHA), rejection latch and operator hold (ZD-01).
- `imagePullPolicy` drift: Git `Never`, live `IfNotPresent`.

## Definition of done

Done when all of the plan's section 4 holds with receipts: unattended `main`
deploys; three measured rollouts with zero failed requests and zero
unavailable replicas (`infra/scripts/rollout-probe.py`); a broken image never
removes the serving pod; no downgrade/loop; hold and pause runbooks verified;
docs/PROGRESS/README match live state. Finish with one dev → main PR through
the protected flow (`git-flow.sh finish` with the configured required
check), without polling CI. Report in Portuguese to the owner: what is live,
what is measured, what remains OWED.

## Stop and ask the owner when

- the deploy timer is in a failure loop or an operator hold exists;
- ZD-00 shows the storage server cannot be moved without changing existing
  signed/private media URLs;
- the quota cannot fit two API pods (raising it is the owner's call);
- a drill needs a deliberately broken commit on signed `main` lineage;
- any live step would affect another app on the shared VPS.
