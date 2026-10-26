# MAP-29 performance and compatibility contract

Version: 2026-10-26. Written before any MAP-29 measurement. This contract does not
authorize a national rollout, a commercial SLA, physical battery claims or a release.
A result on an emulator is a proxy and is labelled as such.

## Scenarios

- S1 static release candidate: `:app:assembleRelease` from the current head with the pinned
  engine candidate, universal APK; no test maps embedded.
- S2 reference emulator: owned `emulator-5554`, API 35, 16 KB kernel pages, x86_64 host with
  Android ARM64 translation and software/native GLES path; debug app; admitted regional
  `Brazil_Parana_East.mwm`, World and WorldCoasts in app-private files; airplane/offline
  after installation; no external provider.
- S3 low-resource/other-ABI profile: reported separately only if it actually executes. An
  API 30/API 35 profile is never recorded as API 26 evidence. A physical 4 KB arm64 profile
  is recorded as an informational reference, not as 16 KB or battery proof.
- S4 routing/planning: one 1.6 km real native route plus the MAP-27 planner bounded calls;
  cancellation and stale-callback checks.
- S5 measured UI: the MAP-28 real flow (vehicle → map → plan → alternative → failure →
  replan → save → reopen), functional proof; its UI timings are not a frame-pacing budget.

## Metrics and budgets

Mandatory for this local gate:

1. Release universal APK ≤ 80 MiB (existing ceiling, unchanged). Report bytes and MiB;
   download/installed/Play splits are not claimed.
2. Only the supported ARM ABIs are advertised and packaged. Every packaged
   `liborganicmaps.so` LOAD segment is 0x4000-aligned; `zipalign -c -P 16 4` passes. Static
   ELF verification is separate from runtime execution.
3. Engine initialization to native readiness on S2: ≤ 15,000 ms per sample (3 samples).
4. One real 1.6 km route build through the pinned engine on S2: ≤ 5,000 ms per sample
   (3 samples).
5. Total process PSS after native map initialization on S2: ≤ 512 MiB per sample (3 samples).
   This is an emulator proxy, not a low-end physical memory budget.
6. Cancellation: a cancelled native route publishes no result to the cancelled owner and
   cancellation returns in ≤ 1,000 ms; stale callback generation is ignored.
7. App cold launch `am start -W` on S2: ≤ 4,000 ms per sample (3 samples), debug build.

Measured/informational or owed, never inferred as PASS:

- Frame time/pacing and jank of Compose and the native SurfaceView: OWED. The MAP-14
  historical 21–27 FPS profile and SurfaceFlinger queue figure are context, not a current
  budget or acceptance for this AAR/ABI/driver.
- Native/Java heap growth over repeated navigation/rotation/process death: only the PSS
  sample above is measured; a leak-freedom claim is OWED.
- Offline network bytes and Range/304/retry behavior: MAP-24 recorded 9 → 9 requests for
  its regional journey; this MAP-29 run does not re-measure current-head network bytes.
- PostGIS density/concurrency, cursor pagination, EXPLAIN and job backpressure: OWED. No
  SQL/job/schema change is made by MAP-27/28/29 and no disposable PostGIS service is used in
  this run.
- API 26 runtime: OWED; current profiles are API 30/API 35 only.
- ARMv7 JNI runtime: OWED unless the ARMv7 profile actually completes; static packaging is
  not runtime proof.
- GPS/CPU/battery in navigation: OWED; emulator power is not physical autonomy.
- National/dense-region capacity, staging CDN behavior, commercial SLA, release certification:
  OWED.

## Protocol

- Record exact commit, build variant, APK/AAR SHA-256, toolchain, emulator image, kernel page
  size, ABI actually executed, map file hashes, commands, repetitions and host load.
- Do not change a budget after seeing a failure. A failed budget remains failed; a known
  external obligation is PARTIAL/OWED, not a waiver.
- Use `--max-workers=1` for Gradle, do not run `connectedCheck` against all devices, and
  select the owned emulator serial explicitly.
- Sanitize raw output; no route traces, precise GPS, account keys, tokens or private data in
  Git. Synthetic charging fixtures remain labelled as synthetic.
- Gate result: `PERFORMANCE_GATE PASS` only when every mandatory item above has compatible
  proof. Otherwise `PERFORMANCE_GATE PARTIAL`, listing each owed item and its owner/condition.
