package com.anpfuel.data.worker

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.*
import com.anpfuel.data.repository.DurableQualityReports
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.concurrent.TimeUnit
import javax.inject.Inject
import javax.inject.Singleton

@HiltWorker
class QualityReportWorker @AssistedInject constructor(
    @Assisted context:Context,@Assisted params:WorkerParameters,
    private val reports:DurableQualityReports,
    private val scheduler:QualityReportWorkScheduler,
):CoroutineWorker(context,params) {
    override suspend fun doWork():Result {
        return try {
            val done=if(inputData.getBoolean("quality_repair_only",false)) {
                val repaired=reports.repairKeys();scheduler.enqueue();repaired
            } else reports.runOnce()
            if(done)Result.success() else Result.retry()
        }catch(e:kotlinx.coroutines.CancellationException){throw e}
        catch(_:Exception){Result.retry()}
    }
}

/** No owner/claim text/auth authority in WorkManager input; only current owner can dispatch. */
@Singleton
class QualityReportWorkScheduler @Inject constructor(@ApplicationContext private val context:Context) {
    fun enqueue() {
        val request=OneTimeWorkRequestBuilder<QualityReportWorker>()
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL,30,TimeUnit.SECONDS).build()
        WorkManager.getInstance(context).enqueueUniqueWork("quality-reports-dispatch",ExistingWorkPolicy.KEEP,request)
    }
    fun maintain() {
        // Repair must also run offline after logout; dispatch will fail closed without live authority.
        val request=PeriodicWorkRequestBuilder<QualityReportWorker>(15,TimeUnit.MINUTES)
            .setInputData(workDataOf("quality_repair_only" to true)).build()
        WorkManager.getInstance(context).enqueueUniquePeriodicWork("quality-reports-maintenance",ExistingPeriodicWorkPolicy.KEEP,request)
        enqueue()
    }
}
