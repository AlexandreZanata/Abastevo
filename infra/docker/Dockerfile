# anpfuel backend images.
#
# One pinned builder compiles the process roles and catalog operator
# from the same base layers; each target ships its static binaries on a
# non-root distroless runtime with no shell. Same revision label across roles
# is the release provenance. Secrets travel at runtime (env), never in layers.
#
# Build from the repository root, e.g.:
#   docker build -f infra/docker/Dockerfile --target api \
#     --build-arg REVISION=$(git rev-parse --short HEAD) .
ARG GOLANG_IMAGE=golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5
ARG RUST_IMAGE=rust:1.99.0-bookworm@sha256:114c7a4425406451c2866b6aafe69fe29b1b298832db1277d411ac73c82d04d6
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
ARG REVISION=dev

FROM ${RUST_IMAGE} AS rust-builder
# Static musl build so the distroless/static runtime needs no glibc. The
# musl toolchain is build-only; only the pinned native preparer binaries are
# copied into the worker image. The locked crate dependency set is reused
# unchanged (dependency licenses already reviewed for station-prep).
RUN apt-get update \
 && apt-get install -y --no-install-recommends musl-tools \
 && rm -rf /var/lib/apt/lists/* \
 && rustup target add x86_64-unknown-linux-musl
ENV CARGO_TARGET_DIR=/src/target
WORKDIR /src/tools/station-prep
COPY tools/station-prep/Cargo.toml tools/station-prep/Cargo.lock ./
COPY LICENSE /src/LICENSE
COPY tools/station-prep/src ./src
COPY contracts/ev /src/contracts/ev
COPY contracts/station-prep /src/contracts/station-prep
RUN cargo build --locked --release --target x86_64-unknown-linux-musl --bin prepare_quality --bin prepare_sanctions --bin prepare_fine_history --bin prepare_enforcement --bin prepare_charging --bin prepare_vehicle \
 && chmod 755 /src/target/x86_64-unknown-linux-musl/release/prepare_quality /src/target/x86_64-unknown-linux-musl/release/prepare_sanctions /src/target/x86_64-unknown-linux-musl/release/prepare_fine_history /src/target/x86_64-unknown-linux-musl/release/prepare_enforcement /src/target/x86_64-unknown-linux-musl/release/prepare_charging /src/target/x86_64-unknown-linux-musl/release/prepare_vehicle

FROM ${GOLANG_IMAGE} AS builder
ENV CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=local
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/cmd ./cmd
COPY backend/internal ./internal
COPY backend/db ./db
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
 && go build -trimpath -ldflags="-s -w" -o /out/station-load ./cmd/station-load \
 && go build -trimpath -ldflags="-s -w" -o /out/charging-catalog ./cmd/charging-catalog

FROM ${RUNTIME_IMAGE} AS api
ARG REVISION
LABEL org.opencontainers.image.title="anpfuel-api" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.licenses="LicenseRef-Abastevo-Visible-Attribution-1.0"
COPY --from=builder --chown=65532:65532 /out/api /api
COPY --chown=65532:65532 LICENSE THIRD_PARTY_NOTICES.md /usr/share/doc/abastevo/
COPY --chown=65532:65532 LICENSES/MIT-original.txt /usr/share/doc/abastevo/MIT-original.txt
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/api"]

FROM ${RUNTIME_IMAGE} AS worker
ARG REVISION
LABEL org.opencontainers.image.title="anpfuel-worker" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.licenses="LicenseRef-Abastevo-Visible-Attribution-1.0"
COPY --from=builder --chown=65532:65532 /out/worker /worker
COPY --from=builder --chown=65532:65532 /out/charging-catalog /charging-catalog
# Trusted native quality preparer: immutable image path, mode 755, static
# musl build, never a hostPath or job-provided executable.
COPY --from=rust-builder --chown=65532:65532 /src/target/x86_64-unknown-linux-musl/release/prepare_quality /prepare_quality
COPY --from=rust-builder --chown=65532:65532 /src/target/x86_64-unknown-linux-musl/release/prepare_sanctions /prepare_sanctions
COPY --from=rust-builder --chown=65532:65532 /src/target/x86_64-unknown-linux-musl/release/prepare_fine_history /prepare_fine_history
COPY --from=rust-builder --chown=65532:65532 /src/target/x86_64-unknown-linux-musl/release/prepare_enforcement /prepare_enforcement
COPY --from=rust-builder --chown=65532:65532 /src/target/x86_64-unknown-linux-musl/release/prepare_charging /prepare_charging
COPY --from=rust-builder --chown=65532:65532 /src/target/x86_64-unknown-linux-musl/release/prepare_vehicle /prepare_vehicle
COPY --chown=65532:65532 LICENSE THIRD_PARTY_NOTICES.md /usr/share/doc/abastevo/
COPY --chown=65532:65532 LICENSES/MIT-original.txt /usr/share/doc/abastevo/MIT-original.txt
USER 65532:65532
ENTRYPOINT ["/worker"]

FROM ${RUNTIME_IMAGE} AS migrate
ARG REVISION
LABEL org.opencontainers.image.title="anpfuel-migrate" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.licenses="LicenseRef-Abastevo-Visible-Attribution-1.0"
COPY --from=builder --chown=65532:65532 /out/migrate /migrate
COPY --chown=65532:65532 LICENSE THIRD_PARTY_NOTICES.md /usr/share/doc/abastevo/
COPY --chown=65532:65532 LICENSES/MIT-original.txt /usr/share/doc/abastevo/MIT-original.txt
USER 65532:65532
ENTRYPOINT ["/migrate"]

FROM ${RUNTIME_IMAGE} AS station-load
ARG REVISION
LABEL org.opencontainers.image.title="anpfuel-station-load" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.licenses="LicenseRef-Abastevo-Visible-Attribution-1.0"
COPY --from=builder --chown=65532:65532 /out/station-load /station-load
COPY --chown=65532:65532 LICENSE THIRD_PARTY_NOTICES.md /usr/share/doc/abastevo/
COPY --chown=65532:65532 LICENSES/MIT-original.txt /usr/share/doc/abastevo/MIT-original.txt
USER 65532:65532
ENTRYPOINT ["/station-load"]
