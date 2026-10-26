package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.*
import com.anpfuel.application.usecase.charging.ChargingCommunityUseCase
import com.anpfuel.application.usecase.charging.ChargingSaveOutcome
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton
import java.util.concurrent.TimeUnit

/** Durable replay only; no credentials or report bodies in WorkManager inputs. */
@HiltWorker
class ChargingObservationWorker @AssistedInject constructor(
    @Assisted context: Context, @Assisted params: WorkerParameters,
    private val community: ChargingCommunityUseCase,
) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val owner = inputData.getString("owner") ?: return Result.failure()
        return try {
            when (community.retryPending(owner)) {
                is ChargingSaveOutcome.Offline -> if (runAttemptCount < 25) Result.retry() else Result.failure()
                is ChargingSaveOutcome.Refused, is ChargingSaveOutcome.Conflict -> Result.failure()
                else -> Result.success()
            }
        } catch (cancelled: kotlinx.coroutines.CancellationException) { throw cancelled
        } catch (_: Exception) { if (runAttemptCount < 25) Result.retry() else Result.failure() }
    }
}

@Singleton
class ChargingObservationScheduler @Inject constructor(@ApplicationContext private val context: Context) {
    fun enqueue(owner: String) {
        val request = OneTimeWorkRequestBuilder<ChargingObservationWorker>()
            .setInputData(workDataOf("owner" to owner))
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS).build()
        WorkManager.getInstance(context).enqueueUniqueWork("charging-observations-$owner", ExistingWorkPolicy.APPEND_OR_REPLACE, request)
    }
}
