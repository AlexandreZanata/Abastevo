package com.anpfuel.app.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalContext

private val LightColorScheme = lightColorScheme(
    primary = AnpBlue,
    onPrimary = ColorTokens.White,
    primaryContainer = ColorTokens.BlueContainerLight,
    onPrimaryContainer = AnpBlueDark,
    secondary = AnpGreen,
    onSecondary = ColorTokens.White,
    secondaryContainer = ColorTokens.GreenContainerLight,
    onSecondaryContainer = AnpGreenDark,
)

private val DarkColorScheme = darkColorScheme(
    primary = ColorTokens.BlueLight,
    onPrimary = AnpBlueDark,
    primaryContainer = AnpBlueDark,
    onPrimaryContainer = ColorTokens.BlueContainerLight,
    secondary = ColorTokens.GreenLight,
    onSecondary = AnpGreenDark,
    secondaryContainer = AnpGreenDark,
    onSecondaryContainer = ColorTokens.GreenContainerLight,
)

@Composable
fun AnpFuelTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    dynamicColor: Boolean = true,
    content: @Composable () -> Unit,
) {
    val colorScheme = when {
        dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> {
            val context = LocalContext.current
            if (darkTheme) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        }
        darkTheme -> DarkColorScheme
        else -> LightColorScheme
    }

    MaterialTheme(
        colorScheme = colorScheme,
        typography = Typography,
        content = {
            AnpSystemBarsEffect(darkTheme = darkTheme)
            content()
        },
    )
}
