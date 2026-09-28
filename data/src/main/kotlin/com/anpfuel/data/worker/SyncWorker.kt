package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.anpfuel.application.usecase.sync.AutoDownloadLatestWeekOutcome
import com.anpfuel.application.usecase.sync.AutoDownloadLatestWeekUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesUseCase
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.exception.DomainException
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

/**
 * UC-001 background worker — discovers, downloads, and imports ANP price tables.
 */
@HiltWorker
class SyncWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val syncPriceTablesUseCase: SyncPriceTablesUseCase,
    private val autoDownloadLatestWeekUseCase: AutoDownloadLatestWeekUseCase,
) : CoroutineWorker(appContext, params) {

    override suspend fun doWork(): Result {
        val source = resolveSource()
        return try {
            val syncResult = when (val autoOutcome = autoDownloadLatestWeekUseCase(source)) {
                is AutoDownloadLatestWeekOutcome.Disabled -> syncPriceTablesUseCase(source)
                is AutoDownloadLatestWeekOutcome.UpToDate -> {
                    return Result.success()
                }
                is AutoDownloadLatestWeekOutcome.Success -> autoOutcome.syncResult
                is AutoDownloadLatestWeekOutcome.Failed -> {
                    return Result.failure()
                }
            }
            when (syncResult.outcome) {
                SyncJobOutcome.FAILED -> Result.failure()
                SyncJobOutcome.SUCCESS,
                SyncJobOutcome.PARTIAL,
                -> {
                    SyncWorkScheduler.enqueueRetentionCleanup(applicationContext)
                    SyncWorkScheduler.enqueuePriceDropEvaluation(applicationContext)
                    Result.success()
                }
                SyncJobOutcome.NO_NEW_DATA -> Result.success()
            }
        } catch (_: DomainException) {
            Result.failure()
        } catch (_: Exception) {
            Result.retry()
        }
    }

    private fun resolveSource(): SyncRequestSource =
        when (inputData.getString(KEY_SOURCE)) {
            SOURCE_MANUAL -> SyncRequestSource.MANUAL
            SOURCE_FIRST_LAUNCH -> SyncRequestSource.FIRST_LAUNCH
            else -> SyncRequestSource.SCHEDULED
        }

    companion object {
        const val KEY_SOURCE = "sync_source"
        const val SOURCE_SCHEDULED = "scheduled"
        const val SOURCE_MANUAL = "manual"
        const val SOURCE_FIRST_LAUNCH = "first_launch"

        internal fun createForTest(
            context: Context,
            params: WorkerParameters,
            syncPriceTablesUseCase: SyncPriceTablesUseCase,
            autoDownloadLatestWeekUseCase: AutoDownloadLatestWeekUseCase,
        ): SyncWorker = SyncWorker(
            context,
            params,
            syncPriceTablesUseCase,
            autoDownloadLatestWeekUseCase,
        )
    }
}
