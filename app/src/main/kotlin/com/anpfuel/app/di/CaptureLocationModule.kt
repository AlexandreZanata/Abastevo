package com.anpfuel.app.di

import com.anpfuel.app.location.LocationPermissionHandler
import com.anpfuel.application.port.CaptureLocationSource
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

@Module
@InstallIn(SingletonComponent::class)
object CaptureLocationModule {
    @Provides
    fun captureLocationSource(handler: LocationPermissionHandler): CaptureLocationSource =
        CaptureLocationSource { handler.getFreshCaptureFix() }
}
