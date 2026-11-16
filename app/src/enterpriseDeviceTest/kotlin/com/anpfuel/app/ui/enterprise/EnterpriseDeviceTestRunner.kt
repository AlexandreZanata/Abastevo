package com.anpfuel.app.ui.enterprise

import android.app.Application
import android.content.Context
import android.os.Bundle
import androidx.test.runner.AndroidJUnitRunner
import androidx.work.Configuration
import androidx.work.ListenableWorker
import androidx.work.WorkManager
import androidx.work.WorkerFactory
import androidx.work.WorkerParameters
import dagger.hilt.android.testing.HiltTestApplication
import java.util.concurrent.TimeUnit

/** Selected only by the isolated enterpriseDeviceValidation Gradle property. */
class EnterpriseDeviceTestRunner : AndroidJUnitRunner() {
    override fun newApplication(cl: ClassLoader?, className: String?, context: Context?): Application =
        super.newApplication(cl, HiltTestApplication::class.java.name, context)

    /*
     * The validation package keeps one JobScheduler namespace across classes and runs. A class that composes the real
     * UI graph initializes WorkManager and the app enqueues its periodic work; those system jobs outlive the process
     * and, when due, JobScheduler binds SystemJobService into a later class's process, which crashed without an
     * initialized WorkManager ("WorkManager needs to be initialized"). Every process now initializes WorkManager before
     * any service can be created (here, before the application's onCreate and the instrumentation thread) and drops
     * the work of earlier classes before its first test.
     */
    override fun onCreate(arguments: Bundle?) {
        EnterpriseDeviceWork.initialize(targetContext)
        super.onCreate(arguments)
    }

    override fun onStart() {
        EnterpriseDeviceWork.dropEarlierWork(targetContext)
        super.onStart()
    }
}

/** The process-wide WorkManager of the validation package; a class that needs real workers installs its factory. */
object EnterpriseDeviceWork {
    @Volatile private var delegate: WorkerFactory? = null
    private val factory = object : WorkerFactory() {
        override fun createWorker(appContext: Context, workerClassName: String, workerParameters: WorkerParameters): ListenableWorker? =
            delegate?.createWorker(appContext, workerClassName, workerParameters)
    }

    internal fun initialize(context: Context) = WorkManager.initialize(context, Configuration.Builder().setWorkerFactory(factory).build())

    internal fun dropEarlierWork(context: Context) {
        WorkManager.getInstance(context).cancelAllWork().result.get(30, TimeUnit.SECONDS)
    }

    /** The calling test's workers (its own Hilt graph) run this process's jobs from now on. */
    fun use(factory: WorkerFactory) { delegate = factory }
}
