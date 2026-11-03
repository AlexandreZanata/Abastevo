# Zero-downtime continuous deploy plan (staging VPS)

Status: SOURCE_LOCAL_DONE / LIVE_ACCEPTANCE_OWED (2026-11-03). Owner request: every `main` update reaches the
VPS automatically, with **zero downtime** for people using the app while the
new image rolls out, following the delivery model already running in the
TBG (Tupi Battle Grounds) project. Written for execution by another agent:
each task lists inputs, outputs, acceptance and STOP conditions.

Scope: staging origin `https://teste.abastevo.com.br`, k3s namespace
`abastevo-temp` on the shared VPS (ADR-019). Production stays a manual,
approved release behind G09 ([STAGING_CONTINUOUS_DEPLOY_PLAN](STAGING_CONTINUOUS_DEPLOY_PLAN.md)
Phase 5); nothing here deploys production. Never infer live readiness from
this plan: every live claim needs a receipt.

## 1. Reference model: how TBG deploys from main

TBG (`deploy/auto/` on its `main`) is a **signed pull** model:

| TBG mechanism | What it guarantees | Abastevo equivalent today |
| --- | --- | --- |
| `backend-release.yml` runs on `workflow_run` of a successful **push** CI on `main`, matching head SHA only | PRs/forks/stale commits never publish | `release-staging.yml` (same gate over `quick`, `CI`, `backend`) — DONE |
| Immutable release `server-<sha>` + GitHub OIDC build provenance | Hashes alone never authenticate | Signed OCI release manifest + cosign keyless identity on GHCR — DONE |
| VPS systemd timer polls GitHub over HTTPS, no token, no inbound SSH | GitHub holds no host secret | `abastevo-deploy-agent.timer` (2 min) — DONE, enabled 2026-10-28 per pending doc (verify live) |
| Candidate must **descend from the last verified deployed SHA** (`minimum_source` / `current.json`) | No silent downgrade | Agent only checks "ancestor of `main`" — **GAP (ZD-01)** |
| `checked.json` latches a rejected candidate; `operator-hold.json` stops every later run after a failed rollback | No retry/restart loop | Failure receipt only; a failed SHA may be re-applied every run — **GAP, verify (ZD-01)** |
| Two release slots: the new generation starts in the idle slot, is checked, then promoted; the old slot **drains** and stops only after a retire check | Requests in flight and users never see the switch | API `replicas: 1`, `maxSurge: 0`, `maxUnavailable: 1` → the only pod stops before the new one is ready — **GAP: downtime on every deploy (ZD-02..ZD-04)** |
| Public verification through the real proxy (health, admission, WebSocket) before old slot retires | Promotion proven end to end | Smoke after rollout — DONE; edge/drain behaviour unmeasured — **GAP (ZD-06)** |
| Shared non-blocking lock; a busy lock prints `Deferred:` and exits 0 | Families never race | `flock -n` + `apply.lock` — DONE |
| Every step journaled; a crashed run resumes from the journal | Interrupted deploys converge | `rollout undo` + receipts; resume semantics unverified — **verify (ZD-01)** |
| `status.py` one-screen status, staggered timers, explicit `User-Agent` for Cloudflare | Operable by the owner | Not started — **GAP (ZD-08)** |

TBG needs custom slots because its game keeps in-memory matches in one Rust
process. Abastevo is stateless HTTP behind Kubernetes, so the **same
guarantee** is reached with a Deployment surge rollout (new pod ready before
the old one receives SIGTERM) plus a correct drain — no custom slot
controller. What must be copied from TBG is the trust chain, the downgrade
floor, the rejection latch/operator hold, the drain discipline and the
measured proof.

## 2. Why a deploy causes downtime today

1. `infra/k8s/staging/api-deployment.yaml`: `replicas: 1`, `maxSurge: 0`,
   `maxUnavailable: 1`. Kubernetes kills the only API pod first; until the
   new pod passes readiness (≥15 s initial delay) every request fails.
2. The surge path exists in the agent (`ABASTEVO_ALLOW_SURGE`, quota
   headroom check) but ships **off** until measured (CD-04 Part C).
3. The API pod also runs `worker` and `private-storage`. Two pods side by
   side during a surge means two `private-storage` processes on the same
   hostPath media directory. Worker overlap is proven safe (`FOR UPDATE SKIP
   LOCKED` + lease fence, R5 overlap test); **private-storage overlap is
   not** and must be treated as single-writer until proven otherwise.
4. `httpserver.Run` calls `http.Shutdown` immediately on SIGTERM. Kubernetes
   removes the endpoint asynchronously, so for a short window kube-proxy and
   the shared nginx edge still send new connections to a listener that is
   already closed. Readiness never flips to 503 before the drain.
5. `imagePullPolicy` drift (`Never` in Git, `IfNotPresent` live) makes the
   R2 zero-diff proof fail and hides whether the surge pod must pull first.

## 3. Tasks (execute in order; one bounded task at a time)

Task IDs continue the CD series as **ZD-xx** (zero downtime). Every code
task: B-BR/BUC note before behaviour, RED → GREEN → REFACTOR, targeted tests,
`git diff --check`, secret-surface review, PROGRESS entry with tested
revision. Every live step: explicit owner authorization in chat for that
step, receipt in `docs/backend/evidence/cd/ZD-xx.md`, no host/IP/path/token
in Git.

### ZD-00 — Read-only live inventory (no change)

Record, without modifying anything: timer/service state and last 20 journal
lines; `current.json` and the last receipts; k3s/kubectl server version;
ResourceQuota used/hard; live api Deployment strategy, probes, grace period,
`imagePullPolicy`; which server the `private-storage` image runs and whether
it supports concurrent processes on one data directory; the shared nginx
vhost upstream for Abastevo (read-only: upstream target, `proxy_next_upstream`,
timeouts); Cloudflare in front or not.
Acceptance: `ZD-00.md` evidence with values (secrets/IPs redacted).
STOP if the timer is in a failure loop: disable nothing, report to the owner.

### ZD-01 — Agent safety guards (code, harness tests)

Port TBG's guards into `infra/deploy/agent`:

- **Deployment floor**: apply only if the candidate SHA is `ahead` or
  `identical` to `current.json.sha` (anonymous compare API), in addition to
  the existing "ancestor of main" check. Behind/diverged → refuse, receipt.
- **Rejection latch**: a SHA whose apply failed (rollback done) is recorded
  in `checked.json` as `rejected`; later runs skip it with exit 0 until a
  newer SHA appears or an operator passes `--recheck <sha>`.
- **Operator hold**: if `rollout undo` or landing restore fails, write
  `operator-hold.json`; every later run refuses (exit non-zero, no action)
  until `abastevo-deploy-agent release-hold`.
- **Resume**: verify a run killed between migration and rollout, and between
  rollout and `current.json`, converges on the next run (harness cases).
Acceptance: new harness cases for each refusal/latch/hold/resume path;
shellcheck + actionlint + existing harness PASS.

### ZD-02 — Graceful drain in the processes (backend code)

- API: on SIGTERM, `/health/ready` returns 503 at once, the server keeps
  serving for `ANPFUEL_SHUTDOWN_DRAIN_DELAY` (default 10 s, bounded 0–30 s),
  then `http.Shutdown` with the existing timeout. Long uploads in flight
  finish within the shutdown timeout.
- Worker: on SIGTERM stop claiming new jobs and release/finish leases within
  the grace period (verify current behaviour first; change only if needed).
- Invariant to document: `drain delay + shutdown timeout + 5 s <
  terminationGracePeriodSeconds`.
Acceptance: integration test that sends SIGTERM with requests in flight and
new requests arriving during the delay: zero refused connections, readiness
503 during the delay, clean exit 0; `go test -race` for the touched packages.

### ZD-03 — Move `private-storage` out of the API pod

Only if ZD-00 shows the storage server is not safe with two concurrent
processes on one directory (default assumption). New singleton Deployment
`abastevo-temp-storage` (`replicas: 1`, `strategy: Recreate`) + ClusterIP
Service; api/worker reach it by Service DNS. The deploy agent never patches
it (like TBG never touches Caddy). Update `infra/k8s/staging/*.yaml`,
`render.py` placeholders and RBAC only if needed.
Live step (owner-scheduled low-traffic window): apply, verify upload +
signed private read, then R2 zero-diff proof.
Acceptance: storage pod independent of API rollouts; photo upload and
private read smoke pass; zero-diff clean.
STOP if the storage endpoint is embedded in signed URLs/canonical hosts in a
way that would change existing evidence links: report, do not migrate.

### ZD-04 — Rollout manifest hardening

API Deployment: `strategy.rollingUpdate.maxSurge: 1`, `maxUnavailable: 0`;
`minReadySeconds: 10`; `startupProbe` on `/health/live` (replaces the long
initial delay); readiness `periodSeconds: 3`, `timeoutSeconds: 2`,
`failureThreshold: 2`; `terminationGracePeriodSeconds` per the ZD-02
invariant; `imagePullPolicy: IfNotPresent` (fix drift); `revisionHistoryLimit`
kept ≥ 3 for `rollout undo`. Confirm the quota fits two API pods (requests
and limits); raising the quota is an owner decision. The Caddy web
Deployment already rounds to surge 1 / unavailable 0 — verify, do not change.
Agent: keep `ABASTEVO_ALLOW_SURGE` gating; flip the default to `1` only after
ZD-06 passes and the owner approves.
Acceptance: rendered manifests pass the infra gate; live apply via the
reviewed path, zero-diff clean.

### ZD-05 — Edge retry (only if ZD-06 shows edge errors)

If the probe shows 502/connection errors at pod switch, add to the
Abastevo vhost only: `proxy_next_upstream error timeout http_502 http_503;
proxy_next_upstream_tries 2;` for idempotent methods (nginx default), and
keepalive settings that do not pin a dying pod. Shared nginx: owner
authorization, `nginx -t`, reload (never restart), other vhosts untouched.

### ZD-06 — Zero-downtime proof (measured, like TBG's rehearsal)

Add `infra/scripts/rollout-probe.py` (Python stdlib only, no new dependency):
constant load (default 20 req/s) against public staging endpoints
(`/health/ready`, station list, one nearby read, landing `/`) plus one slow
upload-sized request, recording every non-2xx, connection error and p99
latency, with an explicit `User-Agent: abastevo-rollout-probe/1.0`. In
parallel record `kubectl get deploy -w` (unavailable replicas).
Run during 3 agent rollouts (a no-op image bump reviewed by the owner, or 3
real merges).
Acceptance: 0 failed requests and `unavailableReplicas` always 0 in all 3
runs; results in `ZD-06.md`. Then the owner decides the surge default.
STOP on any failure: record it, fix in ZD-02/04/05, re-run; never weaken the
acceptance.

### ZD-07 — Remaining CD-04 acceptance drills

From [CD_STAGING_ACCEPTANCE_REMAINING](pending/CD_STAGING_ACCEPTANCE_REMAINING.md):
two consecutive unattended `main` deploys (distinct receipts), broken-image
drill (bad pod never becomes ready, old pod keeps serving, `rollout undo`,
failure receipt, `current.json` unchanged, latch from ZD-01 set),
contract-floor refusal, landing rollback drill, data rehearsal on a restored
dump. The drill mechanism on signed `main` lineage is an owner decision.

### ZD-08 — Operations

- `infra/deploy/agent/status.sh`: one screen with timer state, `current.json`,
  last receipt, `checked.json`, hold, pods/ReplicaSets, quota.
- Owner failure notification with a VPS-local token (channel is the owner's
  choice; token never in Git).
- Disk retention: k3s image GC thresholds verified, landing keep 5, backups
  keep 7, old ReplicaSets bounded.
- Timer: `RandomizedDelaySec` and `AccuracySec=1s` like TBG; pause runbook
  (`systemctl disable --now abastevo-deploy-agent.timer`) verified.
- Implement or formally amend plan Phase 3 step 5 (worker search-script env
  + one portal refresh), which `apply.sh` does not do.

### ZD-09 — Old-app compatibility (owner decision, optional)

App users run older APKs during and after every deploy. Keep the existing
expand/contract schema window; add a CI check that refuses breaking changes
to the public API contract under `contracts/` (removed path, field, enum
value, required request field) against the last released contract. A new
tool dependency needs the recorded license/security review first.

## 4. Definition of done

- A merge to `main` reaches staging without a human; receipts prove it.
- Three measured rollouts with zero failed requests and zero unavailable
  replicas; a broken image never removes the serving pod.
- No downgrade, no retry loop, operator hold works, pause runbook works.
- Docs, PROGRESS, evidence and the agent README match the live state.

## 5. Never

Secrets, host names/IPs, `/opt` or kubeconfig paths in Git or Actions logs;
push-based SSH deploy from GitHub; `latest` tags; `kubectl delete` of the
namespace, PVC or StatefulSet; `docker compose down -v` / volume deletion;
touching other namespaces or nginx vhosts; production deploys; enabling
anything live without the owner's explicit authorization for that step.

## 6. Execution checkpoint — 2026-11-03

- ZD-00 read-only inventory complete; staging c19f0bde, timer/apply active,
  surge OFF. No live write was performed.
- ZD-01/02/03/04 source and ZD-06/08 tooling locally validated; cumulative
  delivery is one protected dev → main PR. See `docs/backend/evidence/cd/ZD-xx.md`.
- ZD-03/04 live migration/apply/zero-diff, three ZD-06 campaigns, ZD-07 drills,
  notification activation, image GC, pause and actual portal refresh remain
  OWED. Source preparation does not consume those live acceptance gates.
- ZD-05 has no measured trigger; no edge retry change. ZD-09 has no owner opt-in.
- Phase 3 step 5's stable script path/15-minute worker ownership is formally
  amended in STAGING_CONTINUOUS_DEPLOY_PLAN; its live path/refresh is unverified.
- Next owner-controlled live step: approve a reviewed complete agent bundle
  upgrade with prior bundle retained for rollback. Topology/surge/timer/drills
  require their own subsequent approvals; no main merge is performed merely
  to satisfy a request to open the source PR.
