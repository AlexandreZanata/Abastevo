package com.anpfuel.data.di

import android.content.Context
import com.anpfuel.application.enterprise.EnterpriseLocalContext
import com.anpfuel.application.enterprise.EnterprisePersistence
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.UnavailableEnterprisePersistence
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.enterprise.*
import com.anpfuel.data.remote.ApiEnvironment
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import java.io.File
import javax.inject.Named
import javax.inject.Singleton

/** Native/server acceptance remains required; no plaintext DB builder or feature activation. */
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
    fun contactAuthority(): EnterpriseContactAuthority = UnavailableEnterpriseContact

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
    fun persistence(): EnterprisePersistence = UnavailableEnterprisePersistence

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
