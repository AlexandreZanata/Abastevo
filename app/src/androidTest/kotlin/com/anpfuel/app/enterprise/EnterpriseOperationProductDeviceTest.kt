package com.anpfuel.app.enterprise

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.work.Constraints
import androidx.work.Data
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkInfo
import androidx.work.WorkManager
import com.anpfuel.app.AnpFuelApplication
import com.anpfuel.data.di.EnterpriseModule
import com.anpfuel.data.enterprise.AndroidEnterpriseVaultKeys
import com.anpfuel.data.worker.EnterpriseOperationWorker
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** Ordinary application/OS construction, without HiltTestApplication or a test factory.
 * The request has no scope or payload. Existing production jobs and their backoff
 * are preserved; only this fresh opaque acceptance request is cancelled afterward.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseOperationProductDeviceTest {
    @Test
    fun normalHiltFactoryExecutesFreshOpaqueRequest(): Unit = runBlocking {
        val context = InstrumentationRegistry.getInstrumentation().targetContext.applicationContext
        assertTrue("Product acceptance requires the ordinary application", context is AnpFuelApplication)
        assertEquals("Product acceptance requires the isolated debug target", "com.anpfuel.app.debug", context.packageName)
        assertTrue("Preserve existing enterprise data instead of running constructor acceptance",
            EnterpriseModule.vaultFiles(context).allTokens().isEmpty() &&
                AndroidEnterpriseVaultKeys().aliases("abastevo_enterprise_v2_").isEmpty() &&
                AndroidEnterpriseVaultKeys().aliases("abastevo_enterprise_v1_").isEmpty())
        val manager = WorkManager.getInstance(context)
        val request = OneTimeWorkRequestBuilder<EnterpriseOperationWorker>()
            .setInputData(Data.EMPTY)
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .addTag("enterprise-operation-product-acceptance")
            .build()
        try {
            manager.enqueue(request).result.get(10, TimeUnit.SECONDS)
            val observed = withTimeout(30_000) {
                var result: WorkInfo? = null
                while (result == null) {
                    val info = manager.getWorkInfoById(request.id).get(5, TimeUnit.SECONDS)
                    assertTrue("The actual product worker could not be constructed", info?.state != WorkInfo.State.FAILED)
                    if (info != null && info.runAttemptCount > 0 &&
                        info.state in setOf(WorkInfo.State.ENQUEUED, WorkInfo.State.SUCCEEDED)) result = info
                    else delay(100)
                }
                result
            }
            assertTrue(observed.runAttemptCount > 0)
            assertTrue(observed.state in setOf(WorkInfo.State.ENQUEUED, WorkInfo.State.SUCCEEDED))
            assertEquals(WorkInfo.STOP_REASON_NOT_STOPPED, observed.stopReason)
            assertEquals(Data.EMPTY, observed.outputData)
        } finally {
            manager.cancelWorkById(request.id).result.get(10, TimeUnit.SECONDS)
        }
    }
}
