package com.anpfuel.data.di

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.mapsaved.*
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.saved.RoomSavedStore
import com.anpfuel.domain.map.saved.SavedPrivateStore
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/** MAP-25 private favorites/trips wiring. Local only: no network client, no sync dataset. */
@Module
@InstallIn(SingletonComponent::class)
object SavedModule {
    @Provides @Singleton
    fun provideSavedPrivateStore(db: AnpFuelDatabase): SavedPrivateStore = RoomSavedStore(db)

    @Provides @Singleton
    fun provideSavedOwnerResolver(
        sessions: AuthSessionStore,
        @Named("apiOrigin") environment: com.anpfuel.data.remote.ApiEnvironment,
    ): SavedOwnerResolver = SavedOwnerResolver(sessions, environment.origin)

    @Provides fun listSaved(store: SavedPrivateStore, owners: SavedOwnerResolver) = ListSavedUseCase(store, owners)
    @Provides fun isFavorite(store: SavedPrivateStore, owners: SavedOwnerResolver) = IsStationFavoriteUseCase(store, owners)
    @Provides fun toggleFavorite(store: SavedPrivateStore, owners: SavedOwnerResolver) = ToggleStationFavoriteUseCase(store, owners)
    @Provides fun saveRoutePlan(store: SavedPrivateStore, owners: SavedOwnerResolver) = SaveRoutePlanUseCase(store, owners)
    @Provides fun deleteSaved(store: SavedPrivateStore, owners: SavedOwnerResolver) = DeleteSavedUseCase(store, owners)
    @Provides fun getTrip(store: SavedPrivateStore, owners: SavedOwnerResolver) = GetSavedTripUseCase(store, owners)
    @Provides fun importExchange(store: SavedPrivateStore, owners: SavedOwnerResolver) = ImportExchangeUseCase(store, owners)
    @Provides fun exportExchange(store: SavedPrivateStore, owners: SavedOwnerResolver) = ExportExchangeUseCase(store, owners)
    @Provides @Singleton
    fun trackRecording(store: SavedPrivateStore, owners: SavedOwnerResolver) = TrackRecordingUseCase(store, owners)
}
