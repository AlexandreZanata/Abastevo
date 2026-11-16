package com.anpfuel.app.ui.enterprise

import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseAccessResult
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.checklist.EnterpriseChecklistGateway
import com.anpfuel.data.di.EnterpriseChecklistModule
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistApplicable
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistContext
import com.anpfuel.domain.enterprise.maintenance.checklist.ChecklistServerRun
import dagger.Module
import dagger.Provides
import dagger.hilt.components.SingletonComponent
import dagger.hilt.testing.TestInstallIn

/** The validation graph has no company cookie client: checklist company reads are unavailable, local work stays usable. */
@Module
@TestInstallIn(components = [SingletonComponent::class], replaces = [EnterpriseChecklistModule::class])
object EnterpriseChecklistDeviceModule {
    @Provides
    fun gateway(): EnterpriseChecklistGateway = object : EnterpriseChecklistGateway {
        override suspend fun applicable(context: EnterpriseContextSummary, assetId: String, checklistContext: ChecklistContext):
            EnterpriseAccessResult<ChecklistApplicable> = EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
        override suspend fun run(context: EnterpriseContextSummary, assetId: String, runId: String): EnterpriseAccessResult<ChecklistServerRun> =
            EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
    }
}
