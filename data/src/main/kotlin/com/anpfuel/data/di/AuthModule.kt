package com.anpfuel.data.di

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.portable.AuthAccountApi
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthKeyStore
import com.anpfuel.application.portable.AuthPorts
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.portable.AuthWallClock
import com.anpfuel.application.portable.PortableNonceSource
import com.anpfuel.data.local.auth.AccountHttpApi
import com.anpfuel.data.local.auth.AndroidKeystoreKeys
import com.anpfuel.data.local.auth.KeystoreAccountKey
import com.anpfuel.data.local.auth.KeystoreSessionStore
import com.anpfuel.data.local.auth.SessionKeyProvider
import com.anpfuel.data.remote.ApiEnvironment
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import java.util.UUID
import java.util.concurrent.TimeUnit
import javax.inject.Named
import javax.inject.Singleton
import okhttp3.OkHttpClient

/**
 * FREE-account Hilt bindings (P13-T05B, origin shared in P34-T02).
 *
 * The shared [ApiEnvironment] origin selects the backend; preview
 * (`RFC 2606 `.invalid``, never resolves) remains available as
 * [ApiEnvironment.PREVIEW] but DI uses the explicit staging selection.
 * [AuthFlow] stays platform-free; Android owns storage
 * (Keystore), time, randomness and HTTP here. Environment changes never
 * reuse sessions without re-auth: stale/foreign blobs fail closed to
 * logged-out via [KeystoreSessionStore].
 */
@Module
@InstallIn(SingletonComponent::class)
object AuthModule {

    /** Preview API base; kept for reference, DI uses the shared origin. */
    const val PREVIEW_BASE_URL: String = "https://api.anpfuel.example.invalid"

    @Provides
    @Singleton
    @Named("auth")
    fun provideAuthPrefs(@ApplicationContext context: Context): SharedPreferences {
        return context.getSharedPreferences("anpfuel_auth", Context.MODE_PRIVATE)
    }

    @Provides
    @Singleton
    fun provideSessionKeys(): SessionKeyProvider = AndroidKeystoreKeys()

    @Provides
    @Singleton
    fun provideSessionStore(
        @Named("auth") prefs: SharedPreferences,
        keys: SessionKeyProvider,
    ): AuthSessionStore = KeystoreSessionStore(prefs, keys)

    @Provides
    @Singleton
    fun provideAccountKeyStore(
        @Named("auth") prefs: SharedPreferences,
        keys: SessionKeyProvider,
    ): AuthKeyStore = KeystoreAccountKey(prefs, keys)

    @Provides
    @Singleton
    fun provideAccountApi(
        @Named("apiOrigin") environment: ApiEnvironment,
    ): AuthAccountApi {
        val client = OkHttpClient.Builder()
            .connectTimeout(10L, TimeUnit.SECONDS)
            .readTimeout(10L, TimeUnit.SECONDS)
            .build()
        return AccountHttpApi(client, environment.origin)
    }

    @Provides
    @Singleton
    fun provideAuthFlow(
        store: AuthSessionStore,
        keyStore: AuthKeyStore,
        api: AuthAccountApi,
    ): AuthFlow {
        val ports = AuthPorts(
            store = store,
            keys = keyStore,
            clock = AuthWallClock { System.currentTimeMillis() / 1000L },
            nonces = PortableNonceSource { UUID.randomUUID().toString() },
            api = api,
        )
        return AuthFlow(ports)
    }
}
