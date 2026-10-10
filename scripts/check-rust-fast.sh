#!/usr/bin/env bash
# Bounded Rust selection for tools/* changes (RST-02+ station-prep).
# Runs the frozen Rust gate: fmt check, clippy with warnings denied and
# locked-dependency tests. Fails explicitly on missing toolchain/crate;
# never silently skips the area that triggered it.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CRATE="$ROOT/tools/station-prep"
if [[ ! -d "$CRATE" ]]; then
    echo "ERROR: Rust crate missing: tools/station-prep" >&2
    exit 1
fi
if ! command -v cargo >/dev/null 2>&1; then
    echo "ERROR: missing tool cargo (rustup toolchain install 1.99.0 --profile minimal --component rustfmt,clippy)" >&2
    exit 1
fi
cd "$CRATE"

echo "== rust fmt =="
cargo fmt --check
echo "== rust clippy =="
cargo clippy --all-targets -- -D warnings
echo "== rust tests (locked) =="
cargo test --locked
echo "check-rust-fast: ok"
