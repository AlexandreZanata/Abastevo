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

internal object ColorTokens {
    val White = Color.White
    val BlueLight = Color(0xFF90CAF9)
    val BlueContainerLight = Color(0xFFBBDEFB)
    val GreenLight = Color(0xFFA5D6A7)
    val GreenContainerLight = Color(0xFFC8E6C9)
    val SurfaceLight = Color(0xFFFDFDFD)
    val SurfaceContainerLowLight = Color(0xFFF7F2FA)
    val SurfaceDark = Color(0xFF1C1B1F)
    val SurfaceContainerLowDark = Color(0xFF1D1B20)
}

object FuelProductTint {

    fun colorFor(product: FuelProduct, darkTheme: Boolean): Color = when (product) {
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
