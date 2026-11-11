# Go 1.27.4 security toolchain update

Status: LOCAL_DONE
Date: 2026-11-11. Delivery: existing protected dev -> main PR #201; no
deployment or tag.

## Before behavior

Quick verification failed on dev `231e63b7` in the bounded integration gate:
`govulncheck ./...` reported GO-2026-6859 in `net/http`, found in
`net/http@go1.27.3` and fixed in `net/http@go1.27.4`, reachable from the API
server, admin, evidence upload, forum and source fetch paths. The finding is
in the standard library, not in project code or modules.
Source: https://pkg.go.dev/vuln/GO-2026-6859.

## Change

Same scope as the Go 1.27.3 update
([GO1273_SECURITY_REVALIDATION](GO1273_SECURITY_REVALIDATION.md)): Go 1.27
language level and module set unchanged; only the exact toolchain moves.

- `backend/go.mod`: `toolchain go1.27.4`.
- CI `GO_VERSION` in `quick.yml` and `backend.yml`: `1.27.4`.
- Docker builder `golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea`
  (manifest-list digest read from the Docker Hub registry on 2026-11-11).
- Smoke, load, restore and rehearsal scripts build with `GOTOOLCHAIN=go1.27.4`;
  `check-security.sh` now refuses any other go.mod toolchain pin.
- Staticcheck and govulncheck keep their separate analysis-only installs.

## Verification

- Toolchain fetched by the Go command (`go: downloading go1.27.4`), which
  verifies it against the Go checksum database.
- `govulncheck ./...` on go1.27.4: no vulnerabilities found.
- `make quick-verify` locally on the same tree before pushing.

Not verified here: the image build and its smoke run, which follow the
existing release gates.
