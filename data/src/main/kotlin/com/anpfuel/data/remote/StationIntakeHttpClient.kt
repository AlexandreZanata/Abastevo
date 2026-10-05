package com.anpfuel.data.remote

import java.io.IOException
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

/**
 * P27-T04 private intake HTTP client.
 *
 * Session travels in the JSON body on every call (never the URL);
 * transport failures and non-2xx/empty bodies throw [IOException].
 * Idempotency keys make offline retries safe: the same key replays
 * the record instead of duplicating the visible station. HTTPS only.
 */
class StationIntakeHttpClient(
    private val client: OkHttpClient,
    private val baseUrl: String,
) {
    companion object {
        private val JSON = "application/json".toMediaType()
    }

    fun submit(
        familyId: String,
        accessToken: String,
        clientSubmissionId: String,
        proposalJson: String,
    ): String {
        require(familyId.isNotBlank()) { "family_id is blank" }
        require(accessToken.isNotBlank()) { "access_token is blank" }
        require(clientSubmissionId.isNotBlank()) { "client_submission_id is blank" }
        val proposal = try {
            JSONObject(proposalJson)
        } catch (_: Exception) {
            throw IOException("intake submit failed: malformed proposal")
        }
        val body = JSONObject()
            .put("family_id", familyId)
            .put("access_token", accessToken)
            .put("client_submission_id", clientSubmissionId)
            .put("proposal", proposal)
            .toString()
        return post("/v1/stations/suggestions", body)
    }

    fun mine(familyId: String, accessToken: String): String {
        require(familyId.isNotBlank()) { "family_id is blank" }
        require(accessToken.isNotBlank()) { "access_token is blank" }
        val body = JSONObject()
            .put("family_id", familyId)
            .put("access_token", accessToken)
            .toString()
        return post("/v1/stations/suggestions/mine", body)
    }

    fun status(familyId: String, accessToken: String, id: String): String {
        require(id.isNotBlank()) { "suggestion id is blank" }
        val body = JSONObject()
            .put("family_id", familyId)
            .put("access_token", accessToken)
            .toString()
        return post("/v1/stations/suggestions/$id/status", body)
    }

    fun cancel(familyId: String, accessToken: String, id: String): String {
        require(id.isNotBlank()) { "suggestion id is blank" }
        val body = JSONObject()
            .put("family_id", familyId)
            .put("access_token", accessToken)
            .toString()
        return post("/v1/stations/suggestions/$id/cancel", body)
    }

    private fun post(path: String, body: String): String {
        val request = Request.Builder()
            .url(baseUrl.trimEnd('/') + path)
            .post(body.toRequestBody(JSON))
            .header("Accept", "application/json")
            .header("Cache-Control", "no-store")
            .build()
        try {
            client.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    throw IOException("intake call failed: HTTP ${response.code} $path")
                }
                val payload = response.body?.string()
                if (payload.isNullOrBlank()) {
                    throw IOException("intake call failed: empty body $path")
                }
                return payload
            }
        } catch (error: IOException) {
            throw error
        } catch (error: Exception) {
            throw IOException("intake call failed", error)
        }
    }
}
