package com.anpfuel.app.ui.enterprise

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.enterprise.EnterpriseAccessBinding
import com.anpfuel.application.enterprise.EnterpriseAccessError
import com.anpfuel.application.enterprise.EnterpriseAccessResult
import com.anpfuel.application.enterprise.EnterpriseInvitationAcknowledgement
import com.anpfuel.application.enterprise.EnterpriseInvitationFlow
import com.anpfuel.application.enterprise.EnterpriseInvitationGateway
import com.anpfuel.application.enterprise.EnterpriseInvitationGrant
import com.anpfuel.application.enterprise.EnterpriseInvitationPreview
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.application.portable.AuthAccountApi
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthKeyStore
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.portable.ConsumeOk
import com.anpfuel.application.portable.KeyLogin
import com.anpfuel.data.di.AuthModule
import com.anpfuel.data.di.EnterpriseInvitationModule
import com.anpfuel.data.di.NetworkModule
import com.anpfuel.data.enterprise.EnterpriseRevokingAccountApi
import com.anpfuel.data.enterprise.EnterpriseVaultErasure
import com.anpfuel.data.local.auth.SessionKeyProvider
import com.anpfuel.data.remote.AnpFileDownloader
import com.anpfuel.data.remote.AnpListingScraper
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.NominatimClient
import com.anpfuel.domain.portable.PortableAuth
import dagger.Module
import dagger.Provides
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import dagger.hilt.testing.TestInstallIn
import java.io.IOException
import java.time.Clock
import java.util.UUID
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.atomic.AtomicInteger
import javax.inject.Inject
import javax.inject.Named
import javax.inject.Singleton
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient

/** Synthetic outcomes only; no fixture credential can reach an HTTP service. */
@Singleton
class EnterpriseDeviceScenario @Inject constructor() {
    val connected = MutableStateFlow(true)
    val previews = AtomicInteger()
    val accepts = AtomicInteger()
    val declines = AtomicInteger()
    val logins = AtomicInteger()
    val operations = CopyOnWriteArrayList<String>()
    @Volatile var requireFresh = false
    @Volatile var unavailable = false
    @Volatile var invalidCode = false
    @Volatile var pendingConfirmation = true
    @Volatile var familyId = FAMILY
    @Volatile var previewHold: CompletableDeferred<Unit>? = null
    @Volatile var decisionHold: CompletableDeferred<Unit>? = null

    companion object {
        const val ACCOUNT = "7b0e2a54-0000-4000-8000-000000000001"
        const val FAMILY = "7b0e2a54-0000-4000-8000-000000000002"
        const val ORGANIZATION = "7b0e2a54-0000-4000-8000-000000000003"
        const val INVITATION = "7b0e2a54-0000-4000-8000-000000000004"
        const val MEMBERSHIP = "7b0e2a54-0000-4000-8000-000000000005"
        const val ACCOUNT_KEY = "SYNTHETIC-ENTERPRISE-VALIDATION-ACCOUNT-KEY"
        const val CODE = "AIN1.7b0e2a54-0000-4000-8000-000000000004.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
        const val COMPANY_NAME = "Empresa sintética do teste"
    }
}

@Singleton
class EnterpriseDeviceAccountApi @Inject constructor(
    private val scenario: EnterpriseDeviceScenario,
) : AuthAccountApi {
    private fun session() = PortableAuth.Session(
        scenario.familyId, EnterpriseDeviceScenario.ACCOUNT,
        "synthetic-enterprise-access", "synthetic-enterprise-refresh",
        System.currentTimeMillis() / 1000L + 900L,
        System.currentTimeMillis() / 1000L + 86_400L,
    )

    override fun loginWithKey(accountKey: String): AuthApiResult<KeyLogin> {
        val count = scenario.logins.incrementAndGet()
        if (accountKey != EnterpriseDeviceScenario.ACCOUNT_KEY) return AuthApiResult.Err("key-invalid")
        scenario.familyId = "7b0e2a54-0000-4000-8000-" + (1000 + count).toString().padStart(12, '0')
        scenario.requireFresh = false
        return AuthApiResult.Ok(KeyLogin(session(), "synthetic_enterprise"))
    }

    override fun refresh(familyId: String, refreshToken: String): AuthApiResult<PortableAuth.Session> =
        if (familyId == scenario.familyId && refreshToken == "synthetic-enterprise-refresh") {
            AuthApiResult.Ok(session())
        } else AuthApiResult.Err(PortableAuth.Verdict.SESSION_REVOKED)

    override fun requestCode(email: String): AuthApiResult<Unit> = AuthApiResult.Err(PortableAuth.UNAVAILABLE)
    override fun consumeCode(email: String, code: String): AuthApiResult<ConsumeOk> = AuthApiResult.Err(PortableAuth.UNAVAILABLE)
    override fun createKeyAccount(username: String): AuthApiResult<AuthFlow.KeyIssued> = AuthApiResult.Err(PortableAuth.UNAVAILABLE)
    override fun revokeAll(familyId: String, accessToken: String): AuthApiResult<Unit> = AuthApiResult.Ok(Unit)
    override fun linkProvider(session: PortableAuth.Session, provider: String, idToken: String, nonce: String): AuthApiResult<Unit> = AuthApiResult.Err(PortableAuth.UNAVAILABLE)
    override fun deleteAccount(session: PortableAuth.Session): AuthApiResult<Unit> = AuthApiResult.Err(PortableAuth.UNAVAILABLE)
}

@Singleton
class EnterpriseDeviceInvitationGateway @Inject constructor(
    private val sessions: AuthSessionStore,
    private val fence: EnterpriseSessionFence,
    private val scenario: EnterpriseDeviceScenario,
    @Named("apiOrigin") private val environment: ApiEnvironment,
) : EnterpriseInvitationGateway {
    override fun currentBinding(): EnterpriseAccessBinding? = sessions.load()?.let {
        EnterpriseAccessBinding(it.accountId, environment.origin, it.familyId, fence.epoch)
    }

    override suspend fun open(): EnterpriseAccessResult<EnterpriseAccessBinding> {
        val binding = currentBinding() ?: return EnterpriseAccessResult.Err(EnterpriseAccessError.SIGN_IN_REQUIRED)
        return if (scenario.requireFresh) EnterpriseAccessResult.Err(EnterpriseAccessError.FRESH_AUTH_REQUIRED)
        else EnterpriseAccessResult.Ok(binding)
    }

    override suspend fun preview(binding: EnterpriseAccessBinding, code: String): EnterpriseAccessResult<EnterpriseInvitationPreview> {
        scenario.previews.incrementAndGet()
        scenario.previewHold?.let { hold -> withContext(NonCancellable) { hold.await() } }
        if (scenario.unavailable) return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
        if (scenario.invalidCode || code != EnterpriseDeviceScenario.CODE) return EnterpriseAccessResult.Err(EnterpriseAccessError.INVALID_CODE)
        return EnterpriseAccessResult.Ok(EnterpriseInvitationPreview(
            EnterpriseDeviceScenario.COMPANY_NAME, false,
            listOf(EnterpriseInvitationGrant("DRIVER", "SELF")),
            scenario.pendingConfirmation, "a".repeat(64),
        ))
    }

    override suspend fun accept(binding: EnterpriseAccessBinding, operationId: String, code: String, consentDigest: String): EnterpriseAccessResult<EnterpriseInvitationAcknowledgement> {
        scenario.accepts.incrementAndGet()
        scenario.operations += operationId
        scenario.decisionHold?.let { hold -> withContext(NonCancellable) { hold.await() } }
        if (scenario.unavailable) return EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE)
        return EnterpriseAccessResult.Ok(acknowledgement("ACCEPTED", if (scenario.pendingConfirmation) "INVITED" else "ACTIVE"))
    }

    override suspend fun decline(binding: EnterpriseAccessBinding, operationId: String, code: String): EnterpriseAccessResult<EnterpriseInvitationAcknowledgement> {
        scenario.declines.incrementAndGet()
        scenario.operations += operationId
        return EnterpriseAccessResult.Ok(acknowledgement("DECLINED", "ENDED"))
    }

    private fun acknowledgement(state: String, membershipState: String) = EnterpriseInvitationAcknowledgement(
        EnterpriseDeviceScenario.ORGANIZATION, EnterpriseDeviceScenario.INVITATION,
        EnterpriseDeviceScenario.MEMBERSHIP, state, membershipState, false,
    )

    override fun invalidate() = Unit
}

@Module
@TestInstallIn(components = [SingletonComponent::class], replaces = [EnterpriseInvitationModule::class])
object EnterpriseDeviceInvitationModule {
    @Provides @Singleton
    fun gateway(gateway: EnterpriseDeviceInvitationGateway): EnterpriseInvitationGateway = gateway

    @Provides
    fun flow(gateway: EnterpriseInvitationGateway): EnterpriseInvitationFlow =
        EnterpriseInvitationFlow(gateway) { UUID.randomUUID().toString() }
}

@Module
@TestInstallIn(components = [SingletonComponent::class], replaces = [EnterprisePresentationModule::class])
object EnterpriseDevicePresentationModule {
    @Provides @Named("enterpriseConnectivity")
    fun connectivity(scenario: EnterpriseDeviceScenario): NetworkConnectivityGateway =
        object : NetworkConnectivityGateway {
            override fun observeIsConnected() = scenario.connected
        }
}

/** Keep the production Keystore/session/fence composition and substitute only HTTP outcomes. */
@Module
@TestInstallIn(components = [SingletonComponent::class], replaces = [AuthModule::class])
object EnterpriseDeviceAuthModule {
    @Provides @Singleton @Named("auth")
    fun preferences(@ApplicationContext context: Context, @Named("apiOrigin") environment: ApiEnvironment): SharedPreferences =
        AuthModule.provideAuthPrefs(context, environment)

    @Provides @Singleton
    fun keys(): SessionKeyProvider = AuthModule.provideSessionKeys()

    @Provides @Singleton
    fun sessions(@Named("auth") preferences: SharedPreferences, keys: SessionKeyProvider, fence: EnterpriseSessionFence): AuthSessionStore =
        AuthModule.provideSessionStore(preferences, keys, fence)

    @Provides @Singleton
    fun accountKey(@Named("auth") preferences: SharedPreferences, keys: SessionKeyProvider): AuthKeyStore =
        AuthModule.provideAccountKeyStore(preferences, keys)

    @Provides @Singleton
    fun accountApi(api: EnterpriseDeviceAccountApi, sessions: AuthSessionStore, @Named("apiOrigin") environment: ApiEnvironment, erasure: EnterpriseVaultErasure): AuthAccountApi =
        EnterpriseRevokingAccountApi(api, sessions, environment.origin, erasure)

    @Provides @Singleton
    fun auth(sessions: AuthSessionStore, keys: AuthKeyStore, api: AuthAccountApi): AuthFlow =
        AuthModule.provideAuthFlow(sessions, keys, api)
}

/** Unrelated graph initialization must never send synthetic proof to staging. */
@Module
@TestInstallIn(components = [SingletonComponent::class], replaces = [NetworkModule::class])
object EnterpriseDeviceNetworkModule {
    private fun blocked(): OkHttpClient = OkHttpClient.Builder().addInterceptor {
        throw IOException("Enterprise validation network disabled")
    }.build()

    @Provides @Singleton
    fun client(): OkHttpClient = blocked()

    @Provides @Singleton @Named("apiOrigin")
    fun environment(): ApiEnvironment = ApiEnvironment.PREVIEW

    @Provides @Singleton
    fun scraper(client: OkHttpClient): AnpListingScraper = NetworkModule.provideAnpListingScraper(client)

    @Provides @Singleton
    fun downloader(@ApplicationContext context: Context, client: OkHttpClient): AnpFileDownloader =
        NetworkModule.provideAnpFileDownloader(context, client)

    @Provides @Singleton
    fun clock(): Clock = NetworkModule.provideClock()

    @Provides @Singleton @NominatimClient
    fun geocoder(): OkHttpClient = blocked()
}
