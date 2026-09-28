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
import com.anpfuel.application.usecase.alert.EvaluatePriceDropAlertsResult
import com.anpfuel.application.usecase.alert.EvaluatePriceDropAlertsUseCase
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
class PriceDropEvaluationWorkerTest {

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
        val useCase = mockk<EvaluatePriceDropAlertsUseCase>()
        coEvery { useCase.invoke() } returns EvaluatePriceDropAlertsResult(notificationsShown = 1)

        val result = runWorker(useCase)

        assertEquals(ListenableWorker.Result.success(), result)
        coVerify(exactly = 1) { useCase.invoke() }
    }

    @Test
    fun doWork_returnsRetryWhenUseCaseThrows() = runBlocking {
        val useCase = mockk<EvaluatePriceDropAlertsUseCase>()
        coEvery { useCase.invoke() } throws RuntimeException("boom")

        val result = runWorker(useCase)

        assertEquals(ListenableWorker.Result.retry(), result)
    }

    private fun runWorker(
        useCase: EvaluatePriceDropAlertsUseCase,
    ): ListenableWorker.Result {
        val worker = TestListenableWorkerBuilder<PriceDropEvaluationWorker>(context)
            .setWorkerFactory(
                object : WorkerFactory() {
                    override fun createWorker(
                        appContext: Context,
                        workerClassName: String,
                        workerParameters: WorkerParameters,
                    ): ListenableWorker? {
                        if (workerClassName != PriceDropEvaluationWorker::class.java.name) {
                            return null
                        }
                        return PriceDropEvaluationWorker.createForTest(
                            appContext,
                            workerParameters,
                            useCase,
                        )
                    }
                },
            )
            .build()

        return runBlocking { worker.doWork() }
    }
}
