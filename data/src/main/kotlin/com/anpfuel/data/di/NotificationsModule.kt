package com.anpfuel.data.di

import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.usecase.notification.EvaluateWeeklyLocalAlertsUseCase
import com.anpfuel.application.usecase.notification.LoadNotificationsUseCase
import com.anpfuel.application.usecase.notification.MarkNotificationsReadUseCase
import com.anpfuel.application.usecase.notification.SyncAccountNotificationsUseCase
import com.anpfuel.data.local.notifications.LocalNotificationStore
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.PhotoProofTransport
import com.anpfuel.data.remote.notifications.AccountNotificationsHttpClient
import com.anpfuel.domain.notification.AccountNotificationCache
import com.anpfuel.domain.notification.AccountNotificationGateway
import com.anpfuel.domain.notification.AccountNotificationShownStore
import com.anpfuel.domain.notification.LocalNotificationInbox
import com.anpfuel.domain.notification.SystemNotificationPoster
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Named
import javax.inject.Singleton

/** B-BR-NOTIF-01..05 bindings: device inbox, account transport and use cases. */
@Module
@InstallIn(SingletonComponent::class)
object NotificationsModule {

    @Provides
    fun localInbox(store: LocalNotificationStore): LocalNotificationInbox = store

    @Provides
    fun accountCache(store: LocalNotificationStore): AccountNotificationCache = store

    @Provides
    fun shownStore(store: LocalNotificationStore): AccountNotificationShownStore = store

    @Provides
    @Singleton
    fun accountGateway(
        @Named("apiOrigin") environment: ApiEnvironment,
        auth: AuthFlow,
        proof: PhotoProofTransport,
    ): AccountNotificationGateway =
        AccountNotificationsHttpClient(OkHttpClientFactory.create(maxRetries = 0), environment.origin, auth, proof)

    @Provides
    fun loadNotifications(
        local: LocalNotificationInbox,
        account: AccountNotificationGateway,
        cache: AccountNotificationCache,
    ): LoadNotificationsUseCase = LoadNotificationsUseCase(local, account, cache)

    @Provides
    fun markNotificationsRead(
        local: LocalNotificationInbox,
        account: AccountNotificationGateway,
        cache: AccountNotificationCache,
    ): MarkNotificationsReadUseCase = MarkNotificationsReadUseCase(local, account, cache)

    @Provides
    fun syncAccountNotifications(
        account: AccountNotificationGateway,
        cache: AccountNotificationCache,
        shown: AccountNotificationShownStore,
        poster: SystemNotificationPoster,
    ): SyncAccountNotificationsUseCase = SyncAccountNotificationsUseCase(account, cache, shown, poster)

    @Provides
    fun weeklyLocalAlerts(
        averagePrices: AveragePriceRepository,
        priceTables: PriceTableRepository,
        preferences: UserPreferencesRepository,
        inbox: LocalNotificationInbox,
        poster: SystemNotificationPoster,
    ): EvaluateWeeklyLocalAlertsUseCase = EvaluateWeeklyLocalAlertsUseCase(averagePrices, priceTables, preferences, inbox, poster)
}
