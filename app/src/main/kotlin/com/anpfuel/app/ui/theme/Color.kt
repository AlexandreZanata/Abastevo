package com.anpfuel.app.ui.theme

import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.ui.graphics.Color
import com.anpfuel.domain.valueobject.FuelProduct

val AnpBlue = Color(0xFF1565C0)
val AnpBlueDark = Color(0xFF0D47A1)
val AnpGreen = Color(0xFF2E7D32)
val AnpGreenDark = Color(0xFF1B5E20)

val FuelAmber = Color(0xFFF57C00)
val FuelAmberDark = Color(0xFFE65100)
val FuelAmberLight = Color(0xFFFFB74D)
val FuelTeal = Color(0xFF00838F)
val FuelTealLight = Color(0xFF4DD0E1)
val FuelOrange = Color(0xFFEF6C00)
val FuelOrangeLight = Color(0xFFFFAB40)
val FuelPremiumBadge = Color(0xFFFFC107)

/**
 * P19-T03 abastevo brand aliases over the approved blue/green palette.
 * The app chrome is black ([AbastevoChrome]) over near-neutral screens
 * ([EnergiaLightTokens], [EnergiaDarkTokens]) with one green accent
 * ([AbastevoAccent]); these blue aliases remain for fuel tints.
 */
val AbastevoActionBlue = AnpBlue
val AbastevoActionBlueDark = AnpBlueDark
val AbastevoCommunityGreen = AnpGreen
val AbastevoCommunityGreenDark = AnpGreenDark

/** Black of the brand chrome, text and filled actions. */
internal val Ink = Color(0xFF000000)
internal val Paper = Color(0xFFFFFFFF)

/** Warm off-white of light screens; white cards sit on it. */
internal val Mist = Color(0xFFF3F3F0)

internal object ColorTokens {
    val White = Color.White
    val BlueLight = Color(0xFF90CAF9)
    val GreenLight = Color(0xFFA5D6A7)
    val SurfaceLight = Color(0xFFFDFDFD)
    val SurfaceContainerLowLight = Color(0xFFF7F2FA)
    val SurfaceDark = Color(0xFF1C1B1F)
    val SurfaceContainerLowDark = Color(0xFF1D1B20)
}

/** One accent in one theme: the color itself, its label, and its tonal container pair. */
@Immutable
data class AccentTones(
    val accent: Color,
    val onAccent: Color,
    val container: Color,
    val onContainer: Color,
)

/**
 * The single brand color on top of the black chrome. It marks links, the
 * location pin and community elements; it is never the fill of a primary
 * button and never the color of a price. A darker tone serves light screens
 * and a brighter one dark screens and the chrome.
 */
internal object AbastevoAccent {
    val Light = AccentTones(
        accent = Color(0xFF0A7A46),
        onAccent = Color(0xFFFFFFFF),
        container = Color(0xFFDDF5E6),
        onContainer = Color(0xFF06452A),
    )

    val Dark = AccentTones(
        accent = Color(0xFF4ADE80),
        onAccent = Color(0xFF052E16),
        container = Color(0xFF12331F),
        onContainer = Color(0xFFBBF7D0),
    )
}

/**
 * Solid black of the header, top app bars, status bar band and bottom bar,
 * in both themes. The dark hero card is a lifted graphite so it still sits
 * apart from the near-black screens. Content on the chrome is white; the
 * selected tab is a light pill with black content.
 */
internal object AbastevoChrome {
    /** Round buttons and the city field on the black chrome. */
    private val ChromeControl = Color(0xFF1F1F1F)

    val Light = AbastevoChromeColors(
        container = Ink,
        content = Paper,
        contentMuted = Color(0xFFB9BABC),
        control = ChromeControl,
        accent = AbastevoAccent.Dark.accent,
        selected = Mist,
        onSelected = Ink,
        hero = Ink,
    )

    val Dark = AbastevoChromeColors(
        container = Ink,
        content = Paper,
        contentMuted = Color(0xFFB9BABC),
        control = ChromeControl,
        accent = AbastevoAccent.Dark.accent,
        selected = Mist,
        onSelected = Ink,
        hero = Color(0xFF232428),
    )
}

/**
 * Light "Energia" foundation, the default theme: warm off-white screens and
 * white containers at every level (`surface` included, which Material uses
 * for list items and outlined cards), so every card, list, panel, dialog and
 * menu stands out from the screen; black text and an amber notice accent.
 * `primary` is black, so filled actions, switches and prices match the
 * chrome; the brand color comes from [AbastevoAccent]. `surfaceVariant`
 * stays gray for placeholders and tracks. Every text pair meets WCAG AA
 * (4.5:1); controls and indicators meet 3:1.
 */
internal object EnergiaLightTokens {
    val Background = Mist
    val OnBackground = Ink
    val Surface = Paper
    val OnSurface = Ink
    val SurfaceVariant = Color(0xFFEDEDE9)
    val OnSurfaceVariant = Color(0xFF5C5D5A)
    val Outline = Color(0xFF858581)
    val OutlineVariant = Color(0xFFDEDED8)
    val SurfaceContainerLowest = Paper
    val SurfaceContainerLow = Paper
    val SurfaceContainer = Paper
    val SurfaceContainerHigh = Paper
    val SurfaceContainerHighest = Paper
    val SurfaceDim = Color(0xFFDADAD4)
    val SurfaceBright = Paper
    val InverseSurface = Ink
    val InverseOnSurface = Mist

    val Primary = Ink
    val OnPrimary = Paper
    val PrimaryContainer = Paper
    val OnPrimaryContainer = Ink
    val Tertiary = Color(0xFF7A4B00)
    val OnTertiary = Color(0xFFFFFFFF)
    val TertiaryContainer = Color(0xFFFFF3DC)
    val OnTertiaryContainer = Color(0xFF5C3900)

    val Error = Color(0xFFB3261E)
    val OnError = Color(0xFFFFFFFF)
    val ErrorContainer = Color(0xFFF9DEDC)
    val OnErrorContainer = Color(0xFF410E0B)

    val Action = Ink
    val OnAction = Paper
}

/**
 * Dark "Energia" foundation: near-black screens, graphite cards, off-white
 * text and off-white filled actions. Same roles and contrast targets as
 * [EnergiaLightTokens].
 */
internal object EnergiaDarkTokens {
    val Background = Color(0xFF0B0C0D)
    val OnBackground = Mist
    val Surface = Color(0xFF1A1B1E)
    val OnSurface = Mist
    val SurfaceVariant = Color(0xFF2F3033)
    val OnSurfaceVariant = Color(0xFFB3B4B6)
    val Outline = Color(0xFF8B8C8F)
    val OutlineVariant = Color(0xFF34353A)
    val SurfaceContainerLowest = Color(0xFF0B0C0D)
    val SurfaceContainerLow = Color(0xFF1A1B1E)
    val SurfaceContainer = Color(0xFF1F2023)
    val SurfaceContainerHigh = Color(0xFF26272A)
    val SurfaceContainerHighest = Color(0xFF2F3033)
    val SurfaceDim = Color(0xFF0B0C0D)
    val SurfaceBright = Color(0xFF36373A)
    val InverseSurface = Mist
    val InverseOnSurface = Color(0xFF1A1B1E)

    val Primary = Mist
    val OnPrimary = Ink
    val PrimaryContainer = Color(0xFF2F3033)
    val OnPrimaryContainer = Mist
    val Tertiary = Color(0xFFF5C56B)
    val OnTertiary = Color(0xFF3F2A00)
    val TertiaryContainer = Color(0xFF3D2F0E)
    val OnTertiaryContainer = Color(0xFFFFE2A6)

    val Error = Color(0xFFF2B8B5)
    val OnError = Color(0xFF601410)
    val ErrorContainer = Color(0xFF8C1D18)
    val OnErrorContainer = Color(0xFFF9DEDC)

    val Action = Mist
    val OnAction = Ink
}

object FuelProductTint {

    fun colorFor(product: FuelProduct, darkTheme: Boolean): Color = when (product) {
        FuelProduct.GASOLINE_PREMIUM_GRADE -> if (darkTheme) Color(0xFFF48FB1) else Color(0xFFC2185B)
        FuelProduct.ETHANOL -> if (darkTheme) ColorTokens.GreenLight else AnpGreen
        FuelProduct.GASOLINE_REGULAR -> if (darkTheme) ColorTokens.BlueLight else AnpBlue
        FuelProduct.GASOLINE_PREMIUM -> if (darkTheme) ColorTokens.BlueLight else AnpBlueDark
        FuelProduct.DIESEL_S500 -> if (darkTheme) FuelAmberLight else FuelAmberDark
        FuelProduct.DIESEL_S10 -> if (darkTheme) FuelAmberLight else Color(0xFFBF360C)
        FuelProduct.CNG -> if (darkTheme) FuelTealLight else FuelTeal
        FuelProduct.LPG_P13 -> if (darkTheme) FuelOrangeLight else Color(0xFFD84315)
    }

    @Composable
    fun colorFor(product: FuelProduct): Color = colorFor(product, LocalAnpDarkTheme.current)
}
