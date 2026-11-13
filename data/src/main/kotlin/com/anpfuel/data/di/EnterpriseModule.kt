package com.anpfuel.data.di

import android.content.Context
import com.anpfuel.application.enterprise.EnterpriseLocalContext
import com.anpfuel.application.enterprise.EnterpriseContactAuthority
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.enterprise.EnterpriseContextProtection
import com.anpfuel.application.enterprise.EnterprisePersistence
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.enterprise.*
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.PhotoProofTransport
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import java.io.File
import java.util.concurrent.TimeUnit
import javax.inject.Named
import javax.inject.Singleton
import okhttp3.OkHttpClient

/** Signed contact and protected custody share source-owned account and business authority boundaries. */
@Module
@InstallIn(SingletonComponent::class)
object EnterpriseModule {
    @Provides
    @Singleton
    fun sessionFence(): EnterpriseSessionFence = EnterpriseSessionFence()

    @Provides
    @Singleton
    fun vaultFiles(@ApplicationContext context: Context): EnterpriseVaultFiles =
        AtomicEnterpriseVaultFiles(File(context.noBackupFilesDir, "enterprise-sealed-v1"))

    @Provides
    @Singleton
    fun vaultKeys(): EnterpriseVaultKeys = AndroidEnterpriseVaultKeys()

    @Provides
    @Singleton
    fun erasure(files: EnterpriseVaultFiles, keys: EnterpriseVaultKeys, fence: EnterpriseSessionFence): EnterpriseVaultErasure =
        EnterpriseVaultErasure(files, keys, fence)

    @Provides
    @Singleton
    fun vaultJanitor(@ApplicationContext context: Context, files: EnterpriseVaultFiles, keys: EnterpriseVaultKeys,
        fence: EnterpriseSessionFence, erasure: EnterpriseVaultErasure): EnterpriseVaultJanitor =
        EnterpriseVaultJanitor(files, keys, fence, erasure, AndroidEnterpriseDeviceClock(context)::read)

    @Provides
    @Singleton
    fun contactAuthority(client: EnterpriseDeviceContactClient): EnterpriseContactAuthority = client

    @Provides
    @Singleton
    fun contactSigner(proof: PhotoProofTransport): EnterpriseDeviceSigner = EnterpriseIdentityV1ContactSigner(proof)

    @Provides
    @Singleton
    fun signedContact(
        @Named("apiOrigin") environment: ApiEnvironment, sessions: AuthSessionStore, auth: AuthFlow,
        fence: EnterpriseSessionFence, signer: EnterpriseDeviceSigner,
    ): EnterpriseDeviceContactClient = EnterpriseDeviceContactClient(
        OkHttpClient.Builder().connectTimeout(10, TimeUnit.SECONDS).readTimeout(10, TimeUnit.SECONDS).build(),
        environment.origin, sessions, auth, fence, signer,
    )

    @Provides
    @Singleton
    fun contextProtection(store: ProtectedEnterprisePersistence): EnterpriseContextProtection = store

    @Provides
    @Singleton
    fun protectedStore(
        @ApplicationContext context: Context,
        files: EnterpriseVaultFiles, keys: EnterpriseVaultKeys, fence: EnterpriseSessionFence,
        erasure: EnterpriseVaultErasure, sessions: AuthSessionStore,
        @Named("apiOrigin") environment: ApiEnvironment,
        authority: EnterpriseContactAuthority,
    ): ProtectedEnterprisePersistence = ProtectedEnterprisePersistence(
        files, keys, fence, erasure,
        identity = {
            runCatching {
                sessions.load()?.let { EnterpriseLocalIdentity(it.accountId, environment.origin, it.familyId) }
            }.getOrNull()
        },
        clock = AndroidEnterpriseDeviceClock(context)::read,
        authority = authority,
    )

    @Provides
    @Singleton
    fun persistence(store: ProtectedEnterprisePersistence): EnterprisePersistence = store

    @Provides
    @Singleton
    fun localContext(
        auth: AuthFlow, @Named("apiOrigin") environment: ApiEnvironment, persistence: EnterprisePersistence,
        fence: EnterpriseSessionFence,
    ): EnterpriseLocalContext =
        EnterpriseLocalContext(
            ownerAccount = {
                when (val state = auth.rehydrate()) {
                    is AuthFlow.AuthState.Active -> state.session.accountId
                    is AuthFlow.AuthState.NeedsRefresh -> state.session.accountId
                    else -> null
                }
            },
            serverOrigin = { environment.origin },
            persistence = persistence,
            accessEpoch = { fence.epoch },
        )
}
