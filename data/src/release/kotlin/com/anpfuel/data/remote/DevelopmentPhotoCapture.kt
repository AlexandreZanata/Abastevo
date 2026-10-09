package com.anpfuel.data.remote

import org.json.JSONObject

internal object DevelopmentPhotoCapture {
    const val available = false
    fun request(origin: String, stationId: String, clientId: String): JSONObject {
        throw UnsupportedOperationException("development capture unavailable")
    }
}
