# Android UI container guidelines

Scope: Compose content containers in `app/src/main/kotlin/com/anpfuel/app`.
This rule keeps cards and grouped surfaces recognizable in light, dark and
dynamic-color themes without turning every layout into an outlined box.

## Default rule

Every new content card or content-grouping `Surface` uses the shared neutral
outline from `AnpContainerDefaults`:

- width: `AnpContainerDefaults.BorderWidth` (`1.dp`, never a hairline);
- color: `AnpContainerDefaults.borderStroke(containerColor)`;
- pass the **effective fill color** (`CardDefaults.cardColors(...).containerColor`
  or the `Surface` color), not the screen background;
- if the container is transparent, pass the color it is actually drawn over
  (`MaterialTheme.colorScheme.surface`).

The helper starts from `MaterialTheme.colorScheme.outlineVariant` and, only
when that fails the minimum contrast against the fill, moves toward
`MaterialTheme.colorScheme.outline`. Because both tokens come from the active
`MaterialTheme.colorScheme`, the rule follows the app theme preference and
Android 12+ dynamic colors. It never reads `isSystemInDarkTheme()`.

The chosen border meets at least `2.0:1` against the effective fill. This is a
decorative grouping boundary, not a control boundary; WCAG 2.2 non-text
contrast (`3:1`) remains the target for controls and state indicators.

## Correct usage

```kotlin
val containerColor = MaterialTheme.colorScheme.surfaceContainerLow

Card(
    modifier = Modifier.fillMaxWidth(),
    colors = CardDefaults.cardColors(containerColor = containerColor),
    border = AnpContainerDefaults.borderStroke(containerColor),
) {
    // Card content
}
```

For a Surface:

```kotlin
Surface(
    color = containerColor,
    shape = MaterialTheme.shapes.large,
    border = AnpContainerDefaults.borderStroke(containerColor),
) {
    // Grouped content
}
```

Keep the existing `Shape` on the card/surface. Material applies the border to
that same shape, so the fill, outline and clip cannot disagree. Do not add a
second `Modifier.border` when the component exposes a `border` parameter.

`ElevatedCard` in the pinned Material 3 version has no `border` parameter; its
screen uses `Modifier.border(AnpContainerDefaults.borderStroke(fill), CardDefaults.shape)`
with the same shape. Revisit that call if the Material version adds the
parameter.

## What not to outline

- `Scaffold`, screens, rows, columns, boxes, icons, images and navigation bars.
- Buttons, chips, text fields, tabs, switches, badges and list items; they keep
  their Material states.
- Components that already provide a suitable outline, such as `OutlinedCard`,
  selectable option rows, error/focus indicators and `ActionCardButton`.
- A nested group inside a card unless it is genuinely an independent card.
- Decorative status/branded surfaces: `HomeCommunityHero` (strong navy artwork
  is already distinct), sync/offline banners, source/time badges, the brand
  banner and the station artwork icon.
- Loading placeholders as a class (`SkeletonCard`); individual skeletons may
  mirror a real card when the transition would otherwise jump.

Exceptions must have a concrete visual reason, not "out of scope".

## Interaction and accessibility

Adding an outline never authorizes interaction. Static summaries stay static;
in particular, `StationsListSummary` must remain without click, button role or
focus. Preserve existing semantics, ripple, gestures and inner actions.

Neutral outlines must not replace focus, selection or error borders. Those
states keep priority and must not change the card's outer size or cause layout
jumps. Preserve 48dp touch targets, reading order, TalkBack descriptions,
normal/enlarged text up to 200%, long names/amounts, narrow screens and
multi-line content. Do not reduce text or fix heights to hide overflow.

Review the active app theme, not only the system theme, in light and dark
modes, with fixed and dynamic colors. Check normal, pressed, selected,
disabled, loading and error states where they exist.

## Review evidence

- JVM rule test: `app/src/test/kotlin/com/anpfuel/app/ui/theme/AnpContainerDefaultsTest.kt`
  checks light/dark direction, minimum contrast against neutral and highlighted
  fills, and that `outlineVariant` is retained when already sufficient.
- Existing text/fuel contrast tests remain applicable; this change does not
  alter text colors.
- Rendered light/dark/dynamic verification and 200% font/device checks are
  manual: VISUAL_VALIDATION_PENDING until an authorized device/emulator run.

## Premium accent exception

Premium entries are the only fixed gold controls. They use
`PremiumColors.current()` from `app/ui/theme/PremiumColors.kt`, which selects a
fixed light/dark palette from the active app theme (`LocalAnpDarkTheme`), not
from dynamic `colorScheme` accents. The crown is a local vector; the action text
still names Premium, so the icon remains decorative for TalkBack.

The gold fill and border are limited to the Premium entry and compact shortcut.
Premium status, capability, notice and action cards use the neutral
`AnpContainerDefaults` outline like other content cards. The palette test
requires 4.5:1 for text and 3:1 for the accent/control boundary. Do not spread
gold hex values or copy this palette into screens.
