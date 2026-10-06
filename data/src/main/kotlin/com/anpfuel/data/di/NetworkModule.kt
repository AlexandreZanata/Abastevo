package com.anpfuel.data.di

import android.content.Context
import com.anpfuel.data.remote.AnpFileDownloader
import com.anpfuel.data.remote.AnpListingScraper
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.NominatimOkHttpClientFactory
import com.anpfuel.data.remote.NominatimClient
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import java.time.Clock
import okhttp3.OkHttpClient
import javax.inject.Named
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object NetworkModule {

    @Provides
    @Singleton
    fun provideOkHttpClient(): OkHttpClient = OkHttpClientFactory.create()

    /**
     * P34-T02 shared staging origin.
     *
     * Explicit staging selection for P34-P38 construction. The release target
     * stays separate and requires its own certification; do not reuse this
     * test origin as production without that gate.
     */
    @Provides
    @Singleton
    @Named("apiOrigin")
    fun provideApiEnvironment(): ApiEnvironment = ApiEnvironment.STAGING

    @Provides
    @Singleton
    fun provideAnpListingScraper(okHttpClient: OkHttpClient): AnpListingScraper =
        AnpListingScraper(okHttpClient)

    @Provides
    @Singleton
    fun provideAnpFileDownloader(
        @ApplicationContext context: Context,
        okHttpClient: OkHttpClient,
    ): AnpFileDownloader = AnpFileDownloader(context, okHttpClient)

    @Provides
    @Singleton
    fun provideClock(): Clock = Clock.systemUTC()

    @Provides
    @Singleton
    @NominatimClient
    fun provideNominatimOkHttpClient(): OkHttpClient = NominatimOkHttpClientFactory.create()
}
