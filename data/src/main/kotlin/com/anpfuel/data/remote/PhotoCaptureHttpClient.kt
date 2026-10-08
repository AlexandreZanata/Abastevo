package com.anpfuel.data.remote

import com.anpfuel.application.port.CaptureFix
import com.anpfuel.application.port.PhotoCaptureGate
import com.anpfuel.application.port.PhotoCapturePermission
import java.io.IOException
import java.time.Instant
import java.util.UUID
import javax.inject.Inject
import javax.inject.Singleton
import org.json.JSONObject

@Singleton
class PhotoCaptureHttpClient @Inject constructor(private val transport: PhotoProofTransport): PhotoCaptureGate {
    override fun isCurrent(permission: PhotoCapturePermission): Boolean =
        permission.origin == transport.origin && permission.ownerScope == transport.ownerScope()

    override suspend fun authorize(stationId: String,clientCaptureId: String,fix: CaptureFix): PhotoCapturePermission {
        require(UUID.fromString(stationId).toString()==stationId.lowercase())
        val scope=transport.identityScope()
        val location=JSONObject().put("verdict","VERIFIED").put("permission_granted",fix.permissionGranted)
            .put("has_fix",true).put("source_info_present",fix.sourceInfoPresent).put("simulated",fix.simulated)
            .put("accuracy_m",fix.accuracyMeters).put("manual",false)
            .put("captured_at",Instant.ofEpochMilli(fix.capturedAtMillis).toString())
            .put("lat",fix.latitude).put("lon",fix.longitude)
        val result=transport.post("/v1/photo-captures",clientCaptureId,JSONObject()
            .put("client_capture_id",clientCaptureId).put("station_id",stationId).put("location",location))
        return decode(result, stationId, scope, false)
    }

    override suspend fun authorizeDevelopmentPreview(stationId: String, clientCaptureId: String): PhotoCapturePermission {
        check(DevelopmentPhotoCapture.available)
        require(UUID.fromString(stationId).toString() == stationId.lowercase())
        val request = DevelopmentPhotoCapture.request(transport.origin, stationId, clientCaptureId)
        val scope = transport.identityScope()
        return decode(transport.post("/v1/photo-captures", clientCaptureId, request), stationId, scope, true)
    }

    private fun decode(result: JSONObject, stationId: String, scope: String, development: Boolean): PhotoCapturePermission {
        val receipt=PhotoCapturePermission(result.getString("capture_id"),result.getString("station_id"),
            Instant.parse(result.getString("issued_at")).toEpochMilli(),Instant.parse(result.getString("camera_expires_at")).toEpochMilli(),
            Instant.parse(result.getString("expires_at")).toEpochMilli(),scope,transport.origin, development)
        if(!isCurrent(receipt) || receipt.stationId!=stationId || result.optString("policy_version") != (if (development) "photo-capture-ui-test-v1" else "photo-capture-v1") ||
            receipt.cameraExpiresAtMillis-receipt.issuedAtMillis !in 1..120000 ||
            receipt.expiresAtMillis-receipt.issuedAtMillis !in 1..86400000 ||
            receipt.expiresAtMillis < receipt.cameraExpiresAtMillis ||
            receipt.issuedAtMillis > System.currentTimeMillis() ||
            System.currentTimeMillis()>=receipt.cameraExpiresAtMillis) throw IOException("photo.invalid-permission")
        UUID.fromString(receipt.captureId)
        return receipt
    }
}
