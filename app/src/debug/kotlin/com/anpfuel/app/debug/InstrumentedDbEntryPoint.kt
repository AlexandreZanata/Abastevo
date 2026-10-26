package com.anpfuel.app.debug

import com.anpfuel.data.local.AnpFuelDatabase
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
}
