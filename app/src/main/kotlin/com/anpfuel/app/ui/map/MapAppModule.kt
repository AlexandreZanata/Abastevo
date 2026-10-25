package com.anpfuel.app.ui.map

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

@Module
@InstallIn(SingletonComponent::class)
interface MapAppModule {
    @Binds
    fun bindMapWorkControl(impl: WorkManagerControl): MapWorkControl
}
