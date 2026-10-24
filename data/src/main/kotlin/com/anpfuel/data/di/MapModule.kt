package com.anpfuel.data.di

import android.content.Context
import com.anpfuel.data.local.dao.MapPackageDao
import com.anpfuel.data.map.MapPackageRepository
import com.anpfuel.data.map.MapPackageStore
import com.anpfuel.data.map.RoomMapPackageStore
import com.anpfuel.data.remote.OkHttpClientFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import java.io.File
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object MapModule {
    @Provides
    @Singleton
    fun provideMapPackageStore(dao: MapPackageDao): MapPackageStore = RoomMapPackageStore(dao)

    @Provides
    @Singleton
    fun provideMapPackageRepository(
        store: MapPackageStore,
        @ApplicationContext context: Context,
    ): MapPackageRepository = MapPackageRepository(
        store = store,
        filesRoot = File(context.filesDir, "maps"),
        http = OkHttpClientFactory.create(maxRetries = 0),
    )
}
