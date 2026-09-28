package com.anpfuel.app

import android.content.Context
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.core.view.WindowCompat
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.navigation.compose.rememberNavController
import com.anpfuel.app.locale.AppLocaleApplier
import com.anpfuel.app.locale.AppLocaleHolder
import com.anpfuel.app.navigation.AnpAppNavHost
import com.anpfuel.app.ui.theme.AnpFuelTheme
import dagger.hilt.android.AndroidEntryPoint

@AndroidEntryPoint
class MainActivity : ComponentActivity() {

    override fun attachBaseContext(newBase: Context) {
        super.attachBaseContext(AppLocaleApplier.wrap(newBase, AppLocaleHolder.localeTag))
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        WindowCompat.setDecorFitsSystemWindows(window, false)
        setContent {
            val systemDarkTheme = isSystemInDarkTheme()
            var darkThemeOverride by rememberSaveable { mutableStateOf<Boolean?>(null) }
            val darkTheme = darkThemeOverride ?: systemDarkTheme
            val navController = rememberNavController()

            AnpFuelTheme(
                darkTheme = darkTheme,
                dynamicColor = true,
            ) {
                AnpAppNavHost(
                    navController = navController,
                    darkTheme = darkTheme,
                    onToggleTheme = {
                        darkThemeOverride = !(darkThemeOverride ?: systemDarkTheme)
                    },
                )
            }
        }
    }
}
