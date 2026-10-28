package com.anpfuel.data.di

import android.content.Context
import com.anpfuel.data.BuildConfig
import com.anpfuel.data.local.dao.MapPackageDao
import com.anpfuel.data.map.AssetMapRegionResolver
import com.anpfuel.data.map.MapActivationObserver
import com.anpfuel.data.map.MapDistribution
import com.anpfuel.data.map.MapDistributionConfig
import com.anpfuel.data.map.MapDistributionKeys
import com.anpfuel.data.map.MapPackageRepository
import com.anpfuel.data.map.MapPackageStore
import com.anpfuel.data.map.MapRegionDismissalStore
import com.anpfuel.data.map.MapRegionResolver
import com.anpfuel.data.map.RoomMapPackageStore
import com.anpfuel.data.map.SharedPreferencesMapRegionDismissalStore
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.domain.map.MapContractException
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import java.io.File
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object MapModule {
    // Explicit debug integration origin only; release always retains HTTPS-only.
    internal fun mapDistributionHttpClient(config: MapDistributionConfig): okhttp3.OkHttpClient {
        val client = OkHttpClientFactory.create(maxRetries = 0)
        val origin = runCatching { java.net.URI(config.baseUrl) }.getOrNull()
        if (!BuildConfig.DEBUG || origin?.scheme != "http" ||
            origin.host !in setOf("127.0.0.1", "localhost")) return client
        val loopbackGuard = okhttp3.Interceptor { chain ->
            val url = chain.request().url
            if (!url.isHttps && (url.host != origin.host || url.port != origin.port)) {
                throw java.io.IOException("Only the configured loopback proof origin permits HTTP")
            }
            chain.proceed(chain.request())
        }
        return client.newBuilder().apply {
            interceptors().removeAll { it is com.anpfuel.data.remote.interceptor.HttpsOnlyInterceptor }
            addInterceptor(loopbackGuard)
            addNetworkInterceptor(loopbackGuard)
        }.build()
    }

    @Provides
    @Singleton
    fun provideMapPackageStore(dao: MapPackageDao): MapPackageStore = RoomMapPackageStore(dao)

    @Provides
    @Singleton
    fun provideMapDistributionConfig(): MapDistributionConfig {
        val keysRaw = BuildConfig.MAP_DIST_KEYS
        var keysInvalid = false
        val keys = try {
            MapDistributionKeys.parse(keysRaw)
        } catch (_: MapContractException) {
            keysInvalid = keysRaw.isNotBlank()
            emptyMap()
        }
        return MapDistributionConfig(
            baseUrl = BuildConfig.MAP_DIST_BASE_URL.trimEnd('/'),
            // Optional default; the app picks the region of the opened city at runtime.
            region = BuildConfig.MAP_DIST_REGION,
            trustedKeys = keys,
            keysInvalid = keysInvalid,
        )
    }

    @Provides
    @Singleton
    fun provideMapRegionResolver(impl: AssetMapRegionResolver): MapRegionResolver = impl

    @Provides
    @Singleton
    fun provideMapRegionDismissalStore(impl: SharedPreferencesMapRegionDismissalStore): MapRegionDismissalStore = impl

    @Provides
    @Singleton
    fun provideMapDistribution(
        config: MapDistributionConfig,
    ): MapDistribution = MapDistribution(
        http = mapDistributionHttpClient(config),
        config = config,
    )

    @Provides
    @Singleton
    fun provideMapPackageRepository(
        store: MapPackageStore,
        @ApplicationContext context: Context,
        activationObserver: MapActivationObserver,
        config: MapDistributionConfig,
    ): MapPackageRepository = MapPackageRepository(
        store = store,
        filesRoot = File(context.filesDir, "maps"),
        http = mapDistributionHttpClient(config),
        nativeRoot = context.filesDir,
        activationObserver = activationObserver,
    )
}
