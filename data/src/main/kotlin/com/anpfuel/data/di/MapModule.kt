package com.anpfuel.data.di

import android.content.Context
import com.anpfuel.data.BuildConfig
import com.anpfuel.data.local.dao.MapPackageDao
import com.anpfuel.data.map.MapDistribution
import com.anpfuel.data.map.MapDistributionConfig
import com.anpfuel.data.map.MapDistributionKeys
import com.anpfuel.data.map.MapPackageRepository
import com.anpfuel.data.map.MapPackageStore
import com.anpfuel.data.map.RoomMapPackageStore
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
            region = BuildConfig.MAP_DIST_REGION,
            trustedKeys = keys,
            keysInvalid = keysInvalid,
        )
    }

    @Provides
    @Singleton
    fun provideMapDistribution(
        config: MapDistributionConfig,
    ): MapDistribution = MapDistribution(
        http = OkHttpClientFactory.create(maxRetries = 0),
        config = config,
    )

    @Provides
    @Singleton
    fun provideMapPackageRepository(
        store: MapPackageStore,
        @ApplicationContext context: Context,
    ): MapPackageRepository = MapPackageRepository(
        store = store,
        filesRoot = File(context.filesDir, "maps"),
        http = OkHttpClientFactory.create(maxRetries = 0),
        nativeRoot = context.filesDir,
    )
}
