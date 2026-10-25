package com.anpfuel.app.ui.map

import com.anpfuel.data.map.MapActivationObserver
import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

@Module
@InstallIn(SingletonComponent::class)
interface MapAppModule {
    @Binds
    fun bindMapActivationObserver(impl: MapEngineActivationObserver): MapActivationObserver

    @Binds
    fun bindMapWorkControl(impl: WorkManagerControl): MapWorkControl
}
