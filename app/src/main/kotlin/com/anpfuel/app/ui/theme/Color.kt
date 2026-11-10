package com.anpfuel.app.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
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
 * The app chrome is monochrome ([EnergiaLightTokens], [EnergiaDarkTokens]) with
 * one green accent ([AbastevoAccent]); these blue aliases remain for fuel tints.
 * No new hex values: aliases reuse the audited contrast-checked palette above.
 */
val AbastevoActionBlue = AnpBlue
val AbastevoActionBlueDark = AnpBlueDark
val AbastevoCommunityGreen = AnpGreen
val AbastevoCommunityGreenDark = AnpGreenDark

/** Ink and paper of the monochrome foundation: text, filled actions and screens. */
internal val Ink = Color(0xFF0A0A0A)
internal val Paper = Color(0xFFFFFFFF)

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
 * The single brand color on top of the monochrome foundation. It marks the
 * selected tab, links, the location pin and community elements; it is never
 * the fill of a primary button and never the color of a price.
 * A darker tone serves white screens and a brighter one black screens.
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
 * Light "Energia" foundation: white screens and cards, ink text, gray tonal
 * containers and an amber notice accent. `primary` is ink, so filled actions,
 * switches and prices stay black; the brand color comes from [AbastevoAccent].
 * Every text pair meets WCAG AA (4.5:1); controls and indicators meet 3:1.
 */
internal object EnergiaLightTokens {
    val Background = Paper
    val OnBackground = Ink
    val Surface = Paper
    val OnSurface = Ink
    val SurfaceVariant = Color(0xFFEEEEEE)
    val OnSurfaceVariant = Color(0xFF5E5E5E)
    val Outline = Color(0xFF8A8A8A)
    val OutlineVariant = Color(0xFFD6D6D6)
    val SurfaceContainerLowest = Paper
    val SurfaceContainerLow = Paper
    val SurfaceContainer = Color(0xFFF6F6F6)
    val SurfaceContainerHigh = Color(0xFFEEEEEE)
    val SurfaceContainerHighest = Color(0xFFE2E2E2)
    val SurfaceDim = Color(0xFFDDDDDD)
    val SurfaceBright = Paper
    val InverseSurface = Ink
    val InverseOnSurface = Paper

    val Primary = Ink
    val OnPrimary = Paper
    val PrimaryContainer = Color(0xFFEEEEEE)
    val OnPrimaryContainer = Ink
    val Tertiary = Color(0xFF7A4B00)
    val OnTertiary = Color(0xFFFFFFFF)
    val TertiaryContainer = Color(0xFFFFF3DC)
    val OnTertiaryContainer = Color(0xFF5C3900)

    val Error = Color(0xFFB3261E)
    val OnError = Color(0xFFFFFFFF)
    val ErrorContainer = Color(0xFFF9DEDC)
    val OnErrorContainer = Color(0xFF410E0B)

    val NavContainer = Paper
    val NavSelected = Ink
    val NavUnselected = Color(0xFF5E5E5E)

    val Action = Ink
    val OnAction = Paper
}

/**
 * Dark "Energia" foundation: true black screens, near-black cards, white text
 * and white filled actions. Same roles and contrast targets as
 * [EnergiaLightTokens].
 */
internal object EnergiaDarkTokens {
    val Background = Color(0xFF000000)
    val OnBackground = Color(0xFFF5F5F5)
    val Surface = Color(0xFF000000)
    val OnSurface = Color(0xFFF5F5F5)
    val SurfaceVariant = Color(0xFF2C2C2C)
    val OnSurfaceVariant = Color(0xFFB0B0B0)
    val Outline = Color(0xFF8A8A8A)
    val OutlineVariant = Color(0xFF333333)
    val SurfaceContainerLowest = Color(0xFF000000)
    val SurfaceContainerLow = Color(0xFF141414)
    val SurfaceContainer = Color(0xFF1A1A1A)
    val SurfaceContainerHigh = Color(0xFF222222)
    val SurfaceContainerHighest = Color(0xFF2C2C2C)
    val SurfaceDim = Color(0xFF000000)
    val SurfaceBright = Color(0xFF333333)
    val InverseSurface = Color(0xFFF5F5F5)
    val InverseOnSurface = Color(0xFF141414)

    val Primary = Color(0xFFF5F5F5)
    val OnPrimary = Ink
    val PrimaryContainer = Color(0xFF2C2C2C)
    val OnPrimaryContainer = Color(0xFFF5F5F5)
    val Tertiary = Color(0xFFF5C56B)
    val OnTertiary = Color(0xFF3F2A00)
    val TertiaryContainer = Color(0xFF3D2F0E)
    val OnTertiaryContainer = Color(0xFFFFE2A6)

    val Error = Color(0xFFF2B8B5)
    val OnError = Color(0xFF601410)
    val ErrorContainer = Color(0xFF8C1D18)
    val OnErrorContainer = Color(0xFFF9DEDC)

    val NavContainer = Color(0xFF000000)
    val NavSelected = Color(0xFFF5F5F5)
    val NavUnselected = Color(0xFF9E9E9E)

    val Action = Paper
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
    fun colorFor(product: FuelProduct): Color = colorFor(product, isSystemInDarkTheme())
}
