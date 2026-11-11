package com.anpfuel.app.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.platform.LocalContext

val LocalAnpDarkTheme = staticCompositionLocalOf { false }

/** Chrome brighter than this needs dark system bar icons. */
private const val LIGHT_BAR_MIN_LUMINANCE = 0.5f

internal val LightColorScheme = lightColorScheme(
    background = EnergiaLightTokens.Background,
    onBackground = EnergiaLightTokens.OnBackground,
    surface = EnergiaLightTokens.Surface,
    onSurface = EnergiaLightTokens.OnSurface,
    surfaceVariant = EnergiaLightTokens.SurfaceVariant,
    onSurfaceVariant = EnergiaLightTokens.OnSurfaceVariant,
    outline = EnergiaLightTokens.Outline,
    outlineVariant = EnergiaLightTokens.OutlineVariant,
    surfaceContainerLowest = EnergiaLightTokens.SurfaceContainerLowest,
    surfaceContainerLow = EnergiaLightTokens.SurfaceContainerLow,
    surfaceContainer = EnergiaLightTokens.SurfaceContainer,
    surfaceContainerHigh = EnergiaLightTokens.SurfaceContainerHigh,
    surfaceContainerHighest = EnergiaLightTokens.SurfaceContainerHighest,
    surfaceDim = EnergiaLightTokens.SurfaceDim,
    surfaceBright = EnergiaLightTokens.SurfaceBright,
    inverseSurface = EnergiaLightTokens.InverseSurface,
    inverseOnSurface = EnergiaLightTokens.InverseOnSurface,
    primary = EnergiaLightTokens.Primary,
    onPrimary = EnergiaLightTokens.OnPrimary,
    primaryContainer = EnergiaLightTokens.PrimaryContainer,
    onPrimaryContainer = EnergiaLightTokens.OnPrimaryContainer,
    secondary = AbastevoAccent.Light.accent,
    onSecondary = AbastevoAccent.Light.onAccent,
    secondaryContainer = AbastevoAccent.Light.container,
    onSecondaryContainer = AbastevoAccent.Light.onContainer,
    tertiary = EnergiaLightTokens.Tertiary,
    onTertiary = EnergiaLightTokens.OnTertiary,
    tertiaryContainer = EnergiaLightTokens.TertiaryContainer,
    onTertiaryContainer = EnergiaLightTokens.OnTertiaryContainer,
    error = EnergiaLightTokens.Error,
    onError = EnergiaLightTokens.OnError,
    errorContainer = EnergiaLightTokens.ErrorContainer,
    onErrorContainer = EnergiaLightTokens.OnErrorContainer,
)

internal val DarkColorScheme = darkColorScheme(
    background = EnergiaDarkTokens.Background,
    onBackground = EnergiaDarkTokens.OnBackground,
    surface = EnergiaDarkTokens.Surface,
    onSurface = EnergiaDarkTokens.OnSurface,
    surfaceVariant = EnergiaDarkTokens.SurfaceVariant,
    onSurfaceVariant = EnergiaDarkTokens.OnSurfaceVariant,
    outline = EnergiaDarkTokens.Outline,
    outlineVariant = EnergiaDarkTokens.OutlineVariant,
    surfaceContainerLowest = EnergiaDarkTokens.SurfaceContainerLowest,
    surfaceContainerLow = EnergiaDarkTokens.SurfaceContainerLow,
    surfaceContainer = EnergiaDarkTokens.SurfaceContainer,
    surfaceContainerHigh = EnergiaDarkTokens.SurfaceContainerHigh,
    surfaceContainerHighest = EnergiaDarkTokens.SurfaceContainerHighest,
    surfaceDim = EnergiaDarkTokens.SurfaceDim,
    surfaceBright = EnergiaDarkTokens.SurfaceBright,
    inverseSurface = EnergiaDarkTokens.InverseSurface,
    inverseOnSurface = EnergiaDarkTokens.InverseOnSurface,
    primary = EnergiaDarkTokens.Primary,
    onPrimary = EnergiaDarkTokens.OnPrimary,
    primaryContainer = EnergiaDarkTokens.PrimaryContainer,
    onPrimaryContainer = EnergiaDarkTokens.OnPrimaryContainer,
    secondary = AbastevoAccent.Dark.accent,
    onSecondary = AbastevoAccent.Dark.onAccent,
    secondaryContainer = AbastevoAccent.Dark.container,
    onSecondaryContainer = AbastevoAccent.Dark.onContainer,
    tertiary = EnergiaDarkTokens.Tertiary,
    onTertiary = EnergiaDarkTokens.OnTertiary,
    tertiaryContainer = EnergiaDarkTokens.TertiaryContainer,
    onTertiaryContainer = EnergiaDarkTokens.OnTertiaryContainer,
    error = EnergiaDarkTokens.Error,
    onError = EnergiaDarkTokens.OnError,
    errorContainer = EnergiaDarkTokens.ErrorContainer,
    onErrorContainer = EnergiaDarkTokens.OnErrorContainer,
)

@Composable
fun AnpFuelTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    dynamicColor: Boolean = false,
    content: @Composable () -> Unit,
) {
    val useDynamicColor = dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S
    val colorScheme = when {
        useDynamicColor -> {
            val context = LocalContext.current
            if (darkTheme) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        }
        darkTheme -> DarkColorScheme
        else -> LightColorScheme
    }
    val chromeColors = when {
        useDynamicColor -> AbastevoChromeDefaults.fromScheme(colorScheme)
        darkTheme -> AbastevoChromeDefaults.Dark
        else -> AbastevoChromeDefaults.Light
    }
    val navColors = AbastevoNavDefaults.fromChrome(chromeColors)
    val actionColors = when {
        useDynamicColor -> AbastevoActionDefaults.fromScheme(colorScheme)
        darkTheme -> AbastevoActionDefaults.Dark
        else -> AbastevoActionDefaults.Light
    }

    MaterialTheme(
        colorScheme = colorScheme,
        typography = Typography,
        shapes = AbastevoShapes,
        content = {
            CompositionLocalProvider(
                LocalAnpDarkTheme provides darkTheme,
                LocalAbastevoChromeColors provides chromeColors,
                LocalAbastevoNavColors provides navColors,
                LocalAbastevoActionColors provides actionColors,
            ) {
                val lightChrome = chromeColors.container.luminance() > LIGHT_BAR_MIN_LUMINANCE
                AnpSystemBarsEffect(
                    lightStatusBar = lightChrome,
                    lightNavigationBar = navColors.container.luminance() > LIGHT_BAR_MIN_LUMINANCE,
                )
                content()
            }
        },
    )
}
