package com.anpfuel.data.remote

import org.json.JSONObject

internal object DevelopmentPhotoCapture {
    const val available = true
    fun request(origin: String, stationId: String, clientId: String): JSONObject {
        require(origin == ApiEnvironment.STAGING.origin) { "development capture requires owned staging origin" }
        return JSONObject().put("client_capture_id",clientId).put("station_id",stationId).put("development_preview",true)
    }
}
