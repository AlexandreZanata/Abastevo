package com.anpfuel.application.usecase.identity

import com.anpfuel.application.port.AnonymousContributionFlagProvider
import com.anpfuel.application.port.AnonymousDeviceKeyPort
import com.anpfuel.domain.portable.PortableAnonymousProof

/**
 * P10-T03 — Anonymous device-key flow with the frozen proof profile.
 *
 * Flag disabled: [Outcome.Disabled], caller keeps browsing/ANP paths.
 * Flag enabled + no key: [Outcome.MissingKey] with the honest loss notice.
 * Flag enabled + key: [Outcome.Ready] carries only public coordinates and
 * the fingerprint; the private key never leaves the data adapter.
 *
 * Replay defense: each bound nonce spends exactly once client-side, so a
 * blind retry cannot loop forever. The server still enforces TTL, replay
 * and key binding; these checks are hints, never proof.
 */
sealed interface AnonymousDeviceOutcome {
    data object Disabled : AnonymousDeviceOutcome
    data class MissingKey(val lossNotice: String) : AnonymousDeviceOutcome
    data class Ready(
        val fingerprint: String,
        val jwkX: String,
        val jwkY: String,
    ) : AnonymousDeviceOutcome
}

class AnonymousDeviceFlow(
    private val flagProvider: AnonymousContributionFlagProvider,
    private val keys: AnonymousDeviceKeyPort,
    private val nowEpochSeconds: () -> Long = { System.currentTimeMillis() / 1000L },
    private val clientNonce: () -> String = { java.util.UUID.randomUUID().toString() },
) {
    fun currentIdentity(): AnonymousDeviceOutcome {
        if (!flagProvider.isEnabled()) return AnonymousDeviceOutcome.Disabled
        val fingerprint = keys.fingerprint() ?: return AnonymousDeviceOutcome.MissingKey(
            PortableAnonymousProof.LOST_KEY_NOTICE,
        )
        val jwk = keys.publicJwk() ?: return AnonymousDeviceOutcome.MissingKey(
            PortableAnonymousProof.LOST_KEY_NOTICE,
        )
        if (!PortableAnonymousProof.isFingerprintShape(fingerprint)) {
            return AnonymousDeviceOutcome.MissingKey(PortableAnonymousProof.LOST_KEY_NOTICE)
        }
        return AnonymousDeviceOutcome.Ready(fingerprint, jwk.first, jwk.second)
    }

    /**
     * Binds a server challenge to a fresh client nonce:
     * `<challenge-id>.<client-nonce>`. Returns null when the challenge id
     * is blank or the nonce source is empty (fails closed, never sent).
     */
    fun bindNonce(challengeId: String): String? {
        if (challengeId.isBlank()) return null
        val nonce = clientNonce()
        if (nonce.isBlank()) return null
        val bound = "$challengeId.$nonce"
        if (PortableAnonymousProof.splitNonce(bound) == null) return null
        return bound
    }

    /**
     * Builds the registration base lines for the actual POST path/query
     * and configured authority. Returns null when the identity is not
     * ready or the window/nonce is malformed (nothing is signed).
     */
    fun registrationLines(
        method: String,
        authority: String,
        path: String,
        query: String,
        challengeId: String,
        created: Long,
        expires: Long,
    ): List<String>? {
        val ready = currentIdentity() as? AnonymousDeviceOutcome.Ready ?: return null
        val bound = bindNonce(challengeId) ?: return null
        if (spentNonces.contains(bound)) return null
        if (!PortableAnonymousProof.isWindowValid(created, expires, nowEpochSeconds())) return null
        val lines = PortableAnonymousProof.buildBaseLines(
            method = method,
            authority = authority,
            path = path,
            query = query,
            contentType = null,
            contentDigest = null,
            created = created,
            expires = expires,
            keyId = ready.fingerprint,
            nonce = bound,
        )
        if (!PortableAnonymousProof.isLinesShapeValid(lines)) return null
        spentNonces.add(bound)
        return lines
    }

    /**
     * Canonical rotation intent both proofs sign. Returns null when either
     * challenge id or the new key coordinates are blank.
     */
    fun rotationIntent(
        newJwkX: String,
        newJwkY: String,
        oldChallengeId: String,
        newChallengeId: String,
    ): String? {
        if (newJwkX.isBlank() || newJwkY.isBlank()) return null
        if (oldChallengeId.isBlank() || newChallengeId.isBlank()) return null
        return PortableAnonymousProof.rotationIntentJson(
            newJwkX = newJwkX,
            newJwkY = newJwkY,
            oldChallengeId = oldChallengeId,
            newChallengeId = newChallengeId,
        )
    }

    /** Honest loss path: forgets the local reference; identity is unrecoverable. */
    fun markKeyLost(): String {
        keys.forgetKey()
        return PortableAnonymousProof.LOST_KEY_NOTICE
    }

    private val spentNonces: MutableSet<String> = mutableSetOf()
}
