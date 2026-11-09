package com.anpfuel.app.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.Composable
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
 * App actions use the navy and blue Energia palettes ([EnergiaLightTokens],
 * [EnergiaDarkTokens]); these blue aliases remain for fuel tints. Green
 * signals contribution/community and never price truth. No new hex values:
 * aliases reuse the audited contrast-checked palette above.
 */
val AbastevoActionBlue = AnpBlue
val AbastevoActionBlueDark = AnpBlueDark
val AbastevoCommunityGreen = AnpGreen
val AbastevoCommunityGreenDark = AnpGreenDark

/** Brand navy shared by the header's lower edge and the bottom bar in both themes. */
internal val BrandNavy = Color(0xFF0A2152)

internal object ColorTokens {
    val White = Color.White
    val BlueLight = Color(0xFF90CAF9)
    val GreenLight = Color(0xFFA5D6A7)
    val SurfaceLight = Color(0xFFFDFDFD)
    val SurfaceContainerLowLight = Color(0xFFF7F2FA)
    val SurfaceDark = Color(0xFF1C1B1F)
    val SurfaceContainerLowDark = Color(0xFF1D1B20)
}

/**
 * Light "Energia" palette from the home mockup: pale blue-white screens, deep
 * navy header and bottom bar, blue actions and a green community accent.
 * Every text pair meets WCAG AA (4.5:1); controls and indicators meet 3:1.
 */
internal object EnergiaLightTokens {
    val Background = Color(0xFFF3F6FC)
    val OnBackground = Color(0xFF0B1B3F)
    val Surface = Color(0xFFFFFFFF)
    val OnSurface = Color(0xFF0B1B3F)
    val SurfaceVariant = Color(0xFFE3EAF6)
    val OnSurfaceVariant = Color(0xFF3C4A66)
    val Outline = Color(0xFF6B7896)
    val OutlineVariant = Color(0xFFD0D9EA)
    val SurfaceContainerLowest = Color(0xFFFFFFFF)
    val SurfaceContainerLow = Color(0xFFFFFFFF)
    val SurfaceContainer = Color(0xFFF7F9FD)
    val SurfaceContainerHigh = Color(0xFFEDF2FA)
    val SurfaceContainerHighest = Color(0xFFE3EAF6)
    val SurfaceDim = Color(0xFFD5DDEB)
    val SurfaceBright = Color(0xFFFFFFFF)
    val InverseSurface = Color(0xFF0B1B3F)
    val InverseOnSurface = Color(0xFFFFFFFF)

    val Primary = Color(0xFF1456C4)
    val OnPrimary = Color(0xFFFFFFFF)
    val PrimaryContainer = Color(0xFFDCE8FB)
    val OnPrimaryContainer = Color(0xFF0B2C6B)
    val Secondary = Color(0xFF14803C)
    val OnSecondary = Color(0xFFFFFFFF)
    val SecondaryContainer = Color(0xFFD8F3E2)
    val OnSecondaryContainer = Color(0xFF0B3D1F)
    val Tertiary = Color(0xFF5B3FBF)
    val OnTertiary = Color(0xFFFFFFFF)
    val TertiaryContainer = Color(0xFFE8E1FA)
    val OnTertiaryContainer = Color(0xFF2A1466)

    val Error = Color(0xFFB3261E)
    val OnError = Color(0xFFFFFFFF)
    val ErrorContainer = Color(0xFFF9DEDC)
    val OnErrorContainer = Color(0xFF410E0B)

    val NavContainer = BrandNavy
    val NavIndicator = Color(0xFF2A6FE6)
    val NavSelectedIcon = Color(0xFFFFFFFF)
    val NavSelectedLabel = Color(0xFFFFFFFF)
    val NavUnselected = Color(0xFFC9D6F0)
}

/**
 * Dark "Energia" palette: graphite surfaces with high-contrast text, blue used
 * only for actions and selection, a green community accent, muted tonal
 * containers and the brand navy bottom bar. Same contrast targets as [EnergiaLightTokens].
 */
internal object EnergiaDarkTokens {
    val Background = Color(0xFF101214)
    val OnBackground = Color(0xFFEEF0F3)
    val Surface = Color(0xFF101214)
    val OnSurface = Color(0xFFEEF0F3)
    val SurfaceVariant = Color(0xFF343940)
    val OnSurfaceVariant = Color(0xFFC4C8CF)
    val Outline = Color(0xFF8E939B)
    val OutlineVariant = Color(0xFF3A3E45)
    val SurfaceContainerLowest = Color(0xFF0B0C0E)
    val SurfaceContainerLow = Color(0xFF181B1F)
    val SurfaceContainer = Color(0xFF1E2227)
    val SurfaceContainerHigh = Color(0xFF282C32)
    val SurfaceContainerHighest = Color(0xFF343940)
    val SurfaceDim = Color(0xFF101214)
    val SurfaceBright = Color(0xFF3A3E45)
    val InverseSurface = Color(0xFFEEF0F3)
    val InverseOnSurface = Color(0xFF2A2D32)

    val Primary = Color(0xFF8FB4FF)
    val OnPrimary = Color(0xFF06307A)
    val PrimaryContainer = Color(0xFF243457)
    val OnPrimaryContainer = Color(0xFFDCE6FF)
    val Secondary = Color(0xFF6EDC9A)
    val OnSecondary = Color(0xFF04391C)
    val SecondaryContainer = Color(0xFF1F3B2C)
    val OnSecondaryContainer = Color(0xFFC5F2D8)
    val Tertiary = Color(0xFFCBB8FF)
    val OnTertiary = Color(0xFF33207A)
    val TertiaryContainer = Color(0xFF3A3470)
    val OnTertiaryContainer = Color(0xFFE8E0FF)

    val Error = Color(0xFFF2B8B5)
    val OnError = Color(0xFF601410)
    val ErrorContainer = Color(0xFF8C1D18)
    val OnErrorContainer = Color(0xFFF9DEDC)

    val NavContainer = BrandNavy
    val NavIndicator = Color(0xFF2A6FE6)
    val NavSelectedIcon = Color(0xFFFFFFFF)
    val NavSelectedLabel = Color(0xFFFFFFFF)
    val NavUnselected = Color(0xFFC9D6F0)
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
