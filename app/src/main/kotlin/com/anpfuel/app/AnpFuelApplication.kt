package com.anpfuel.app

import android.app.Application
import androidx.hilt.work.HiltWorkerFactory
import androidx.work.Configuration
import com.anpfuel.app.locale.AppLocaleHolder
import com.anpfuel.app.locale.AppLocales
import com.anpfuel.data.worker.SyncWorkScheduler
import com.anpfuel.domain.repository.UserPreferencesRepository
import dagger.hilt.android.HiltAndroidApp
import javax.inject.Inject
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking

@HiltAndroidApp
class AnpFuelApplication : Application(), Configuration.Provider {

    @Inject
    lateinit var workerFactory: HiltWorkerFactory

    @Inject
    lateinit var syncWorkScheduler: SyncWorkScheduler

    @Inject
    lateinit var userPreferencesRepository: UserPreferencesRepository

    private val applicationScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    override fun onCreate() {
        super.onCreate()
        runBlocking(Dispatchers.IO) {
            val preferences = userPreferencesRepository.getPreferences()
            val resolvedTag = if (!preferences.localeUserSelected) {
                val deviceTag = AppLocales.resolveDeviceLocaleTag(this@AnpFuelApplication)
                userPreferencesRepository.savePreferences(
                    preferences.copy(
                        localeTag = deviceTag,
                        localeUserSelected = true,
                    ),
                )
                deviceTag
            } else {
                AppLocales.normalizeLocaleTag(preferences.localeTag)
            }
            AppLocaleHolder.localeTag = resolvedTag
        }
        applicationScope.launch {
            syncWorkScheduler.schedulePeriodicSync()
        }
    }

    override val workManagerConfiguration: Configuration
        get() = Configuration.Builder()
            .setWorkerFactory(workerFactory)
            .build()
}
