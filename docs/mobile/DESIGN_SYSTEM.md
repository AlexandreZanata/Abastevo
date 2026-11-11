# Android design system

Scope: Compose UI in `app/src/main/kotlin/com/anpfuel/app`. The identity is
"black and green" (owner choice, 2026-11-11): the header, top app bars,
status bar band and bottom bar are one solid black chrome in both themes,
over warm off-white screens where every container is white (light) or
near-black screens with graphite cards (dark), with a single green accent.
The light theme is the default; Perfil switches to dark for the session. Fuel icons, prices
and the map are the only colorful content. Typography is shared with the
landing page (`landing/static/assets/styles.css`). Containers keep the rules
in [UI_CONTAINER_GUIDELINES](UI_CONTAINER_GUIDELINES.md).

## Where the tokens live

| Token group | File |
| --- | --- |
| Foundation colors | `ui/theme/Color.kt` (`EnergiaLightTokens`, `EnergiaDarkTokens`) |
| Accent | `ui/theme/Color.kt` (`AbastevoAccent`) |
| Chrome (header, bars, hero) | `ui/theme/Color.kt` (`AbastevoChrome`), `ui/theme/AbastevoChromeColors.kt` |
| Filled action colors | `ui/theme/AbastevoActionColors.kt` |
| Bottom bar colors | `ui/theme/AbastevoNavColors.kt` (derived from the chrome) |
| Typography | `ui/theme/Type.kt` |
| Shapes | `ui/theme/Shape.kt` |
| Spacing | `ui/theme/Spacing.kt` |
| Buttons, chips, selection | `ui/designsystem/` |

`AnpFuelTheme` wires all of them into `MaterialTheme`, so components read
`MaterialTheme.colorScheme`, `MaterialTheme.typography` and
`MaterialTheme.shapes` instead of literal values. Chrome colors are read from
`LocalAbastevoChromeColors`.

## Color

Foundation (every value is a near-neutral gray, at most a few steps warm):

| Role | Light | Dark |
| --- | --- | --- |
| `background` (screens) | `#F3F3F0` | `#0B0C0D` |
| `surface`, cards (`surfaceContainerLow`) | `#FFFFFF` | `#1A1B1E` |
| `surfaceContainer` / `High` / `Highest`, `primaryContainer` | `#FFFFFF` | `#1F2023` / `#26272A` / `#2F3033`, `#2F3033` |
| `surfaceVariant` (placeholders, skeletons, tracks) | `#EDEDE9` | `#2F3033` |
| `onSurface`, `primary` | `#000000` | `#F3F3F0` |
| `onSurfaceVariant` | `#5C5D5A` | `#B3B4B6` |
| `outline` / `outlineVariant` | `#858581` / `#DEDED8` | `#8B8C8F` / `#34353A` |
| filled button fill / label | `#000000` / `#FFFFFF` | `#F3F3F0` / `#000000` |

Every light container role is white, `surface` included (Material uses it
for list items, outlined cards and segmented buttons), so cards, lists,
panels, dialogs, menus and Material default cards all stand out from the
off-white screen; screens paint `background`, never `surface`; a gray fill
is never used for a container. Selection inside a list uses the accent
container, not a darker gray. `primary` is black, the same color as the
chrome: filled buttons, selected chips, the selected segmented button,
switches and large prices are black on light screens and off-white on dark
ones.

Chrome (`AbastevoChrome`):

| Role | Light | Dark |
| --- | --- | --- |
| `container` (header, top bars, status bar band, bottom bar) | `#000000` | `#000000` |
| `content` / `contentMuted` | `#FFFFFF` / `#B9BABC` | `#FFFFFF` / `#B9BABC` |
| `control` (round buttons, city field) | `#1F1F1F` | `#1F1F1F` |
| `accent` (location pin) | `#4ADE80` | `#4ADE80` |
| `selected` / `onSelected` (tab pill, week chip, action on hero) | `#F3F3F0` / `#000000` | `#F3F3F0` / `#000000` |
| `hero` (brand hero card) | `#000000` | `#232428` |

The dark hero is lifted above the dark cards so it does not melt into the
near-black screen.

Accent (`AbastevoAccent`, mapped to the Material `secondary` roles): green,
`#0A7A46` on white screens and `#4ADE80` on black ones, with tonal containers
`#DDF5E6` and `#12331F`.

The accent marks text links (`TextButton`), the location pin and community
elements. It is never the
fill of a primary button and never the color of a price.

Other fixed meanings:

- **Amber** (`tertiary`) is notice: stale data and operational notes.
- Fuel tints (`FuelProductTint`) and Premium colors (`PremiumColors`) are
  separate fixed palettes.

`BrandPaletteContrastTest` holds the contrast targets for both themes,
chrome and hero included: 4.5:1 for every text pair, 3:1 for control fills,
outlines and indicators.

## Chrome

- **Status bar band** (`AbastevoStatusBarBand`): drawn once at the app root,
  so every screen, the map included, has the solid chrome behind the status
  bar and light status icons.
- **Header** (`AbastevoHeader`): solid chrome under the status bar, no
  hairline. Logo and white name on the left, round `control` notification and
  profile buttons on the right. On Explorar and Comunidade one `control` pill
  field holds the city (green pin) and, as a light chip at its right end, the
  survey week.
- **Top app bar** (`AnpTopAppBar`): solid chrome with white title and icons.
- **Bottom bar** (`AbastevoBottomBar`): solid chrome extending under the
  gesture area, no hairline. The selected tab sits in a light pill with a
  filled icon and a bold graphite label; the others use outlined icons in
  `contentMuted`.
- **Hero card** (`TankFillCostCard` on Explorar): at most one per screen,
  filled with `hero`, white text and a light `selected` action; no outline.
- **Entry cards** (Comunidade): contributing a price always comes first and
  is the screen's hero (`hero` fill, uppercase label, title, one line of
  explanation, a light filled action). The forum follows as one white
  tappable row card: black icon badge, title, one line and a chevron. No
  accent tint.
- **Week picker**: one card says what every week contains; each week is a
  white card with a calendar badge (black once downloaded), the date, an
  "Atual" tag for the latest week and a black "Baixar" action or an outlined
  "Baixada" state. The loading skeleton mirrors these cards.
- **Empty states** (`EmptyState`): a white outlined card with a black icon
  badge, the message as title, an optional hint and an optional outlined
  action that leads out of the empty state.
- **Forum** (social feed): white screen under the black top bar, category
  underline tabs, posts edge to edge with hairlines between them. Each post
  has a round avatar (brand mark and verified badge for official Abastevo
  posts, initials for people), author line with relative time, kind and
  category, title, excerpt and icon actions (comment, share). The compose
  button is a round black FAB. Comments and replies follow the same layout.
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
| `extraSmall` | 8dp | menus and small marks |
| `small` | 12dp | buttons, text fields |
| `medium` | 16dp | cards and content containers (`AnpContainerDefaults.containerShape()`) |
| `large` | 20dp | large panels and the hero card |
| `extraLarge` | 28dp | dialogs and bottom sheets |

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
| `Button` | 12dp radius, 48dp minimum height, black fill (off-white fill in dark) |
| `OutlinedButton` | 12dp radius, 48dp minimum height, 1.5dp black border |
| `TextButton` | 12dp radius, accent label |
| `FilterChip` | pill; selected: solid black with inverse label |
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
