package com.anpfuel.app.ui.map

import com.anpfuel.data.map.MapActivationObserver
import com.anpfuel.maps.engine.MapEngine
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** B-BR-MAP-NATIVE-SYNC-01: detach lazy native readers before publication. */
@Singleton
class MapEngineActivationObserver @Inject constructor(
    private val engine: MapEngine,
) : MapActivationObserver {
    override suspend fun beforeActivation() = withContext(Dispatchers.Main.immediate) {
        if (engine.isInitialized()) {
            check(engine.suspendMapReads()) { "native-map-readers-busy" }
        }
    }

    override suspend fun afterActivation() = withContext(Dispatchers.Main.immediate) {
        if (engine.isInitialized()) {
            check(engine.reloadMaps()) { "native-map-reload-failed" }
        }
    }
}
