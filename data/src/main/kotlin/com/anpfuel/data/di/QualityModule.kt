package com.anpfuel.data.di

import com.anpfuel.application.usecase.quality.ReadQuality
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.quality.QualityHttpClient
import com.anpfuel.domain.repository.QualityGateway
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object QualityModule {
    @Provides @Singleton
    fun gateway(@Named("apiOrigin") origin:ApiEnvironment):QualityGateway =
        QualityHttpClient(OkHttpClientFactory.create(maxRetries=0),origin.origin)

    @Provides
    fun read(gateway:QualityGateway)=ReadQuality(gateway)
}
