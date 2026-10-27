package com.anpfuel.data.remote.contributor

import com.anpfuel.domain.contributor.Claim
import com.anpfuel.domain.contributor.ClaimProof
import com.anpfuel.domain.contributor.ClaimState
import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.ContributorLink
import com.anpfuel.domain.contributor.ContributorLinkHosts
import com.anpfuel.domain.contributor.IssuedClaim
import com.anpfuel.domain.contributor.ProfileEdit
import com.anpfuel.domain.contributor.ProfileRead
import com.anpfuel.domain.contributor.ProfileVisibility
import com.anpfuel.domain.contributor.StoredProfile
import com.anpfuel.domain.repository.ContributorSelfGateway
import java.io.IOException
import java.util.UUID
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import okhttp3.Call
import okhttp3.Callback
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import org.json.JSONArray
import org.json.JSONObject

private val JSON = "application/json; charset=utf-8".toMediaType()
private const val BODY_BUDGET = 256L * 1024L + 1
private const val MAX_ITEMS = 100

/**
 * B-BR-CTB signed-in contributor self-service transport.
 *
 * Proof pattern mirrors `/v1/premium/status`: `family_id` + `access_token`
 * body, signed owner on the server side. Server error codes pass through
 * verbatim so the UI shows them instead of inventing messages.
 */
class ContributorSelfHttpClient(
    http: OkHttpClient,
    baseUrl: String,
) : ContributorSelfGateway {

    private val origin = baseUrl.toHttpUrl()
    private val client = http.newBuilder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).callTimeout(20, TimeUnit.SECONDS).build()

    override suspend fun openClaim(
        familyId: String,
        accessToken: String,
        githubLogin: String,
        operationId: String,
    ): IssuedClaim {
        val root = post(
            "v1/contributor/claims",
            proof(familyId, accessToken)
                .put("github_login", githubLogin)
                .put("operation_id", operationId),
        )
        return try {
            IssuedClaim(
                claimId = uuid(root.getString("claim_id"), "claim"),
                code = root.getString("code").takeIf { it.isNotEmpty() } ?: throw invalid("code"),
            )
        } catch (e: ContributorException) {
            throw e
        } catch (e: Exception) {
            throw invalid("body")
        }
    }

    override suspend fun readClaims(familyId: String, accessToken: String): List<Claim> {
        val root = post("v1/contributor/claims/read", proof(familyId, accessToken))
        return items(root).map(::claim)
    }

    override suspend fun submitProof(
        familyId: String,
        accessToken: String,
        claimId: String,
        gistId: String,
    ): Claim {
        uuid(claimId, "claim")
        val root = post(
            "v1/contributor/claims/$claimId/proof",
            proof(familyId, accessToken).put("gist_id", gistId),
        )
        return claim(root)
    }

    override suspend fun withdrawClaim(familyId: String, accessToken: String, claimId: String): Claim {
        uuid(claimId, "claim")
        val root = post("v1/contributor/claims/$claimId/withdraw", proof(familyId, accessToken))
        return claim(root)
    }

    override suspend fun readProfile(familyId: String, accessToken: String): ProfileRead {
        val root = post("v1/contributor/profile/read", proof(familyId, accessToken))
        return profileRead(root)
    }

    override suspend fun upsertProfile(familyId: String, accessToken: String, edit: ProfileEdit): ProfileRead {
        val links = JSONArray()
        for (link in edit.links) {
            links.put(JSONObject().put("platform", link.platform).put("handle", link.handle))
        }
        val root = post(
            "v1/contributor/profile",
            proof(familyId, accessToken)
                .put("display_name", edit.displayName)
                .put("links", links)
                .put("visibility", edit.visibility.name)
                .put("public_consent", edit.publicConsent)
                .put("show_roles_publicly", edit.showRoles),
        )
        return profileRead(root)
    }

    private fun proof(familyId: String, accessToken: String): JSONObject {
        if (familyId.isBlank() || accessToken.isBlank()) throw ContributorException.Denied("contributors.signed-out")
        return JSONObject().put("family_id", familyId).put("access_token", accessToken)
    }

    private fun items(root: JSONObject): List<JSONObject> {
        val raw = root.optJSONArray("items") ?: throw invalid("items")
        if (raw.length() > MAX_ITEMS) throw invalid("items")
        return (0 until raw.length()).map { raw.getJSONObject(it) }
    }

    private fun claim(root: JSONObject): Claim {
        try {
            return Claim(
                id = uuid(root.getString("claim_id"), "claim"),
                githubLogin = root.getString("github_login"),
                state = ClaimState.parse(root.optString("state", "")),
                proof = ClaimProof.parse(root.optString("proof_status", "")),
                gistId = root.optString("gist_id", "").takeIf { it.isNotEmpty() },
                override = root.optBoolean("override", false),
                createdAt = root.optString("created_at", ""),
                expiresAt = root.optString("expires_at", ""),
                code = root.optString("code", "").takeIf { it.isNotEmpty() },
            )
        } catch (e: ContributorException) {
            throw e
        } catch (e: Exception) {
            throw invalid("body")
        }
    }

    private fun profileRead(root: JSONObject): ProfileRead {
        try {
            val raw = if (root.isNull("profile")) {
                null
            } else {
                val node = root.getJSONObject("profile")
                val linksRaw = node.optJSONArray("links") ?: JSONArray()
                val links = (0 until linksRaw.length()).mapNotNull { index ->
                    val link = linksRaw.optJSONObject(index) ?: return@mapNotNull null
                    val platform = link.optString("platform", "")
                    val handle = link.optString("handle", "")
                    val url = link.optString("url", "")
                    if (ContributorLinkHosts.hostFor(platform) == null) return@mapNotNull null
                    ContributorLink(platform, handle, url)
                }
                StoredProfile(
                    displayName = node.getString("display_name"),
                    links = links,
                    visibility = if (node.optString("visibility", "") == "PUBLIC") {
                        ProfileVisibility.PUBLIC
                    } else {
                        ProfileVisibility.HIDDEN
                    },
                    publicConsent = node.optBoolean("public_consent", false),
                    showRoles = node.optBoolean("show_roles_publicly", false),
                )
            }
            val rolesRaw = root.optJSONArray("roles") ?: JSONArray()
            val roles = (0 until rolesRaw.length()).mapNotNull { rolesRaw.optString(it, "").takeIf(String::isNotEmpty) }
            return ProfileRead(raw, roles)
        } catch (e: ContributorException) {
            throw e
        } catch (e: Exception) {
            throw invalid("body")
        }
    }

    private fun uuid(raw: String, part: String): String {
        try {
            if (UUID.fromString(raw).toString() != raw.lowercase()) throw invalid(part)
        } catch (e: ContributorException) {
            throw e
        } catch (_: Exception) {
            throw invalid(part)
        }
        return raw
    }

    private fun invalid(part: String): ContributorException =
        ContributorException.Invalid("contributors.invalid-$part")

    private suspend fun post(path: String, body: JSONObject): JSONObject {
        val url = origin.newBuilder().addPathSegments(path).build()
        return try {
            withContext(Dispatchers.IO) {
                suspendCancellableCoroutine { continuation ->
                    val request = Request.Builder().url(url)
                        .header("Accept", "application/json")
                        .header("Cache-Control", "no-store")
                        .post(body.toString().toByteArray(Charsets.UTF_8).toRequestBody(JSON)).build()
                    val call = client.newCall(request)
                    continuation.invokeOnCancellation { call.cancel() }
                    call.enqueue(object : Callback {
                        override fun onFailure(call: Call, e: IOException) {
                            if (continuation.isActive) {
                                continuation.resumeWithException(ContributorException.Offline("contributors.offline"))
                            }
                        }

                        override fun onResponse(call: Call, response: Response) {
                            response.use {
                                if (!continuation.isActive) return
                                try {
                                    when (response.code) {
                                        200, 201 -> Unit
                                        400 -> throw ContributorException.Invalid("contributor.invalid")
                                        401 -> throw ContributorException.Denied("contributor.auth-required")
                                        403 -> throw ContributorException.Forbidden("contributor.forbidden")
                                        404 -> throw ContributorException.Denied("contributor.not-found")
                                        409 -> throw ContributorException.Conflict(
                                            serverCode(response) ?: "contributor.conflict",
                                        )
                                        429 -> throw ContributorException.Quota("contributor.quota")
                                        else -> throw ContributorException.Unavailable("contributor.unavailable")
                                    }
                                    val source = response.body?.source()
                                        ?: throw ContributorException.Unavailable("contributors.empty")
                                    if (source.request(BODY_BUDGET)) {
                                        throw ContributorException.Invalid("contributors.over-budget")
                                    }
                                    continuation.resume(JSONObject(source.readUtf8()))
                                } catch (e: Exception) {
                                    if (continuation.isActive) continuation.resumeWithException(e)
                                }
                            }
                        }
                    })
                }
            }
        } catch (e: CancellationException) {
            throw e
        }
    }

    private fun serverCode(response: Response): String? {
        return try {
            val peeked = response.peekBody(BODY_BUDGET).string()
            JSONObject(peeked).optJSONObject("error")?.optString("code")?.takeIf { it.isNotEmpty() }
        } catch (_: Exception) {
            null
        }
    }

}
