package com.anpfuel.app.ui.enterprise

import com.anpfuel.application.enterprise.EnterpriseAccessBinding
import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseAccessResult
import com.anpfuel.application.enterprise.EnterpriseContextFlow
import com.anpfuel.application.enterprise.EnterpriseContextGateway
import com.anpfuel.application.enterprise.EnterpriseContextPage
import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.EnterpriseLocalContext
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.di.EnterpriseContextModule
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.domain.enterprise.EnterprisePartition
import dagger.Module
import dagger.Provides
import dagger.hilt.components.SingletonComponent
import dagger.hilt.testing.TestInstallIn
import java.util.concurrent.atomic.AtomicInteger
import javax.inject.Inject
import javax.inject.Named
import javax.inject.Singleton
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.withContext

/** Current source outcomes only; no persistence, permission grant or vault unlock. */
@Singleton
class EnterpriseContextDeviceScenario @Inject constructor() {
    val lists = AtomicInteger()
    val reads = AtomicInteger()
    val completedReads = AtomicInteger()
    @Volatile var denied = false
    @Volatile var unavailable = false
    @Volatile var paginated = false
    @Volatile var listHold: CompletableDeferred<Unit>? = null
    @Volatile var readHold: CompletableDeferred<Unit>? = null

    companion object {
        const val ORGANIZATION_A = "7b0e2a54-0000-4000-8000-000000000003"
        const val ORGANIZATION_B = "7b0e2a54-0000-4000-8000-000000000006"
        const val MEMBERSHIP_A = "7b0e2a54-0000-4000-8000-000000000005"
        const val MEMBERSHIP_B = "7b0e2a54-0000-4000-8000-000000000007"
        const val COMPANY_A = "Empresa sintética Alfa"
        const val COMPANY_B = "Empresa sintética Beta"
    }
}

class EnterpriseContextDeviceGateway @Inject constructor(
    private val sessions: AuthSessionStore,
    private val fence: EnterpriseSessionFence,
    private val scenario: EnterpriseContextDeviceScenario,
    @Named("apiOrigin") private val environment: ApiEnvironment,
) : EnterpriseContextGateway {
    override fun currentBinding(): EnterpriseAccessBinding? = sessions.load()?.let {
        EnterpriseAccessBinding(it.accountId, environment.origin, it.familyId, fence.epoch)
    }

    override suspend fun open(): EnterpriseAccessResult<EnterpriseAccessBinding> {
        val binding = currentBinding() ?: return EnterpriseAccessResult.Err(EnterpriseAccessError.SIGN_IN_REQUIRED)
        return if (scenario.denied) EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED)
        else EnterpriseAccessResult.Ok(binding)
    }

    private fun contexts(binding: EnterpriseAccessBinding) = listOf(
        EnterpriseContextSummary(
            EnterprisePartition.of(binding.accountId, binding.origin, EnterpriseContextDeviceScenario.ORGANIZATION_A),
            EnterpriseContextDeviceScenario.MEMBERSHIP_A, EnterpriseContextDeviceScenario.COMPANY_A,
        ),
        EnterpriseContextSummary(
            EnterprisePartition.of(binding.accountId, binding.origin, EnterpriseContextDeviceScenario.ORGANIZATION_B),
            EnterpriseContextDeviceScenario.MEMBERSHIP_B, EnterpriseContextDeviceScenario.COMPANY_B,
        ),
    )

    override suspend fun list(binding: EnterpriseAccessBinding, afterOrganizationId: String?): EnterpriseAccessResult<EnterpriseContextPage> {
        scenario.lists.incrementAndGet()
        scenario.listHold?.let { withContext(NonCancellable) { it.await() } }
        if (scenario.denied) return EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED)
        if (scenario.unavailable) return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
        val values = contexts(binding)
        val page = when {
            !scenario.paginated -> EnterpriseContextPage(values)
            afterOrganizationId == null -> EnterpriseContextPage(listOf(values.first()), EnterpriseContextDeviceScenario.ORGANIZATION_A)
            afterOrganizationId == EnterpriseContextDeviceScenario.ORGANIZATION_A -> EnterpriseContextPage(listOf(values.last()))
            else -> error("Unexpected synthetic context cursor")
        }
        return EnterpriseAccessResult.Ok(page)
    }

    override suspend fun read(binding: EnterpriseAccessBinding, context: EnterpriseContextSummary): EnterpriseAccessResult<EnterpriseContextSummary> {
        scenario.reads.incrementAndGet()
        scenario.readHold?.let { withContext(NonCancellable) { it.await() } }
        scenario.completedReads.incrementAndGet()
        if (scenario.denied) return EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED)
        if (scenario.unavailable) return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
        return contexts(binding).firstOrNull { it.partition == context.partition && it.membershipId == context.membershipId }
            ?.let { EnterpriseAccessResult.Ok(it) }
            ?: EnterpriseAccessResult.Err(EnterpriseAccessError.ACCESS_DENIED)
    }

    override fun invalidate() = Unit
}

@Module
@TestInstallIn(components = [SingletonComponent::class], replaces = [EnterpriseContextModule::class])
object EnterpriseContextDeviceModule {
    @Provides
    fun gateway(gateway: EnterpriseContextDeviceGateway): EnterpriseContextGateway = gateway

    @Provides
    fun flow(gateway: EnterpriseContextGateway, local: EnterpriseLocalContext): EnterpriseContextFlow =
        EnterpriseContextFlow(gateway, local)
}
