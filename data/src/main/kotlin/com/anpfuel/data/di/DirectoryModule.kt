package com.anpfuel.data.di

import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.DirectoryStationHttpClient
import com.anpfuel.data.remote.OkHttpClientFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/**
 * P35-T01 canonical Directory bindings (origin shared from P34-T02).
 *
 * Network clients use the shared [ApiEnvironment] staging origin;
 * the memory cache keeps last-known server pages for failed-refresh
 * recovery. No Room migration in this slice.
 */
@Module
@InstallIn(SingletonComponent::class)
object DirectoryModule {

    @Provides
    @Singleton
    fun provideDirectoryStationHttpClient(
        @Named("apiOrigin") environment: ApiEnvironment,
    ): DirectoryStationHttpClient =
        DirectoryStationHttpClient(
            client = OkHttpClientFactory.create(),
            baseUrl = environment.origin,
        )
    @Provides
    @Singleton
    fun provideServerStationCache(
        dao: com.anpfuel.data.local.dao.ServerStationCacheDao,
        @Named("apiOrigin") environment: ApiEnvironment,
    ): com.anpfuel.domain.repository.ServerStationCache =
        com.anpfuel.data.repository.DirectoryStationRoomCache(dao, environment.origin)

}
