package com.anpfuel.app.di

import com.anpfuel.app.notification.SystemNotificationPosterImpl
import com.anpfuel.domain.notification.SystemNotificationPoster
import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

/** Android notifications are worded in the app module, which owns every locale (B-BR-NOTIF-04/05). */
@Module
@InstallIn(SingletonComponent::class)
abstract class NotificationPosterModule {
    @Binds
    abstract fun bindSystemNotificationPoster(impl: SystemNotificationPosterImpl): SystemNotificationPoster
}
