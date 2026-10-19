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
    fun gateway(@Named("apiOrigin") origin:ApiEnvironment,@dagger.hilt.android.qualifiers.ApplicationContext context:android.content.Context):QualityGateway =
        QualityHttpClient(OkHttpClientFactory.create(maxRetries=0),origin.origin,
            if(com.anpfuel.data.BuildConfig.QUALITY_CACHE_ENABLED)com.anpfuel.data.local.quality.QualityPublicCache(java.io.File(context.cacheDir,"quality-public-v1")) else null)

    @Provides @Singleton @Named("qualitySignedReports")
    fun signedReports(proof:com.anpfuel.data.remote.PhotoProofTransport,auth:com.anpfuel.application.portable.AuthFlow):com.anpfuel.domain.repository.QualityReportGateway =
        com.anpfuel.data.remote.quality.QualityReportHttpClient.signed(proof,auth)

    @Provides @Singleton
    fun qualityKeys():com.anpfuel.data.local.quality.QualityReportKeys = com.anpfuel.data.local.quality.AndroidQualityReportKeys()

    @Provides @Singleton
    fun durable(@Named("qualitySignedReports") remote:com.anpfuel.domain.repository.QualityReportGateway,
        dao:com.anpfuel.data.local.dao.QualityReportOutboxDao,keys:com.anpfuel.data.local.quality.QualityReportKeys,
        scheduler:com.anpfuel.data.worker.QualityReportWorkScheduler,@dagger.hilt.android.qualifiers.ApplicationContext context:android.content.Context):com.anpfuel.data.repository.DurableQualityReports =
        com.anpfuel.data.repository.DurableQualityReports(remote,dao,keys,scheduler::enqueue,enabled={com.anpfuel.data.BuildConfig.QUALITY_OUTBOX_ENABLED},markers=com.anpfuel.data.local.quality.FileQualityErasureMarkers(java.io.File(context.noBackupFilesDir,"quality-erasure-v1")))

    @Provides
    fun reports(durable:com.anpfuel.data.repository.DurableQualityReports):com.anpfuel.domain.repository.QualityReportGateway=durable

    @Provides
    fun erasure(durable:com.anpfuel.data.repository.DurableQualityReports):com.anpfuel.application.port.QualityReportLocalErasure=durable

    @Provides
    fun read(gateway:QualityGateway)=ReadQuality(gateway)
}
