package com.anpfuel.maps.engine

import android.content.Context
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/**
 * Process-scoped singleton engine. One [MapEngine] per application process;
 * Compose hosts share it and never create a second native core. The engine
 * owns initialization, overlay generations and location; hosts own surfaces,
 * camera persistence and explicit recenter actions.
 */
@Module
@InstallIn(SingletonComponent::class)
object MapEngineModule {
    @Provides
    @Singleton
    fun provideMapEngine(): MapEngine = MapEngineHolder.engine

    @Provides
    @Singleton
    fun provideMapCameraStore(@ApplicationContext context: Context): MapCameraStore =
        SharedPreferencesMapCameraStore(context)
}

/** Process-lifetime holder; Hilt singleton delegates here for testability. */
object MapEngineHolder {
    @Volatile
    private var instance: MapEngine? = null

    val engine: MapEngine
        get() = instance ?: synchronized(this) {
            instance ?: ComapsMapEngine().also { instance = it }
        }

    @Synchronized
    fun resetForTests() {
        instance = null
    }
}
