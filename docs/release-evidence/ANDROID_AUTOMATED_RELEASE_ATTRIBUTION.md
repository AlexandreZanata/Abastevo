# Automated Android releases and visible project attribution

Status: LOCAL_DONE
Validation: PASS

## Authorized scope and baseline

The maintainer requested automatic Android versioning and release APK generation
in the delivery flow, an initial GitHub release, and the same visible-attribution
license as the maintainer's TBG project, adapted to Abastevo. iOS is future scope;
no iOS build or native work is resumed. The maintainer confirmed this is the first
Android release and authorized creating/preserving a signing key for GitHub Actions.

Baseline: merged main `972df4f4` (PR #194). Maintained dev remains occupied by the
separate forum effort; construction is isolated without modifying that checkout.
Final delivery must preserve required checks and the maintained-branch policy;
its routing exception has not been inferred from an earlier PR's approval.
The existing runtime defaults to `https://teste.abastevo.com.br`. Android release
build type and internal GitHub prerelease do not certify production, a public pilot,
Play Store publication or G09. Existing production obligations remain separate.

## Behavior contract

B-BR-ANDREL-01 / BUC-ANDREL-01: After the current main revision's quick, CI and
backend push workflows pass, build and verify a signed Android release APK for
that exact revision. Manual retries retain the same gates. PRs, forks, stale main
heads, missing/failed checks and malformed event identity cannot obtain signing
material or publish. Site/backend deployment remains independently operable.

B-BR-ANDREL-02: Version name and Android version code are deterministic and
monotonically increase with protected main deliveries from the explicit release
baseline. The first delivery after 3.1.0/code 4 is 3.1.1/code 5. Multiple construction
commits merged together count as one main delivery. Same-head retries reuse the
same identity; tags/assets are immutable and conflicting prior publication fails.

B-BR-ANDREL-03: Preserve the initial signing identity for updates. Keep private
material out of Git/logs/artifacts and scope CI signing to a main-only Android
staging release environment. Verify the APK certificate against its public pin,
non-debuggable release manifest, package identity, admitted native map engine,
existing 80-MiB size budget, checksums and license/notice bundle before publication.

B-BR-LIC-01 / BUC-LIC-01 (maintainer clarification, 2026-11-05): Adapt the TBG
license to Abastevo and explicitly exempt the original copyright holder's own
distributions from visible self-attribution. The official APK and first-party
interfaces retain their existing navigation without an author-credit strip.
Third-party use/derivatives must display readable persistent Abastevo/creator
credit with both original links. Document exactly how downstream projects comply
in GitHub's README and licensing guide. Preserve original/third-party notices;
do not present this custom license as MIT or OSI-approved.

B-BR-LIC-02: Preserve the prior MIT notice as historical/origin evidence and retain
all existing third-party terms, including the Apache-2.0 CoMaps SDK and its bundled
MIT/BSD/other notices. Own Rust package metadata and distributed backend images must identify/carry
the current license and preserved origin notices. The new license does not relabel government datasets or
third-party assets, or rewrite the license shipped with historical revisions.

## Reference and acceptance

Reference: `AlexandreZanata/TBG-Tupi-Battle-Grounds`, `main/LICENSE`, observed
2026-11-05. Its text permits commercial use and requires persistent visible
creator/project credit with both original destinations. Adapt game identity/use
wording and the explicit original-owner exemption requested above; preserve
its five-section structure and third-party attribution conditions.

Required evidence: version graph/failure/idempotency tests; trusted workflow gate
and publication refusal cases; signed release build and certificate/native/size
checks; verify no forced creator credit in the official APK and preserve normal
navigation; document the third-party visible-credit example; affected compile/static,
whitespace and secret-surface review. Runtime signing/release metadata must be
read back from the actual GitHub release; configuration alone is not publication.

## Construction verification — 2026-11-05

The initial RSA-4096/PKCS12 signing key was created once in the repository's
ignored private `.local/android-release-signing/` directory with mode 700/600.
Only its public certificate/pin is in source. The public SHA-256 fingerprint is
`406597ba94a5bb53b139c1f5049ecd51f0d93b6b638a16f1590c6e8af5cabd1b`.
The main-only GitHub environment `android-staging-release` and its four encrypted
signing secrets were configured and read back; values were never printed.
The private backup remains local; an encrypted offline copy is still an owner
custody obligation. Never regenerate the identity for a retry.

Twenty standard-library Python tests pass: first-parent merge counting, same-head
version determinism, invalid epoch/overflow, stale/fork/PR/malformed/manual event
refusal, all latest exact-head workflow checks, wrong package/debug/certificate,
native engine/missing notices/private material/size rejection, bundle integrity,
immutable asset conflicts, draft recovery/readback before exposure, same-bundle idempotency, stale publication refusal and create-only tag race/permission failure. The
workflow boundary test enforces pinned actions, secret-free read-only gate,
main-only environment, exact checkout, serialized execution and key cleanup.
`actionlint` passes. No runtime dependency was added.

## Superseded attribution-interface draft

The following initial UI checks describe a discarded draft, not the final APK.
The maintainer clarified that own distributions must not show self-credit. All
new attribution components/strings/test harnesses and injected website strips
were removed; their prior successful checks below do not justify retaining them.

Landing: 80 tests, typecheck/build/check pass. Admin: 50 tests and build/check
pass. Browser at desktop and 390px verifies readable persistent project/creator
links and 44px targets on landing and login with no horizontal overflow; the
real frozen portal also shows the credit while SP search and Cuiabá fiscal
filters work. Original MIT notice SHA-256 remains byte-identical to main's
previous LICENSE (`d85b4ca093fe9a6ce497232da01c7cd44f7738fb723f8d7f40c033a4394f4167`).
Debug/release Kotlin compilation and lintDebug passed before the final extracted
footer harness; final device/layout and signed APK evidence follows below.
Static mobile guards, whitespace and tracked secret-surface checks pass.

Final debug/instrumentation build and lintDebug passed (5m07s). Three emulator
API-30 tests passed in 5.564s: light normal navigation, dark 200% font navigation,
and dark 200% active guidance. Both attribution destinations open independently;
the normal action/stop button stays above the measured footer and remains usable.
The existing 16-KiB AVD suffered a system_server SIGSEGV before instrumentation;
its result is not app acceptance. A separate private API-30 AVD was created from
an installed image, without changing the physical phone or original AVD data.

The physical Android debug APK was updated with `adb install -r` (no uninstall,
clear or downgrade). A fresh device UI dump confirms the visible Abastevo
project link and Alexandre Zanata (@AlexandreZanata) creator link in Portuguese.
Private screenshot/UI evidence stays ignored; no account data entered Git.
The physical layout review shortened the project-link caption in all eight
locales to keep the two credit controls on one row at normal phone widths;
large text continues to wrap. This resource-only refinement is included in the
final rebuild/layout rerun below.

## Final owner-exempt build validation

The owner's clarification supersedes the draft credit UI above. All app source
changes for attribution were removed: the root Scaffold and navigation are
byte-identical to main. The official landing/admin builds and portal templates
also have no injected author strip. The README and licensing guide describe the
third-party obligation, full destinations, owner exemption, and complete HTML/
Compose layout examples. The current LICENSE states that exemption explicitly.

Final landing: 79 tests, build/check pass; admin: 50 tests, build/check pass.
Final portal race tests, vet and pinned staticcheck pass. The real national
candidate verifies 52,863 pages / 62,554 files within all unchanged budgets; see
the linked NAT evidence. Cargo metadata resolves the custom owner license file.
The non-root backend image builds and carries byte-exact current LICENSE,
original MIT notice and third-party notice files with the correct license label.

Owner APK assembleDebug/lintDebug pass (5m15s). Physical Android was updated in
place again; a fresh UI dump shows no creator/project-credit strip and all four
normal navigation actions remain available. The initial key is unchanged.
Signed owner-exempt assembleRelease/verifyReleaseApkSize passed (4m46s).
Independent apksigner/aapt/ZIP verification passed for 3.1.1/code 5, package
com.anpfuel.app, non-debuggable manifest, the pinned original certificate,
arm64-v8a/armeabi-v7a native engines, and byte-exact current/original/third-party
notices. APK: 78,996,987 bytes (75.34 MiB), SHA-256
`d2e00044aa3468f63a3e37b105abcecd57fbed7fc35c9b049b9c6a3d92058a46`.
The signed release installed and cold-started successfully in the isolated
emulator (819ms reported by am start); the running UI has no author-credit strip.
The local validation manifest deliberately uses a nonpublishable LOCAL marker;
only a verified protected-main revision can generate the public release manifest.

Final rule suite: 20 tests pass, including current-head/check revalidation before
signing and publication, failed final recheck before any tag/release mutation,
and complete workflow pagination. Actionlint, secret scan and whitespace checks
pass. Browser readback confirms zero injected author strips in the official
landing/login/real-data portal, with 50 SP search results still functioning.

Remote state: INTEGRATION_PENDING. No GitHub APK release has been published yet.
The main-only key environment is configured; protected final integration, actual
CI, first automatic release readback, staging deploy and wiki remain final
publication steps. Maintained dev still carries the separate open forum PR #192;
this task's isolated delivery needs its own routing approval. No iOS or production
certification is claimed.
