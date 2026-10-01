package com.anpfuel.data.di

import android.content.Context
import com.anpfuel.application.portable.LocationEnvironment
import com.anpfuel.application.portable.LocationFlow
import com.anpfuel.application.portable.LocationPorts
import com.anpfuel.application.portable.LocationSignalSource
import com.anpfuel.data.BuildConfig
import com.anpfuel.data.local.location.AndroidLocationSignals
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/**
 * Location integrity Hilt bindings (P16-T02).
 *
 * [LocationFlow] stays platform-free; Android owns the one-shot
 * signal read (last-known fix plus framework mock flag), the build
 * environment and nothing else. No location updates are ever
 * requested here: freshness is enforced by the frozen contract, and
 * debug injection exists only in debuggable builds (a static gate in
 * `scripts/check-mobile.sh` asserts production sources carry no
 * test-injection hook).
 */
@Module
@InstallIn(SingletonComponent::class)
object LocationModule {

    @Provides
    @Singleton
    fun provideLocationSignals(@ApplicationContext context: Context): LocationSignalSource {
        return AndroidLocationSignals(context)
    }

    @Provides
    @Singleton
    fun provideLocationEnvironment(): LocationEnvironment {
        return object : LocationEnvironment {
            override val allowTestInjection: Boolean = BuildConfig.DEBUG
        }
    }

    @Provides
    @Singleton
    fun provideLocationFlow(
        source: LocationSignalSource,
        environment: LocationEnvironment,
    ): LocationFlow {
        return LocationFlow(LocationPorts(source, environment))
    }
}
