# Bottom tab base navigation — 2026-10-13

Task: user-requested bounded P19-T02/P20-T02 navigation correction.
Base: `8a4503c`; isolated worktree because maintained dev contains another agent's edits.

## Behavior contract

- B-BR-NAV-01: Every bottom-bar tap opens the selected tab's base destination: Explore → Home, Community → Community, Profile → Profile. A previously visited child must not be restored by that action.
- B-BR-NAV-02: Repeated taps at a base destination do not add duplicate entries. Existing screen Back actions and persisted application data retain their contracts.
- BUC-NAV-01: Given Home → Select location (including deeper search/location paths), tapping Explore returns to Home.
- BUC-NAV-02: Given Community → Contributions or Profile → Settings → Help, tapping the corresponding tab returns to its base.
- BUC-NAV-03: Given another tab was left on a child screen, switching back opens its base. Repeated taps and subsequent Back do not reveal that discarded child.

## Scope and validation

Presentation navigation only: no domain/auth/storage/backend/dependency change. Reuse the existing Navigation Compose controller and bottom bar. Stop saving/restoring child back stacks for bottom-bar actions; keep single-top navigation and the existing graph start anchor.

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING (dev delivery only; native execution OWED).

Executed from the isolated worktree:

```sh
ANDROID_HOME=/data/dev/android/sdk/Sdk ./gradlew :app:testDebugUnitTest --tests '*navigation.*' :app:lintDebug :app:assembleDebug :app:assembleDebugAndroidTest --no-parallel --max-workers=2 --console=plain
```

Result: BUILD SUCCESSFUL (3m 23s), 18 JVM navigation tests / zero failures, errors or skips (Routes 10, tab mapping 7, auth manifest 1). Android lint: zero errors/fatal issues, 222 warnings. Debug app and instrumentation APK compilation passed. Five new actual-controller/bar instrumentation cases cover location/deeper paths, Community/Profile child stacks, switching tabs, repeated taps and an initial location destination. These cases compiled but were NOT EXECUTED; native behavior remains OWED under the deferred manual/device batch. Existing JVM route tests do not prove native stack behavior.

`git diff --check` passed. Scoped source/test/docs review found no credentials, private data, GPS or media added. No dependency or domain change; no backend/SQL checks required.

Tested source SHA-256:


Publication: user authorized push to dev; no main merge, deployment or wiki publication in this task. Preserve the occupied checkout and all unrelated changes. Recovery: revert only this task's commit.

- `app/src/main/kotlin/com/anpfuel/app/navigation/AnpNavGraph.kt`: `188f78d6385980cb8fc99c99b908a50b616a80587b789df717c98b1a0f5f7727`.
- `app/src/main/kotlin/com/anpfuel/app/navigation/TabNavigation.kt`: `f1bca9648aa9d5411fbd4c3991577acc237f0947c7779cc7e498180e3afc2eee`.
- `app/src/androidTest/kotlin/com/anpfuel/app/navigation/BottomTabNavigationDeviceTest.kt`: `9840a395e3e7c47ca6d3032fa40382f00b3a9bbe80d167cd7bba12eb37bf1357`.

Remote verified: `git@github.com:AlexandreZanata/Abastevo.git` (previous repository name redirects to Abastevo); push permission present. Main protection still requires strict `Quick verification` (GitHub Actions app 15368). No checks/merge requested or inferred for main.
