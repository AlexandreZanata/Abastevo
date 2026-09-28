package com.anpfuel.data.worker

import android.content.Context
import android.util.Log
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.Configuration
import androidx.work.ListenableWorker
import androidx.work.WorkerFactory
import androidx.work.WorkerParameters
import androidx.work.testing.SynchronousExecutor
import androidx.work.testing.TestListenableWorkerBuilder
import androidx.work.testing.WorkManagerTestInitHelper
import androidx.work.workDataOf
import com.anpfuel.application.usecase.sync.AutoDownloadLatestWeekOutcome
import com.anpfuel.application.usecase.sync.AutoDownloadLatestWeekUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesResult
import com.anpfuel.application.usecase.sync.SyncPriceTablesUseCase
import com.anpfuel.domain.event.SyncJobOutcome
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.exception.DomainException
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class SyncWorkerTest {

    private lateinit var context: Context

    @Before
    fun setUp() {
        context = ApplicationProvider.getApplicationContext()
        val config = Configuration.Builder()
            .setMinimumLoggingLevel(Log.DEBUG)
            .setExecutor(SynchronousExecutor())
            .build()
        WorkManagerTestInitHelper.initializeTestWorkManager(context, config)
    }

    @After
    fun tearDown() {
        WorkManagerTestInitHelper.closeWorkDatabase()
    }

    @Test
    fun doWork_returnsSuccessWhenUseCaseSucceeds() = runBlocking {
        val useCase = mockk<SyncPriceTablesUseCase>()
        val autoDownloadLatestWeekUseCase = mockk<AutoDownloadLatestWeekUseCase>()
        coEvery { autoDownloadLatestWeekUseCase(SyncRequestSource.SCHEDULED) } returns
            AutoDownloadLatestWeekOutcome.Disabled
        coEvery { useCase(SyncRequestSource.SCHEDULED) } returns SyncPriceTablesResult(
            outcome = SyncJobOutcome.SUCCESS,
        )

        val result = runWorker(
            useCase = useCase,
            autoDownloadLatestWeekUseCase = autoDownloadLatestWeekUseCase,
            inputData = workDataOf(SyncWorker.KEY_SOURCE to SyncWorker.SOURCE_SCHEDULED),
        )

        assertEquals(ListenableWorker.Result.success(), result)
        coVerify(exactly = 1) { useCase(SyncRequestSource.SCHEDULED) }
    }

    @Test
    fun doWork_returnsFailureWhenUseCaseFails() = runBlocking {
        val useCase = mockk<SyncPriceTablesUseCase>()
        val autoDownloadLatestWeekUseCase = mockk<AutoDownloadLatestWeekUseCase>()
        coEvery { autoDownloadLatestWeekUseCase(SyncRequestSource.MANUAL) } returns
            AutoDownloadLatestWeekOutcome.Disabled
        coEvery { useCase(SyncRequestSource.MANUAL) } returns SyncPriceTablesResult(
            outcome = SyncJobOutcome.FAILED,
        )

        val result = runWorker(
            useCase = useCase,
            autoDownloadLatestWeekUseCase = autoDownloadLatestWeekUseCase,
            inputData = workDataOf(SyncWorker.KEY_SOURCE to SyncWorker.SOURCE_MANUAL),
        )

        assertEquals(ListenableWorker.Result.failure(), result)
        coVerify(exactly = 1) { useCase(SyncRequestSource.MANUAL) }
    }

    @Test
    fun doWork_returnsFailureWhenConcurrencyRuleRejectsSync() = runBlocking {
        val useCase = mockk<SyncPriceTablesUseCase>()
        val autoDownloadLatestWeekUseCase = mockk<AutoDownloadLatestWeekUseCase>()
        coEvery { autoDownloadLatestWeekUseCase(any()) } returns AutoDownloadLatestWeekOutcome.Disabled
        coEvery { useCase(any()) } throws DomainException("Sync already in progress")

        val result = runWorker(
            useCase = useCase,
            autoDownloadLatestWeekUseCase = autoDownloadLatestWeekUseCase,
        )

        assertEquals(ListenableWorker.Result.failure(), result)
    }

    private fun runWorker(
        useCase: SyncPriceTablesUseCase,
        autoDownloadLatestWeekUseCase: AutoDownloadLatestWeekUseCase,
        inputData: androidx.work.Data = workDataOf(
            SyncWorker.KEY_SOURCE to SyncWorker.SOURCE_SCHEDULED,
        ),
    ): ListenableWorker.Result {
        val worker = TestListenableWorkerBuilder<SyncWorker>(context)
            .setWorkerFactory(
                object : WorkerFactory() {
                    override fun createWorker(
                        appContext: Context,
                        workerClassName: String,
                        workerParameters: WorkerParameters,
                    ): ListenableWorker? {
                        if (workerClassName != SyncWorker::class.java.name) {
                            return null
                        }
                        return SyncWorker.createForTest(
                            appContext,
                            workerParameters,
                            useCase,
                            autoDownloadLatestWeekUseCase,
                        )
                    }
                },
            )
            .setInputData(inputData)
            .build()

        return runBlocking { worker.doWork() }
    }
}
