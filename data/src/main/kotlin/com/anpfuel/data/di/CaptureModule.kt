package com.anpfuel.data.di

import com.anpfuel.application.port.PhotoCaptureGate
import com.anpfuel.data.remote.PhotoCaptureHttpClient
import com.anpfuel.application.port.OcrPort
import com.anpfuel.application.port.ImagePriceOcr
import com.anpfuel.data.local.ocr.MlKitImagePriceOcr
import com.anpfuel.data.local.ocr.LocalRegexPriceOcr
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/** Local text parsing and real on-device pixel recognition; dependency review in p37-image-ocr.md. */
@Module
@InstallIn(SingletonComponent::class)
object CaptureModule {

    @Provides
    @Singleton
    fun provideOcrPort(impl: LocalRegexPriceOcr): OcrPort = impl

    @Provides
    @Singleton
    fun provideImageOcr(impl: MlKitImagePriceOcr): ImagePriceOcr = impl
    @Provides
    @Singleton
    fun providePhotoCaptureGate(impl: PhotoCaptureHttpClient): PhotoCaptureGate = impl
}
