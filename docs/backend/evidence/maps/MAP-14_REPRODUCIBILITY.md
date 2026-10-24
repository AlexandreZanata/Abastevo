# MAP-14 T01/T03 — Clean-build reproducibility

Status: **CLEAN_BUILD_REPRODUCIBLE / FRESH_CHECKOUT_CLONE_PENDING**
Date: 2026-10-24. Upstream: `4587b8ad4d12c9f59982709fdaf03cc925948827`.

This is a byte-for-byte clean-build comparison from the same pinned prepared
checkout. It is not a fresh clone with reinitialized submodules, and it does not
close ENGINE_GATE.

## Method

1. Recorded the existing arm64 candidate hashes.
2. Removed only generated build output:
   `android/sdk/build` (2.7 GB) and `android/sdk/.cxx` (9.0 GB). No source,
   submodule, asset or patch file was changed.
3. Re-ran the builder to a new output directory:
   `scripts/maps/build-comaps-aar.sh --abi arm64-v8a --output /tmp/abastevo-comaps-release-arm64-repro`.
4. Compared the AAR, `classes.jar` and stripped native core with `sha256sum` and
   `cmp`; the verifier re-checked minSdk 26, release BuildConfig, no debug/Qt,
   stripped core and 0x4000 LOAD alignment.

## Result

```text
First  AAR   1fe196f928f94128f27898ace124c33fa788859bfe89111b19e1bb89848abed5  35,160,271 bytes
Second AAR   1fe196f928f94128f27898ace124c33fa788859bfe89111b19e1bb89848abed5  35,160,271 bytes
cmp AAR      0

First  classes.jar  074122f4de6c6c8c930bd5fae50be0626c527abb8498dae36f0176ce20ee3ca8
Second classes.jar  074122f4de6c6c8c930bd5fae50be0626c527abb8498dae36f0176ce20ee3ca8
cmp classes.jar      0

First  core  08342926e3799d29851baf093e535c3ca107dd19e218eb69b3e1447e7d155ccb  14,675,096 bytes
Second core  08342926e3799d29851baf093e535c3ca107dd19e218eb69b3e1447e7d155ccb  14,675,096 bytes
cmp core     0

Gradle clean build: BUILD SUCCESSFUL in 4m 1s
```

The byte-identical AAR means the existing multi-ABI release artifact remains
valid for this AAR; no app rebuild was needed.

## Limits and remaining obligations

- Same prepared checkout, not a fresh clone. A fresh clone with all 193
  submodules reinitialized and the diagnostic `generator/check_model.cpp` /
  `build_host/` edits excluded remains to be repeated.
- The source patch was already applied; the builder accepted the reverse check.
  A changed upstream SHA, patch, ABI or native core invalidates this result.
- The clean rebuild regenerated generated build directories only; no source
  change was made. The diagnostic generator edit and `build_host/` remain
  outside the runtime patch.
- This does not prove 16 KB runtime, armv7 runtime, T06 native cluster hit-test,
  T07 legal sign-off or ENGINE_GATE closure.
