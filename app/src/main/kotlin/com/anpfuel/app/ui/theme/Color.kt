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
 * App actions use the teal Energia palettes ([EnergiaLightTokens],
 * [EnergiaDarkTokens]); these blue aliases remain for fuel tints. Green
 * signals contribution/community and never price truth. No new hex values:
 * aliases reuse the audited contrast-checked palette above.
 */
val AbastevoActionBlue = AnpBlue
val AbastevoActionBlueDark = AnpBlueDark
val AbastevoCommunityGreen = AnpGreen
val AbastevoCommunityGreenDark = AnpGreenDark

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
 * Light "Energia" brand palette: pure white screens, deep teal actions and a
 * cyan tonal accent for selection. The app runs without dynamic colors, so
 * Android 12+ wallpaper colors never replace the brand.
 * Every text pair meets WCAG AA (4.5:1); controls and indicators meet 3:1.
 */
internal object EnergiaLightTokens {
    val Background = Color(0xFFFFFFFF)
    val OnBackground = Color(0xFF111418)
    val Surface = Color(0xFFFFFFFF)
    val OnSurface = Color(0xFF111418)
    val SurfaceVariant = Color(0xFFDEE8E6)
    val OnSurfaceVariant = Color(0xFF40504E)
    val Outline = Color(0xFF6B7A78)
    val OutlineVariant = Color(0xFFC8D4D2)
    val SurfaceContainerLowest = Color(0xFFFFFFFF)
    val SurfaceContainerLow = Color(0xFFF4F8F7)
    val SurfaceContainer = Color(0xFFECF3F2)
    val SurfaceContainerHigh = Color(0xFFE5EEEC)
    val SurfaceContainerHighest = Color(0xFFDCE7E5)
    val SurfaceDim = Color(0xFFD9DEE5)
    val SurfaceBright = Color(0xFFFFFFFF)
    val InverseSurface = Color(0xFF2A2F36)
    val InverseOnSurface = Color(0xFFEEF1F5)

    val Primary = Color(0xFF0F766E)
    val OnPrimary = Color(0xFFFFFFFF)
    val PrimaryContainer = Color(0xFFC9F2EC)
    val OnPrimaryContainer = Color(0xFF003732)
    val Secondary = Color(0xFF0E7490)
    val OnSecondary = Color(0xFFFFFFFF)
    val SecondaryContainer = Color(0xFFCDEFF7)
    val OnSecondaryContainer = Color(0xFF06303F)
    val Tertiary = Color(0xFF1A56DB)
    val OnTertiary = Color(0xFFFFFFFF)
    val TertiaryContainer = Color(0xFFDCE6FF)
    val OnTertiaryContainer = Color(0xFF0B2A6F)

    val Error = Color(0xFFB3261E)
    val OnError = Color(0xFFFFFFFF)
    val ErrorContainer = Color(0xFFF9DEDC)
    val OnErrorContainer = Color(0xFF410E0B)

    val NavContainer = Color(0xFFE8F6F4)
    val NavIndicator = Primary
    val NavSelectedIcon = OnPrimary
    val NavSelectedLabel = Color(0xFF0B5D57)
    val NavUnselected = Color(0xFF3F5552)
}

/**
 * Dark "Energia" brand palette: teal-tinted near-black surfaces, light teal
 * actions, a cyan tonal accent and a deep teal bottom bar. Same contrast
 * targets as [EnergiaLightTokens].
 */
internal object EnergiaDarkTokens {
    val Background = Color(0xFF0E1514)
    val OnBackground = Color(0xFFDDE4E2)
    val Surface = Color(0xFF0E1514)
    val OnSurface = Color(0xFFDDE4E2)
    val SurfaceVariant = Color(0xFF3F4947)
    val OnSurfaceVariant = Color(0xFFBEC9C6)
    val Outline = Color(0xFF899391)
    val OutlineVariant = Color(0xFF3F4947)
    val SurfaceContainerLowest = Color(0xFF090F0E)
    val SurfaceContainerLow = Color(0xFF161D1C)
    val SurfaceContainer = Color(0xFF1A2120)
    val SurfaceContainerHigh = Color(0xFF242B2A)
    val SurfaceContainerHighest = Color(0xFF2F3635)
    val SurfaceDim = Color(0xFF0E1514)
    val SurfaceBright = Color(0xFF343B3A)
    val InverseSurface = Color(0xFFDDE4E2)
    val InverseOnSurface = Color(0xFF2B3231)

    val Primary = Color(0xFF5FD4C6)
    val OnPrimary = Color(0xFF003732)
    val PrimaryContainer = Color(0xFF005049)
    val OnPrimaryContainer = Color(0xFF9EF2E4)
    val Secondary = Color(0xFF67D3EE)
    val OnSecondary = Color(0xFF003543)
    val SecondaryContainer = Color(0xFF004D5F)
    val OnSecondaryContainer = Color(0xFFB8EAF8)
    val Tertiary = Color(0xFFB3C5FF)
    val OnTertiary = Color(0xFF002A77)
    val TertiaryContainer = Color(0xFF1B3F96)
    val OnTertiaryContainer = Color(0xFFDBE1FF)

    val Error = Color(0xFFF2B8B5)
    val OnError = Color(0xFF601410)
    val ErrorContainer = Color(0xFF8C1D18)
    val OnErrorContainer = Color(0xFFF9DEDC)

    val NavContainer = Color(0xFF12302C)
    val NavIndicator = Primary
    val NavSelectedIcon = OnPrimary
    val NavSelectedLabel = OnPrimaryContainer
    val NavUnselected = Color(0xFFA9BAB7)
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
