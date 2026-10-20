# FQ-05W8 — Static worker image, explicit source schedules and FQ-06/07 reconciliation

Status: LOCAL_DONE
Validation: PASS

Completed 2026-10-18 on maintained dev, parent a7b13b34 (FQ-05W7). The tested
behavior is this atomic checkpoint's image, schedule, config and
reconciliation changes; delivery remains INTEGRATION_PENDING. No deployment,
tag, import activation, schedule enablement or live source call claimed.

Before behavior: B-BR-FQ-01/02/04/05/13/16, BUC-FQ-01/02/03/04. The worker
image shipped only Go binaries, so the trusted `/prepare_quality` path could
not exist in a real container; no source schedule existed, and FQ-06/07
runtime claims were only adapter-fixture evidence. The image must stay
distroless/static nonroot with no shell, no hostPath executable and no
command override; scheduling must be explicit, default-off and must not
activate unrelated ANP/registry imports.

## Delivered and verified

- Dockerfile `rust-builder` stage on the pinned `rust:1.99.0-bookworm`
  digest builds the locked station-prep crate for
  `x86_64-unknown-linux-musl` (static, no glibc) and explicitly chmods 755.
  The worker target copies only `/prepare_quality` from that stage
  (`--chown=65532:65532`), so no Rust toolchain or source ships. The
  `.dockerignore` still default-denies everything and now re-includes only
  `tools/station-prep/{Cargo.toml,Cargo.lock,src}` beside the existing Go
  inputs; raw archives, fixtures, env files and `target/` stay out.
- Real container acceptance on the built image (legacy builder, no
  BuildKit-only syntax): `docker run --user 65532:65532 --entrypoint
  /prepare_quality` produced one accepted assay from a synthetic 19-column
  CSV (`input=1 accepted=1 duplicates=0 quarantined=0`, contract
  `pmqc-native-19-v2026-10b`) with owned artifacts; invoking it without
  arguments prints usage and exits cleanly; a no-arg run without a mount
  proves the executable and loader work on distroless/static. The migrate
  target from the same Dockerfile applied the full chain through
  `000104` to a fresh disposable database, and the worker image booted
  against it with `ANPFUEL_QUALITY_IMPORTS_ENABLED=true` (schedules off),
  composed all dispatcher slots and exited 0 on SIGTERM. Image sizes:
  worker 6,093,784 bytes, migrate 4,436,220 bytes.
- Two explicit default-off switches install the bounded source schedules:
  `ANPFUEL_QUALITY_DISCOVERY_ENABLED` (`quality-discovery-daily`) and
  `ANPFUEL_QUALITY_REVALIDATION_ENABLED` (`quality-revalidate-daily`).
  Invalid booleans refuse config loading; each schedule is independent and
  never changes `ANPFUEL_QUALITY_IMPORTS_ENABLED` or the unrelated
  ANP/registry schedules. Enabling a schedule without the handler switch
  only queues jobs, which stay at zero attempts.
- FQ-06/07 reconciliation: recorded the actual runtime state instead of
  inheriting the adapter fixtures. There is no `prepare_enforcement` or
  `prepare_sanction` binary, no composed Go import/discovery stream for
  those sources and no verified live resource inventory; the Go typed
  publication path remains separately proven with real PostGIS (FQ-04b).
  FQ-06/FQ-07 evidence, the implementation plan and ROADMAP now say so, and
  the ROADMAP no longer lists FQ-06/07 as plain LOCAL_DONE.

License/security reasoning: the Rust dependency set is the already-reviewed
`Cargo.lock` (MIT crate licenses, unchanged); `musl-tools` is installed only
in the builder stage as a build tool and never enters the runtime image; the
pinned base and runtime digests are unchanged from the existing Dockerfile
policy; no new runtime library or service was added.

Commands (repository root; docker 29.1.3):

```sh
docker build -f infra/docker/Dockerfile --target worker --build-arg REVISION=w8test -t anpfuel-worker:w8test .
docker build -f infra/docker/Dockerfile --target migrate --build-arg REVISION=w8test -t anpfuel-migrate:w8test .
docker run --rm --user 65532:65532 -v /tmp/fq-container-test:/data --entrypoint /prepare_quality anpfuel-worker:w8test /data/input.csv /data/out
docker run --rm --network host -e DATABASE_URL=... anpfuel-migrate:w8test
docker run -d --name w8worker --network host -e DATABASE_URL=... -e ANPFUEL_QUALITY_IMPORTS_ENABLED=true anpfuel-worker:w8test
docker stop w8worker
```

PASS: both images built; container preparer run accepted the synthetic row
and wrote valid artifacts as uid 65532; the migrate image applied through
000104; the worker booted with the native handlers composed and stopped
cleanly (exit 0). From backend/: config unit tests including the new
independent-switch and invalid-boolean cases, `go build ./...`, package vet,
gofmt, `git diff --check` and `scripts/scan-secrets.sh` PASS. No new
dependency; no hostPath executable or command override was introduced.

## Explicit next obligations

Schedules stay disabled until an operator enables imports and the image is
published; no deployment/tag occurred here. FQ-06/07 live Go streams,
prepare CLIs, verified live inventories and complete enforcement/sanction
history remain OWED. FQ-12–17 and the final protected dev→main batch remain;
D is NOT_AUTHORIZED.
