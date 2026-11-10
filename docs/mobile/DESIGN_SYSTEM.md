# Android design system

Scope: Compose UI in `app/src/main/kotlin/com/anpfuel/app`. The app chrome is
monochrome, in the spirit of ride-hailing apps: ink on white in the light
theme, white on true black in the dark theme, with a single accent color.
Fuel icons, prices and the map are the only colorful content. Typography and
radii are shared with the landing page (`landing/static/assets/styles.css`).
Containers keep the rules in
[UI_CONTAINER_GUIDELINES](UI_CONTAINER_GUIDELINES.md).

## Where the tokens live

| Token group | File |
| --- | --- |
| Foundation colors | `ui/theme/Color.kt` (`EnergiaLightTokens`, `EnergiaDarkTokens`) |
| Accent | `ui/theme/Color.kt` (`AbastevoAccent`) |
| Filled action colors | `ui/theme/AbastevoActionColors.kt` |
| Bottom bar colors | `ui/theme/AbastevoNavColors.kt` |
| Typography | `ui/theme/Type.kt` |
| Shapes | `ui/theme/Shape.kt` |
| Spacing | `ui/theme/Spacing.kt` |
| Buttons, chips, selection | `ui/designsystem/` |

`AnpFuelTheme` wires all of them into `MaterialTheme`, so components read
`MaterialTheme.colorScheme`, `MaterialTheme.typography` and
`MaterialTheme.shapes` instead of literal values.

## Color

Foundation (every value is a neutral gray):

| Role | Light | Dark |
| --- | --- | --- |
| `background`, `surface` | `#FFFFFF` | `#000000` |
| cards (`surfaceContainerLow`) | `#FFFFFF` | `#141414` |
| `surfaceContainer` / `High` / `Highest` | `#F6F6F6` / `#EEEEEE` / `#E2E2E2` | `#1A1A1A` / `#222222` / `#2C2C2C` |
| `onSurface`, `primary` | `#0A0A0A` | `#F5F5F5` |
| `onSurfaceVariant` | `#5E5E5E` | `#B0B0B0` |
| `outline` / `outlineVariant` | `#8A8A8A` / `#D6D6D6` | `#8A8A8A` / `#333333` |
| filled button fill / label | `#0A0A0A` / `#FFFFFF` | `#FFFFFF` / `#0A0A0A` |

`primary` is ink, not a brand color. Filled buttons, selected chips, the
selected segmented button, switches and large prices are therefore black on
white screens and white on black ones.

Accent (`AbastevoAccent`, mapped to the Material `secondary` roles): green,
`#0A7A46` on white screens and `#4ADE80` on black ones, with tonal containers
`#DDF5E6` and `#12331F`.

The accent marks the selected tab, text links (`TextButton`), the location
pin and community elements, such as the highlighted "contribute a price"
card. It is never the fill of a primary button and never the color of a
price.

Other fixed meanings:

- **Amber** (`tertiary`) is notice: stale data and operational notes.
- Fuel tints (`FuelProductTint`) and Premium colors (`PremiumColors`) are
  separate fixed palettes.

`BrandPaletteContrastTest` holds the contrast targets for both themes: 4.5:1 for every text pair, 3:1 for control fills, outlines and
indicators.

## Chrome

- **Header** (`AbastevoHeader`): flat on the screen color with a hairline
  below. Logo and name on the left, round gray notification and profile
  buttons on the right. On Explorar and Comunidade one gray pill field holds
  the city (accent pin) and, as a chip at its right end, the survey week.
- **Top app bar** (`AnpTopAppBar`): flat on the screen color.
- **Bottom bar** (`AbastevoBottomBar`): flat on the screen color with a
  hairline above. The selected tab has an accent marker on the top edge, a
  filled icon and a bold ink label; the others use outlined icons in gray.
- **Entry cards** (Comunidade): a small uppercase label says what the card
  is, then icon, title, one line of explanation and one action. The main entry
  (contribute a price) is accent-tinted with the filled button and always
  comes first; the secondary entry (forum) is neutral with an outlined button.
- **Map**: controls float over the map with a shadow instead of sitting in a
  card: a search pill, then city and price-source chips; details and status
  open in a panel below them. Round map buttons use the screen color with ink
  icons. The bottom sheet uses the screen color with pill actions.

## Typography

| Material roles | Family | Weights |
| --- | --- | --- |
| display, headline, title, label | Manrope | 500, 600, 700, 800 |
| body | Source Sans 3 | 400, 600, 700 |

Titles and prices use Manrope with slightly negative tracking, as on the
landing headings. Body sizes sit 1sp above the Material defaults (17/15/13)
because Source Sans 3 has a smaller x-height than Roboto. All sizes are `sp`
and follow the user's font scale.

Characters outside the bundled Latin subset (Cyrillic, Japanese, Chinese,
arrows) fall back to the system font.

## Shape

| Token | Radius | Used by |
| --- | --- | --- |
| `extraSmall`, `small` | 8dp | buttons, text fields, menus |
| `medium` | 12dp | cards and content containers (`AnpContainerDefaults.containerShape()`) |
| `large` | 16dp | large panels |
| `extraLarge` | 24dp | dialogs and bottom sheets |

Pills (full radius) are for chips, segmented controls, the header field and
the floating map controls; circles for icon buttons and avatars.

## Spacing and size

`AbastevoSpacing` is a 4dp scale (`xs` 4, `sm` 8, `md` 12, `lg` 16, `xl` 24,
`xxl` 32). Screen content, cards and the header align on the 16dp
`screenGutter`. Buttons and tappable rows are at least 48dp tall
(`controlMinHeight`).

## Components

Import buttons and chips from `com.anpfuel.app.ui.designsystem`, not from
`androidx.compose.material3`. They take the same parameters as the Material
components and only change the defaults:

| Component | Defaults |
| --- | --- |
| `Button` | 8dp radius, 48dp minimum height, ink fill (white fill in dark) |
| `OutlinedButton` | 8dp radius, 48dp minimum height, 1.5dp ink border |
| `TextButton` | 8dp radius, accent label |
| `FilterChip` | pill; selected: solid ink with inverse label |
| `AssistChip` | pill |

For a segmented control pass `AbastevoSelectionDefaults.segmentedButtonColors()`.

Use one filled button per card or screen section for the main action;
secondary actions are outlined or text buttons.

## Bundled fonts

- Need: user-requested Android typography matching the landing page; no
  runtime font download and no third-party font request.
- Files: `app/src/main/res/font/` (seven static TTF files, about 255 KB).
- Source: the Latin subsets already vendored for the landing
  (`landing/static/assets/fonts/`, provenance in `provenance.json` there).
  Each file is a static weight instance of that variable WOFF2, produced
  locally with fontTools 4.63.0 (`varLib.instancer`); nothing was downloaded.
- License: SIL Open Font License 1.1. The license texts ship in the APK at
  `app/src/main/assets/licenses/`.
- Security review: font data only, no executable content; generated from
  files whose hashes are pinned in the landing provenance record.
- Open item before a store release: Source Sans 3 carries the Reserved Font
  Name "Source". The bundled files are derived instances that keep that name.
  Replace them with the unmodified static files from the upstream release, or
  rename the derived family, before distributing a release build.

| File | SHA-256 |
| --- | --- |
| `manrope_medium.ttf` | `b59673b0e5846a8a65c6358bc484f7470131ca1d06a3fd804d8f4d951730ba7d` |
| `manrope_semibold.ttf` | `ebda9f776199665aa6021d51b978e156a78c00bef941ffcf1f2c0ee5fae10f49` |
| `manrope_bold.ttf` | `9c03dbe013ae7079e635de655c7a86f8a24a60576baf5f4abbebe5dac61ae653` |
| `manrope_extrabold.ttf` | `46b1b6028616506db24b507c80f40f167b8cfbbf9d125a81769cd2232d9d9910` |
| `source_sans_3_regular.ttf` | `51f286e2d5f3319fd694963b09828962d3d8c53794893d6ce1dbd9f4ae9f9d26` |
| `source_sans_3_semibold.ttf` | `a36ca3ecdffc9381a058966c498ffe54c49f8c9bac0e6a9b1cc858a9c4a7593b` |
| `source_sans_3_bold.ttf` | `99d6658a807675737956280bd04ef03d48186da031aa137609b42c49c5713783` |
