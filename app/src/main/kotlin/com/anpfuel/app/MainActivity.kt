package com.anpfuel.app

import android.content.Context
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.core.view.WindowCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import androidx.navigation.compose.rememberNavController
import com.anpfuel.app.capture.PrivateCaptureFiles
import com.anpfuel.app.locale.AppLocaleApplier
import com.anpfuel.app.locale.AppLocaleHolder
import com.anpfuel.app.navigation.AnpAppNavHost
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.application.portable.AuthFlow
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

@AndroidEntryPoint
class MainActivity : ComponentActivity() {

    @Inject lateinit var outbox: com.anpfuel.domain.repository.ContributionOutboxRepository
    @Inject lateinit var contributionScope: com.anpfuel.application.port.ContributionScopeProvider
    @Inject lateinit var contributionScheduler: com.anpfuel.data.worker.ContributionWorkScheduler
    @Inject lateinit var authFlow: AuthFlow
    @Inject lateinit var photoFlow: com.anpfuel.application.portable.PhotoFlow

    override fun onDestroy() {
        if (!isChangingConfigurations) com.anpfuel.app.ui.auth.AccountKeyUnlockSession.grant.reset()
        super.onDestroy()
    }

    override fun attachBaseContext(newBase: Context) {
        super.attachBaseContext(AppLocaleApplier.wrap(newBase, AppLocaleHolder.localeTag))
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Resume and maintain short-lived tokens while the app is visible.
        // AuthFlow serializes rotations with logout; failures retain no live grant.
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                withContext(Dispatchers.IO) { PrivateCaptureFiles(applicationContext).sweep(); photoFlow.sweepExpired() }
                withContext(Dispatchers.IO) {
                    runCatching {
                        val scope = contributionScope.currentScope()
                        outbox.listOwned().filter { it.scope == scope &&
                            it.phase != com.anpfuel.domain.repository.OwnedContributionPhase.CANCELLED &&
                            it.remoteStatus !in setOf(com.anpfuel.domain.repository.ContributionRemoteStatus.VALIDATED,
                                com.anpfuel.domain.repository.ContributionRemoteStatus.REJECTED,
                                com.anpfuel.domain.repository.ContributionRemoteStatus.EXPIRED) }
                            .forEach { contributionScheduler.enqueueContribution(it.commandId) }
                    }
                }
                while (true) {
                    withContext(Dispatchers.IO) { authFlow.refreshSession() }
                    delay(30_000L)
                }
            }
        }
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
