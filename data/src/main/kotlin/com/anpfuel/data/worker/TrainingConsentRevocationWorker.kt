package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.*
import com.anpfuel.application.port.TrainingConsentLocalStore
import com.anpfuel.application.usecase.privacy.TrainingConsentUseCase
import com.anpfuel.data.local.preferences.TrainingConsentOwner
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.withTimeout
import java.util.concurrent.TimeUnit

/** Bounded wakeup, only OFF, no historical-owner scan and no automatic grant. */
@HiltWorker
class TrainingConsentRevocationWorker @AssistedInject constructor(
    @Assisted context:Context,@Assisted params:WorkerParameters,
    private val owner:TrainingConsentOwner,private val local:TrainingConsentLocalStore,
    private val useCase:TrainingConsentUseCase,
):CoroutineWorker(context,params) {
    override suspend fun doWork():Result = try {
        withTimeout(90000) {
            val caller=owner.current()
            com.anpfuel.application.usecase.privacy.ResumeTrainingRevocation(local,useCase)(caller)
        }
        Result.success()
    } catch(e:CancellationException) { throw e }
    catch(_:Exception) { Result.failure() } // Journal remains; next periodic/foreground attempt may retry safely.

    companion object {
        fun schedule(context:Context) {
            val constraints=Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build()
            WorkManager.getInstance(context).enqueueUniquePeriodicWork("training-revocation-periodic",
                ExistingPeriodicWorkPolicy.KEEP,
                PeriodicWorkRequestBuilder<TrainingConsentRevocationWorker>(15,TimeUnit.MINUTES).setConstraints(constraints).build())
            WorkManager.getInstance(context).enqueueUniqueWork("training-revocation-startup",ExistingWorkPolicy.KEEP,
                OneTimeWorkRequestBuilder<TrainingConsentRevocationWorker>().setConstraints(constraints).build())
        }
    }
}
