# MAP batch inventory and gate state (post-merge, source)

Source integration: `main` at `bbd96568180563ced975b96f8f89f7a3c242522e` (PR #151).
This is an inventory of **source** state, not a production certification. The current
`dev` may carry docs-only notes after the merge; they are not in the merged tree.

| Scope | Source state | Local proof | Staging/production | Remaining obligation |
|---|---|---|---|---|
| MAP-00–13 data/packages | LOCAL_DONE per archived evidence | prior local Data/PBF/PostGIS/package receipts; not re-run | staging pending | staging data/geometry/price admission; national capacity |
| MAP-14 engine | LOCAL_DONE / ENGINE_GATE local PASS | MAP-14 AAR receipt, 16 KB/ELF, release 74.76 MiB now | not production | broad device/driver/ARMv7/API26 certification; fresh reproducibility on any new AAR |
| MAP-15–17 minimum map/UI | LOCAL_PASS local | merged native map/download/activation/search proofs | staging pending | current staging geometry/prices; manual smoke |
| MAP-18 markers/clusters | **PARTIAL / source integrated** | UUID pins, clustering, list, KML distinct purple combined style; real combined **native glyph artwork is not delivered** | not certified | Abastevo-owned combined fuel+electric native pictogram + visual-size/theme proof; would require an engine/style/AAR change with full ENGINE_GATE revalidation |
| MAP-19–20 sheet/prices/offline minimum | LOCAL_PASS local | station/price/offline/nearby/accessibility evidence | staging pending | staging exact reviewed coordinates/prices and nearby; no centroid invention |
| MAP-21 dated events | LOCAL_DONE / source integrated | real PostGIS/replay/outbox tests | staging pending | staging activation/smoke |
| MAP-22 native routes | LOCAL_DONE / source integrated | real offline search/routes/stops/privacy, missing-region refusal | staging pending | cross-region staging smoke; ARMv7/API26 runtime |
| MAP-23 guidance | LOCAL_DONE / source integrated | current phone 3/3 + service 1/1 + engine 1/1, off-route recoverable | not field-certified | long wrong-turn field timing; two-device/account; manual TalkBack |
| MAP-24 offline | LOCAL_DONE / OFFLINE_GATE regional PASS | real MWM, airplane restart, request count 9→9, disk/interruption suite | staging pending | current-head network-byte re-check; cross-region production |
| MAP-25 private | LOCAL_DONE (cloud cases not delivered) | real Room owner isolation, exchange, recorder | not certified | signed/cloud backup and two-device restore |
| MAP-26 EV energy | LOCAL_DONE | 53 independent vectors + properties | N/A | none local; no backend parity required |
| MAP-27 EV stops | LOCAL_DONE / source integrated | domain/application/router/mapper + real phone engine; synthetic chargers | no live coverage | live/operator/hours/tariff data; no arrival guarantee |
| MAP-28 EV journey | LOCAL_DONE / source integrated | real app/UI plan→alternative→fail→replan→save→reopen; 200%/320 dp; synthetic fixtures | no staging | real Android↔Go↔PostGIS/TLS price/session flow; two-account/device; manual TalkBack; airplane process-death |
| MAP-29 performance | **PERFORMANCE_GATE PARTIAL** | declared local budgets, emulator 16 KB + phone 4 KB samples, release/ABI/16 KB | no national/SLA | frame pacing/time-to-content, current-head network, PostGIS load/EXPLAIN, API26, clean ARMv7, physical battery, dense/national |
| MAP-30 car | **EXTERNAL_PENDING** | license/disposition only; no adapter | no host | authorized DHU/head unit, exact Car App dependency admission, host lifecycle/certification |
| MAP-31 acceptance | LOCAL_DONE local union | Kotlin 2,466 (4 opt-in skips), Rust 38, Go/race, real PostGIS 132 migrations, lint/release/size, restore 132/163 | no external | four Kotlin wire/live tests, two-device restore, manual TalkBack, runtime gaps above |
| MAP-32 staging runbook | LOCAL_DONE runbook/restore / **STAGING_PENDING** | runbook + local dump/restore; read-only origin probe only | no staging update | authorized operator/SSH/DB access, backup/current SHA/schema/config, staging window |
| MAP-33 delivery | **INTEGRATED / WIKI_SYNCED (source)** | PR #151 checks all green; merge `bbd96568`; wiki `3c08ac4` from source SHA; dev retained | no production | production/G09/store/pilot/tag and staging are separate |

Gate summary: ENGINE_GATE local PASS; MINIMUM_MAP_GATE LOCAL_PASS/STAGING_PENDING;
OFFLINE_GATE regional PASS; PERFORMANCE_GATE PARTIAL; DELIVERY_GATE source INTEGRATED;
production/G09/store/pilot remains NOT AUTHORIZED. MAP-18 combined native artwork is the
one prior map local criterion still PARTIAL and is not hidden by the merge.

Manual/external executions that did not happen: staging smoke, two-account/two-device
private restore, physical 16 KB runtime (phone is 4 KB; emulator covers 16 KB), API 26,
clean ARMv7, long wrong-turn field timing, live TTS/TalkBack traversal, national/dense
capacity, release R8 runtime and production certification.
