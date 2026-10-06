package com.anpfuel.data.remote.profile

import com.anpfuel.application.usecase.profile.OwnedProfileClaim
import com.anpfuel.application.usecase.profile.ProfileClaimGateway
import com.anpfuel.domain.portable.PortableAuth
import java.io.IOException
import java.time.Instant
import java.util.Base64
import java.util.UUID
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONArray
import org.json.JSONObject

class ProfileHttpFailure(val status: Int) : IOException("Profile request refused ($status)")

/** No automatic mutation retries/cache/logging. Server checks every account/grant. */
class ProfileClaimHttpClient(private val client: OkHttpClient, private val baseUrl: String) : ProfileClaimGateway {
    private fun id(value: String): String {
        require(UUID.fromString(value).toString() == value.lowercase())
        return value
    }

    private fun auth(session: PortableAuth.Session) = JSONObject()
        .put("family_id", session.familyId).put("access_token", session.accessToken)

    private suspend fun post(path: String, body: JSONObject): JSONObject = withContext(Dispatchers.IO) {
        val request = Request.Builder().url("${baseUrl.trimEnd('/')}$path")
            .header("Cache-Control", "no-store")
            .post(body.toString().toRequestBody("application/json".toMediaType())).build()
        client.newCall(request).execute().use { response ->
            if (!response.isSuccessful) throw ProfileHttpFailure(response.code)
            val source = response.body?.source() ?: throw IOException("Empty profile response")
            if (source.request(64L * 1024L + 1L)) throw IOException("Profile response exceeds limit")
            try { JSONObject(source.readUtf8()) } catch (_: Exception) { throw IOException("Invalid profile response") }
        }
    }

    private fun claim(doc: JSONObject): OwnedProfileClaim {
        try {
            val scopes = doc.getJSONArray("scopes")
            return OwnedProfileClaim(
                id(doc.getString("id")), id(doc.getString("station_id")), doc.getString("role"),
                (0 until scopes.length()).map { scopes.getString(it) }.toSet(),
                doc.getString("state"), id(doc.getString("declaration_id")),
                doc.getString("declaration"), Instant.parse(doc.getString("expires_at")).epochSecond, doc.getString("declaration_state"),
            )
        } catch (_: Exception) { throw IOException("Invalid claim response") }
    }

    override suspend fun open(session: PortableAuth.Session, stationId: String, role: String, scopes: Set<String>, key: String) =
        claim(post("/v1/stations/${id(stationId)}/claims", auth(session)
            .put("role", role).put("scopes", JSONArray(scopes.sorted())).put("client_key", key)))

    override suspend fun mine(session: PortableAuth.Session): List<String> {
        val items = post("/v1/profile/claims/mine", auth(session)).getJSONArray("items")
        if (items.length() > 20) throw IOException("Claim list exceeds limit")
        return (0 until items.length()).map { id(items.getJSONObject(it).getString("id")) }
    }
    override suspend fun status(session: PortableAuth.Session, claimId: String) =
        claim(post("/v1/profile/claims/${id(claimId)}/status", auth(session)))
    override suspend fun reissue(session: PortableAuth.Session, claimId: String) =
        claim(post("/v1/profile/claims/${id(claimId)}/reissue", auth(session)))
    override suspend fun cancel(session: PortableAuth.Session, claimId: String) {
        post("/v1/profile/claims/${id(claimId)}/cancel", auth(session))
    }
    override suspend fun submit(session: PortableAuth.Session, claim: OwnedProfileClaim, bytes: ByteArray) {
        require(bytes.isNotEmpty() && bytes.size <= 5 * 1024 * 1024)
        val response = post("/v1/profile/claims/${id(claim.id)}/proof", auth(session)
            .put("declaration_id", id(claim.declarationId))
            .put("filename", "signed.pdf").put("content_type", "application/pdf")
            // Conservative retention: embedded scans never inherit multi-year authorization storage.
            .put("kind", "scan").put("content_base64", Base64.getEncoder().encodeToString(bytes)))
        if (response.optString("status") !in setOf("received", "checking", "valid", "indeterminate")) {
            throw IOException("Proof acknowledgment unavailable")
        }
    }
    override suspend fun edit(session: PortableAuth.Session, stationId: String, revision: Int, fields: Map<String, String>) {
        post("/v1/stations/${id(stationId)}/profile", auth(session)
            .put("expected_revision", revision).put("fields", JSONObject(fields)))
    }
    override suspend fun reply(session: PortableAuth.Session, stationId: String, product: String, text: String) {
        post("/v1/stations/${id(stationId)}/profile/replies", auth(session).put("product", product).put("text", text))
    }
}
