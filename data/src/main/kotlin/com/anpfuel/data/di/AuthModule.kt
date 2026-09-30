package com.anpfuel.data.di

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.portable.AuthAccountApi
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthPorts
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.portable.AuthWallClock
import com.anpfuel.application.portable.PortableNonceSource
import com.anpfuel.data.local.auth.AccountHttpApi
import com.anpfuel.data.local.auth.AndroidKeystoreKeys
import com.anpfuel.data.local.auth.KeystoreSessionStore
import com.anpfuel.data.local.auth.SessionKeyProvider
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
 * FREE-account Hilt bindings (P13-T05B).
 *
 * The base URL is an explicit preview placeholder (RFC 2606
 * `.invalid`, never resolves): codes issue nowhere until deployment
 * configuration lands, mirroring the backend memory-mail-sink
 * precedent. [AuthFlow] stays platform-free; Android owns storage
 * (Keystore), time, randomness and HTTP here.
 */
@Module
@InstallIn(SingletonComponent::class)
object AuthModule {

    /** Preview API base; deployment configuration replaces it. */
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
    fun provideAccountApi(): AuthAccountApi {
        val client = OkHttpClient.Builder()
            .connectTimeout(10L, TimeUnit.SECONDS)
            .readTimeout(10L, TimeUnit.SECONDS)
            .build()
        return AccountHttpApi(client, PREVIEW_BASE_URL)
    }

    @Provides
    @Singleton
    fun provideAuthFlow(
        store: AuthSessionStore,
        api: AuthAccountApi,
    ): AuthFlow {
        val ports = AuthPorts(
            store = store,
            clock = AuthWallClock { System.currentTimeMillis() / 1000L },
            nonces = PortableNonceSource { UUID.randomUUID().toString() },
            api = api,
        )
        return AuthFlow(ports)
    }
}
