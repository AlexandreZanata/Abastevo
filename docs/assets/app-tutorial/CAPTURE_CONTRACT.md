# App tutorial capture contract

This contract links three things that must stay equal:

1. an Android screen (Compose sources and string resources in `app/`),
2. its tutorial capture in `docs/assets/app-tutorial/<feature>/NN-*.png`,
3. every page or field of the public site (`landing/`) that shows that capture
   or quotes a label from it.

When an app screen changes, the affected captures are taken again on the
emulator under the same conditions as before, and the site page or field that
uses them is updated in the same delivery. `manifest.json` is the
machine-readable side of this contract and
[`scripts/app_tutorial/tutorial_contract.py`](../../../scripts/app_tutorial/tutorial_contract.py)
enforces it.

## Rules

| ID | Rule |
|---|---|
| C1 | Every PNG in this folder is listed once in `manifest.json` with a caption, the string resources it shows (`strings`), any extra sources (`extraSources`), the derived Compose `sources`, its `sha256` and its context (city, account, notes). Numbers run `01..NN` per feature without gaps. `validate` fails otherwise. |
| C2 | A change to a `source` file, to a listed string (any locale) or to a drawable used by a source makes the step **stale**. The change that makes it stale, or at the latest the next landing update, recaptures the step under the frozen conditions (C4) and runs `refresh` for it. Purely internal changes that leave the screen pixel-identical may be re-baselined with `refresh` after a visual comparison; say so in the PR. |
| C3 | The site never hot-links these masters. Derivatives keep the stem: `<feature>-<NN-slug>-<width>w.webp` (for example `forum-05-write-comment-720w.webp`). Site text that names an app control quotes the current pt-BR string exactly (for example "Rota e ir"). This is how `stale` finds the image uses and the quoted labels to update. |
| C4 | Capture conditions are frozen in `manifest.json` → `capture` and applied by `scripts/app_tutorial/emulator.py setup`. Changing a condition (device profile, size, density, locale, theme, status bar, cities, data week) means recapturing every step. |
| C5 | Privacy: synthetic emulator locations only, public ANP records only, a throwaway username-only account deleted at the end, nothing submitted to the community, forum, ratings or corrections (drafts only), no photos of people or plates, no OCR training images. `FLAG_SECURE` screens are never captured or bypassed; they are described in `textOnlySteps`. |
| C6 | Captures run on an emulator only. `emulator.py` refuses any serial that is not `emulator-*`; never use a contributor's or the owner's phone. |
| C7 | Text-only steps (`textOnlySteps`) follow the same staleness rule through their `sources`; update their wording when those screens change. |

## Frozen conditions

| Item | Value |
|---|---|
| App | Debug build (`./gradlew :app:assembleDebug`), package `com.anpfuel.app.debug`, default API origin `https://abastevo.com.br` |
| AVD | `abastevo-tutorial-api36`: `system-images;android-36;google_apis;x86_64`, device `pixel_7`, 3072 MB RAM, `hw.keyboard=yes` |
| Display | `wm size 1080x1920`, `wm density 420`, light theme |
| Locale | system `pt-BR` (root `setprop persist.sys.locale`), app `pt-BR` (`cmd locale set-app-locales`), time zone `America/Sao_Paulo` |
| Status bar | System UI demo mode: 10:00, battery 100 %, Wi-Fi 4 bars, mobile and notifications hidden |
| Network | emulator Wi-Fi `AndroidWifi`, mobile data off (map packs are Wi-Fi only) |
| Input | Gboard disabled and stylus handwriting off so no keyboard covers the screen; text typed with `adb shell input text` |
| Cities | São Paulo, SP (fixes at -23.5505, -46.6333) and Sorriso, MT (-12.545, -55.711); map, route and navigation use Sorriso |
| Data | ANP week 8–14 Nov 2026 active; 1–7 Nov, 18–24 Oct and 4–10 Oct downloaded in the app for history |
| Synthetic user data | vehicle "Meu carro", 50 L, gasoline, cheapest station, price-drop alert on; one fuel-log entry (40 L, R$ 250,00, 12 500 km); favourite "Posto Dallas" |
| Output | opaque RGB PNG 1080 × 1920, then `optipng -o2 -strip all` (lossless) |

## Recapture procedure

1. **Find what is stale.**

   ```bash
   python3 scripts/app_tutorial/tutorial_contract.py stale
   ```

   The report lists each stale image, why (source, string or drawable), the
   landing files that show it and the landing lines that quote a changed pt-BR
   label, with the old and new text. Use `--head <rev>` to compare a commit and
   `--json` for tooling.

2. **Build the app** from the commit being documented, in a clean worktree
   (system Java is a JRE; use the cached JDK 17):

   ```bash
   JAVA_HOME=~/.gradle/jdks/eclipse_adoptium-17-amd64-linux.2 ./gradlew :app:assembleDebug --offline
   ```

3. **Start the tutorial emulator.** Create the AVD once, outside the
   repository, then boot it headless:

   ```bash
   avdmanager create avd -n abastevo-tutorial-api36 -k "system-images;android-36;google_apis;x86_64" -d pixel_7
   ```

   ```bash
   emulator -avd abastevo-tutorial-api36 -port 5590 -no-window -no-audio -no-boot-anim -no-snapshot-save -gpu swiftshader_indirect -memory 3072
   ```

4. **Apply the frozen conditions and install.**

   ```bash
   ANDROID_SERIAL=emulator-5590 python3 scripts/app_tutorial/emulator.py setup
   ```

   ```bash
   adb -s emulator-5590 install -r app/build/outputs/apk/debug/app-debug.apk
   ```

   Restart the emulator framework once after the first `setup` so the system
   locale applies (`adb -s emulator-5590 shell 'stop; start'`), then run
   `setup` again.

5. **Reproduce the state** named in the step's `context`: city
   (`emulator.py gps sorriso-mt` and pick the city in the app), signed-in or
   guest, the active week and the synthetic data above. Navigate to the screen
   exactly as the caption says.

6. **Capture** with the same file name:

   ```bash
   ANDROID_SERIAL=emulator-5590 python3 scripts/app_tutorial/emulator.py shot docs/assets/app-tutorial/map/05-route-preview.png
   ```

   A black capture fails: the screen is `FLAG_SECURE` and becomes a text-only
   step. Then optimise losslessly:

   ```bash
   optipng -quiet -o2 -strip all docs/assets/app-tutorial/map/05-route-preview.png
   ```

7. **Update the manifest.** Record the new hash and the commit the build came
   from; when a caption, string list or step order changes, edit
   `manifest.json` first, then:

   ```bash
   python3 scripts/app_tutorial/tutorial_contract.py refresh map/05-route-preview.png --commit HEAD
   ```

   ```bash
   python3 scripts/app_tutorial/tutorial_contract.py sync-sources --write
   ```

   ```bash
   python3 scripts/app_tutorial/tutorial_contract.py render-readme --write
   ```

   ```bash
   python3 scripts/app_tutorial/tutorial_contract.py validate
   ```

8. **Update the site.** For every `landing image use` in the stale report,
   regenerate the derivative from the new master (same stem, rule C3). For
   every `landing text` line, replace the old label with the new pt-BR string.
   For steps with no current landing use, review the `landingTargets` pages.
   Then run the landing checks:

   ```bash
   cd landing && npm ci && npm run typecheck && npm test && npm run build
   ```

9. **Reset the emulator** and deliver the app change, the recaptures, the
   manifest and the landing update in one pull request:

   ```bash
   ANDROID_SERIAL=emulator-5590 python3 scripts/app_tutorial/emulator.py reset
   ```

## Automated checks

`scripts/quick-verify.sh` runs the contract tests and `validate` (blocking) and
prints the `stale` report (informational; it skips when the clone has no
history back to the capture commit). Before publishing a landing update, run
`stale --strict`: it exits 1 while any step is stale.
