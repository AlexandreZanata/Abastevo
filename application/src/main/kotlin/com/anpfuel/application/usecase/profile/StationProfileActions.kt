package com.anpfuel.application.usecase.profile

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth

/** Wire-bound claim. Private material is never stored in public profile/cache state. */
data class OwnedProfileClaim(
    val id: String,
    val stationId: String,
    val role: String,
    val scopes: Set<String>,
    val state: String,
    val declarationId: String,
    val declaration: String,
    val expiresAt: Long,
    val declarationState: String = "active",
) {
    val canSupplyProof: Boolean get() = declarationState == "active" && state in setOf("draft", "awaiting_proof", "needs_information")
}

interface ProfileClaimGateway {
    suspend fun open(session: PortableAuth.Session, stationId: String, role: String, scopes: Set<String>, key: String): OwnedProfileClaim
    suspend fun mine(session: PortableAuth.Session): List<String>
    suspend fun status(session: PortableAuth.Session, claimId: String): OwnedProfileClaim
    suspend fun reissue(session: PortableAuth.Session, claimId: String): OwnedProfileClaim
    suspend fun cancel(session: PortableAuth.Session, claimId: String)
    suspend fun submit(session: PortableAuth.Session, claim: OwnedProfileClaim, bytes: ByteArray)
    suspend fun edit(session: PortableAuth.Session, stationId: String, revision: Int, fields: Map<String, String>)
    suspend fun reply(session: PortableAuth.Session, stationId: String, product: String, text: String)
}

class ProfileSignInRequired : IllegalStateException()
class ProfileInputInvalid : IllegalArgumentException()

/** Every action reloads account/session; cached approval never grants a command. */
class StationProfileActions(
    private val gateway: ProfileClaimGateway,
    private val sessions: AuthSessionStore,
    private val now: () -> Long,
) {
    private fun session(): PortableAuth.Session = sessions.load()?.takeIf {
        it.accountId.isNotBlank() && PortableAuth.isAccessLive(it, now())
    } ?: throw ProfileSignInRequired()

    suspend fun mine(stationId: String): List<OwnedProfileClaim> {
        val current = session()
        return gateway.mine(current).map { gateway.status(current, it) }.filter { it.stationId == stationId }
    }

    suspend fun open(stationId: String, role: String, scopes: Set<String>, key: String): OwnedProfileClaim {
        val current = session()
        val permitted = if (role == "administrator") setOf("profile.edit", "reply.official", "invite.propose", "revoke.request")
            else if (role == "manager") setOf("profile.edit", "reply.official") else emptySet()
        if (stationId.isBlank() || key.isBlank() || scopes.isEmpty() || !permitted.containsAll(scopes)) throw ProfileInputInvalid()
        return gateway.open(current, stationId, role, scopes, key)
    }

    suspend fun status(id: String): OwnedProfileClaim = gateway.status(session(), id)
    suspend fun reissue(id: String): OwnedProfileClaim = gateway.reissue(session(), id)
    suspend fun cancel(id: String) = gateway.cancel(session(), id)

    suspend fun submit(claim: OwnedProfileClaim, bytes: ByteArray) {
        val current = session()
        // Re-read owner status before touching proof transport (expiry/reissue/revocation).
        val live = gateway.status(current, claim.id)
        if (!live.canSupplyProof || live.expiresAt <= now() || live.declarationId != claim.declarationId ||
            bytes.isEmpty() || bytes.size > 5 * 1024 * 1024 || !bytes.take(5).toByteArray().contentEquals("%PDF-".toByteArray())
        ) throw ProfileInputInvalid()
        if (session().accountId != current.accountId) throw ProfileSignInRequired()
        gateway.submit(current, live, bytes)
    }

    suspend fun edit(stationId: String, revision: Int, fields: Map<String, String>) {
        val current = session()
        if (revision < 1 || fields.isEmpty() || !com.anpfuel.domain.profile.PUBLIC_BUSINESS_KEYS.containsAll(fields.keys)) throw ProfileInputInvalid()
        gateway.edit(current, stationId, revision, fields)
    }

    suspend fun reply(stationId: String, product: String, text: String) {
        val current = session()
        if (text.isBlank() || text.codePointCount(0, text.length) > 280) throw ProfileInputInvalid()
        gateway.reply(current, stationId, product, text)
    }
}
