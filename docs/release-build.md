# Automated Android release APKs

The main-only `android-release` workflow publishes signed Android **staging
prereleases** in this repository after the exact current main SHA passes the
`quick`, `CI` and `backend` push workflows. This is independent from backend/site
autodeploy. It does not certify production, Play Store or iOS.

## Versions and retries

`release/android/channel.json` fixes the baseline at main `972df4f4`, version
3.1.0/code 4. Each subsequent first-parent main delivery increments the patch and
version code once: the first is 3.1.1/code 5. Construction commits inside one merge
count as one delivery. Do not edit this epoch after publication. A future major
or minor baseline change must explicitly preserve increasing version codes.

Versions are derived by `scripts/android/release_flow.py`; Gradle receives both
`androidReleaseVersionName` and `androidReleaseVersionCode`. Local debug builds
retain 3.1.0-debug/code 4. Manual retries use **Actions → android-release → Run
workflow**, on main with its full current SHA; the same three gates still apply.
Same-head retries download and reverify an existing complete publication instead
of rebuilding/replacing assets. A conflicting tag or asset fails closed. Failed
uploads remain draft; no clobber, force-tag or direct-main push is used.

## Signing identity

The first signing identity was generated with the maintainer's authorization.
Its public certificate and SHA-256 pin live in `release/android/`; preserve them
for Android updates. The private PKCS12 key and passwords are encrypted GitHub
environment secrets in **android-staging-release**, with a custom main-only
branch policy:

- `ANDROID_RELEASE_KEYSTORE_BASE64`
- `ANDROID_RELEASE_KEYSTORE_PASSWORD`
- `ANDROID_RELEASE_KEY_ALIAS`
- `ANDROID_RELEASE_KEY_PASSWORD`

Keep an encrypted offline backup of the initial private key and credentials.
They cannot be recovered from GitHub secrets. Do not create a new identity for a
retry. Temporary runner key files are mode 600 and removed on success or failure;
keys/passwords never enter logs, artifacts, source or release attachments.
The existing `ANPFUEL_KEYSTORE_*`/`ANPFUEL_KEY_*` local environment interface is
retained, with PKCS12 store type. Never use the Android debug key for releases.

## Official owner distribution

The original copyright holder's official APK has no forced creator-credit strip.
Third-party users/derivatives must follow the persistent visible-credit steps and
HTML/Android examples in [the licensing guide](license.md). License/copyright and
third-party notices remain bundled as legal records.

## Verification and published files

The workflow builds `:app:assembleRelease :app:verifyReleaseApkSize` with JDK 17,
Android 35 tools, R8/resource shrinking and the pinned native CoMaps engine.
`scripts/android/artifact.py` requires the package `com.anpfuel.app`, exact derived
version, non-debuggable manifest, pinned signing certificate, native map engine
for each advertised ABI and the existing **80-MiB** APK ceiling. It verifies the
in-APK project/original/third-party license bundle and rejects private key files.

Each `android-vX.Y.Z` prerelease attaches the APK, `release.json`, `SHA256SUMS`,
public certificate, current Abastevo license, preserved original MIT notice and
CoMaps/third-party notices. The manifest identifies the exact source commit,
staging origin, version, certificate, ABI set and every payload hash/size.
Publication reads back GitHub asset digests before exposing a draft. Existing
published assets are immutable to this workflow. AAB generation and iOS are not
part of this channel.

Run release-rule tests without signing secrets:

```sh
python3 -m unittest discover -s scripts/android -p 'test_*.py'
```

The old Gate 15/v2.0.0 publication instructions are historical; they do not govern
this channel. [Current acceptance](release-evidence/ANDROID_AUTOMATED_RELEASE_ATTRIBUTION.md)
records actual local/remote evidence and any outstanding delivery obligations.
