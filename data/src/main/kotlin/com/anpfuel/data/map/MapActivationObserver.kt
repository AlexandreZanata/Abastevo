package com.anpfuel.data.map

/** Application adapter coordinates native readers without exposing SDK types to data. */
interface MapActivationObserver {
    suspend fun beforeActivation()
    suspend fun afterActivation()

    object None : MapActivationObserver {
        override suspend fun beforeActivation() = Unit
        override suspend fun afterActivation() = Unit
    }
}
