package com.anpfuel.app.ui.map

import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/**
 * Default sheet prices are explicit absence. Real ANP/community lookups own
 * their follow-up (MAP-19 full wiring uses the station detail + survey week);
 * this binding never invents a price and never falls back silently.
 */
@Module
@InstallIn(SingletonComponent::class)
object MapSheetModule {
    @Provides
    @Singleton
    fun provideSheetPriceSource(): SheetPriceSource =
        SheetPriceSource { _, _, _ -> null }
}
