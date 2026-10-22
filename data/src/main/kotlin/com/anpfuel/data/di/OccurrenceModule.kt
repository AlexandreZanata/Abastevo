package com.anpfuel.data.di

import com.anpfuel.data.local.quality.QualityPublicCache
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.occurrence.OccurrenceHttpClient
import com.anpfuel.domain.repository.OccurrenceGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object OccurrenceModule {
    @Provides @Singleton
    fun gateway(@Named("apiOrigin") origin: ApiEnvironment,@dagger.hilt.android.qualifiers.ApplicationContext context:android.content.Context): OccurrenceGateway =
        OccurrenceHttpClient(OkHttpClientFactory.create(maxRetries=0),origin.origin,
            if(com.anpfuel.data.BuildConfig.QUALITY_CACHE_ENABLED)QualityPublicCache(java.io.File(context.cacheDir,"occurrence-public-v1")) else null)
}
