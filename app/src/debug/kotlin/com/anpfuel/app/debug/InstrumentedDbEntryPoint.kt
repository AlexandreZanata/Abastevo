package com.anpfuel.app.debug

import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.data.repository.DriverLocalStore
import dagger.hilt.EntryPoint
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

/**
 * Debug-only instrumentation seam: lets device tests seed the same Room singleton the
 * app uses instead of opening a second connection to the same file. Not packaged in release.
 */
@EntryPoint
@InstallIn(SingletonComponent::class)
interface InstrumentedDbEntryPoint {
    fun database(): AnpFuelDatabase
    fun driverStore(): DriverLocalStore
    fun sessions(): AuthSessionStore
}
