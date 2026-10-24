package com.anpfuel.data.di

import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.ranking.PriceRankingHttpClient
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/**
 * MEX-31 bounded public price-ranking reads (MEX-29, anonymous).
 *
 * No response cache: each ranked page is fetched with its full filter
 * set; ETag/304 handling is intentionally absent (revalidation would
 * serve stale ranks after a revision switch).
 */
@Module
@InstallIn(SingletonComponent::class)
object RankingModule {

    @Provides
    @Singleton
    fun providePriceRankingHttpClient(
        @Named("apiOrigin") environment: ApiEnvironment,
    ): PriceRankingHttpClient = PriceRankingHttpClient(OkHttpClientFactory.create(maxRetries = 0), environment.origin)
}
