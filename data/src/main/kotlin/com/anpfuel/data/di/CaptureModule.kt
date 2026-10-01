package com.anpfuel.data.di

import com.anpfuel.application.port.OcrPort
import com.anpfuel.data.local.ocr.LocalRegexPriceOcr
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/**
 * P10-T04 capture/OCR bindings.
 *
 * Dependency rationale (recorded per ROADMAP P10-T04): capture uses the
 * system camera intent plus the existing [com.anpfuel.application.portable.PhotoFlow]
 * budgets (JPEG, 150 KiB target / 256 KiB cap, 3 attempts); no CameraX
 * controller is bundled in this slice. OCR candidates come from
 * [LocalRegexPriceOcr] over on-device text; the Play-services ML Kit
 * `text-recognition` engine (`com.google.mlkit:text-recognition`,
 * Apache-2.0, Google Play services, on-device, no bundled model in the
 * thin-client flavor) plugs in behind the same [OcrPort] at the device
 * pass (P10-T08) after APK-size, permission and offline measurement.
 * Rationale: preserve the 15 MB release budget, keep recognition
 * off-network/off-server, stay JVM-testable now and device-proven later.
 * No preview base URL and no upload client live in this module.
 */
@Module
@InstallIn(SingletonComponent::class)
object CaptureModule {

    @Provides
    @Singleton
    fun provideOcrPort(impl: LocalRegexPriceOcr): OcrPort = impl
}
