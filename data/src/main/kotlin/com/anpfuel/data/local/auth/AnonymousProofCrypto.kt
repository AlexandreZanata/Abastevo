package com.anpfuel.data.local.auth

import com.anpfuel.domain.portable.PortableAnonymousProof
import java.math.BigInteger
import java.security.KeyFactory
import java.security.MessageDigest
import java.security.PublicKey
import java.security.Signature
import java.security.spec.ECParameterSpec
import java.security.spec.ECPoint
import java.security.spec.ECPublicKeySpec
import java.util.Base64

/**
 * P10-T03 JVM-testable anonymous-proof crypto.
 *
 * Mirrors the frozen backend profile (`docs/security/identity-profile.md`,
 * P03-T01): exact covered set/order, 5-minute window, `fp:<hex>` binding
 * to the verified key, raw `R || S` (64 bytes) signatures over the SHA-512
 * digest via `NONEwithECDSA`. DER blobs fail closed by length. Key custody
 * stays with the caller (AndroidKeyStore on device, generated EC keys in
 * tests); this object never stores keys.
 */
object AnonymousProofCrypto {

    /** P-256 field prime. */
    private val P: BigInteger = BigInteger(
        "ffffffff00000001000000000000000000000000ffffffffffffffffffffffff",
        16,
    )

    /** P-256 curve constant `b`. */
    private val B: BigInteger = BigInteger(
        "5ac635d8aa3a93e7b3ebbd55769886bc651d06b0cc53b0f63bce3c3e27d2604b",
        16,
    )

    fun b64urlEncode(bytes: ByteArray): String =
        Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)

    fun b64urlDecode(value: String): ByteArray? {
        if (value.isEmpty() || value.length > 8192) return null
        return try {
            Base64.getUrlDecoder().decode(value)
        } catch (_: Exception) {
            null
        }
    }

    fun sha256Hex(bytes: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(bytes)
        val out = StringBuilder(digest.size * 2)
        for (b in digest) {
            val v = b.toInt() and 0xFF
            if (v < 16) out.append('0')
            out.append(v.toString(16))
        }
        return out.toString()
    }

    fun sha512(bytes: ByteArray): ByteArray =
        MessageDigest.getInstance("SHA-512").digest(bytes)

    /** `sha-512=:<standard-base64>:` over the exact transmitted bytes. */
    fun contentDigestHeader(body: ByteArray): String =
        "sha-512=:" + Base64.getEncoder().encodeToString(sha512(body)) + ":"

    /** Fingerprint `fp:<hex>` for JWK coordinates, or null when refused. */
    fun fingerprint(jwkX: String, jwkY: String): String? {
        val x = b64urlDecode(jwkX) ?: return null
        val y = b64urlDecode(jwkY) ?: return null
        if (x.size != 32 || y.size != 32) return null
        if (!isOnP256(x, y)) return null
        val input = PortableAnonymousProof.thumbprintInput(jwkX, jwkY)
            .toByteArray(Charsets.UTF_8)
        return "fp:" + sha256Hex(input)
    }

    /** True when (x, y) is a non-degenerate P-256 point. */
    fun isOnP256(x: ByteArray, y: ByteArray): Boolean {
        if (x.size != 32 || y.size != 32) return false
        var allZero = true
        for (b in x + y) {
            if (b.toInt() != 0) {
                allZero = false
                break
            }
        }
        if (allZero) return false
        return try {
            val bigX = BigInteger(1, x)
            val bigY = BigInteger(1, y)
            if (bigX >= P || bigY >= P) return false
            // y^2 == x^3 - 3x + b (mod p).
            val lhs = bigY.modPow(BigInteger.TWO, P)
            val rhs = bigX.modPow(BigInteger("3"), P)
                .subtract(bigX.multiply(BigInteger("3")).mod(P))
                .add(B).mod(P)
            lhs == rhs
        } catch (_: Exception) {
            false
        }
    }

    /** Raw 64-byte `R || S` to DER. Returns null when malformed. */
    fun rawToDer(raw: ByteArray): ByteArray? {
        if (raw.size != 64) return null
        val r = trimLeadingZeroes(raw.copyOfRange(0, 32))
        val s = trimLeadingZeroes(raw.copyOfRange(32, 64))
        val rEnc = encodeDerInt(r)
        val sEnc = encodeDerInt(s)
        val total = rEnc.size + sEnc.size
        if (total > 127) return null
        return byteArrayOf(0x30.toByte(), total.toByte()) + rEnc + sEnc
    }

    /** DER to raw 64-byte `R || S`. Returns null when malformed. */
    fun derToRaw(der: ByteArray): ByteArray? {
        if (der.size < 8 || der.size > 72) return null
        if (der[0] != 0x30.toByte()) return null
        val total = der[1].toInt() and 0xFF
        if (total != der.size - 2) return null
        var pos = 2
        if (der[pos] != 0x02.toByte()) return null
        val rLen = der[pos + 1].toInt() and 0xFF
        pos += 2
        if (rLen <= 0 || rLen > 33 || pos + rLen > der.size) return null
        val r = der.copyOfRange(pos, pos + rLen)
        pos += rLen
        if (pos >= der.size || der[pos] != 0x02.toByte()) return null
        val sLen = der[pos + 1].toInt() and 0xFF
        pos += 2
        if (sLen <= 0 || sLen > 33 || pos + sLen != der.size) return null
        val s = der.copyOfRange(pos, pos + sLen)
        val rRaw = stripDerInt(r) ?: return null
        val sRaw = stripDerInt(s) ?: return null
        if (rRaw.size > 32 || sRaw.size > 32) return null
        val out = ByteArray(64)
        rRaw.copyInto(out, 32 - rRaw.size)
        sRaw.copyInto(out, 64 - sRaw.size)
        return out
    }

    /**
     * Verifies a frozen-profile proof. Returns true only when the covered
     * set/order, window, key binding and raw signature all pass; every
     * failure (including DER input, which is never 64 bytes) is false.
     */
    fun verify(
        jwkX: String,
        jwkY: String,
        lines: List<String>,
        signatureB64Url: String,
        nowEpochSeconds: Long,
    ): Boolean {
        if (!PortableAnonymousProof.isLinesShapeValid(lines)) return false
        val created = parseLongLine(lines, "\"created\":") ?: return false
        val expires = parseLongLine(lines, "\"expires\":") ?: return false
        if (!PortableAnonymousProof.isWindowValid(created, expires, nowEpochSeconds)) return false
        val keyId = parseQuotedLine(lines, "\"keyid\":") ?: return false
        if (!PortableAnonymousProof.isFingerprintShape(keyId)) return false
        val expected = fingerprint(jwkX, jwkY) ?: return false
        if (expected != keyId) return false
        val nonce = parseQuotedLine(lines, "\"nonce\":") ?: return false
        if (PortableAnonymousProof.splitNonce(nonce) == null) return false
        val raw = b64urlDecode(signatureB64Url) ?: return false
        if (raw.size != 64) return false
        val der = rawToDer(raw) ?: return false
        val publicKey = publicKey(jwkX, jwkY) ?: return false
        return try {
            val digest = sha512(PortableAnonymousProof.buildBase(lines).toByteArray(Charsets.UTF_8))
            val signature = Signature.getInstance("NONEwithECDSA")
            signature.initVerify(publicKey)
            signature.update(digest)
            signature.verify(der)
        } catch (_: Exception) {
            false
        }
    }

    private fun parseLongLine(lines: List<String>, prefix: String): Long? {
        val line = lines.firstOrNull { it.startsWith(prefix) } ?: return null
        val raw = line.substring(prefix.length).trim()
        if (raw.isEmpty() || raw.length > 20) return null
        val value = raw.toLongOrNull() ?: return null
        if (value.toString() != raw) return null
        return value
    }

    private fun parseQuotedLine(lines: List<String>, prefix: String): String? {
        val line = lines.firstOrNull { it.startsWith(prefix) } ?: return null
        val raw = line.substring(prefix.length).trim()
        if (raw.length < 2 || !raw.startsWith("\"") || !raw.endsWith("\"")) return null
        val inner = raw.substring(1, raw.length - 1)
        if (inner.contains('\r') || inner.contains('\n')) return null
        return inner
    }

    private fun publicKey(jwkX: String, jwkY: String): PublicKey? {
        return try {
            val x = b64urlDecode(jwkX) ?: return null
            val y = b64urlDecode(jwkY) ?: return null
            if (!isOnP256(x, y)) return null
            val params = p256Spec()
            val point = ECPoint(BigInteger(1, x), BigInteger(1, y))
            KeyFactory.getInstance("EC").generatePublic(ECPublicKeySpec(point, params))
        } catch (_: Exception) {
            null
        }
    }

    private fun p256Spec(): ECParameterSpec {
        val params = java.security.AlgorithmParameters.getInstance("EC")
        params.init(java.security.spec.ECGenParameterSpec("secp256r1"))
        return params.getParameterSpec(ECParameterSpec::class.java)
    }

    private fun trimLeadingZeroes(bytes: ByteArray): ByteArray {
        var start = 0
        while (start < bytes.size - 1 && bytes[start].toInt() == 0) start++
        return bytes.copyOfRange(start, bytes.size)
    }

    private fun encodeDerInt(value: ByteArray): ByteArray {
        var v = value
        var start = 0
        while (start < v.size - 1 && v[start].toInt() == 0) start++
        v = v.copyOfRange(start, v.size)
        val needsZero = (v[0].toInt() and 0x80) != 0
        val body = if (needsZero) byteArrayOf(0) + v else v
        return byteArrayOf(0x02.toByte(), body.size.toByte()) + body
    }

    private fun stripDerInt(encoded: ByteArray): ByteArray? {
        if (encoded.isEmpty()) return null
        var v = encoded
        if (v[0] == 0.toByte()) {
            if (v.size < 2 || (v[1].toInt() and 0x80) == 0) return null
            v = v.copyOfRange(1, v.size)
        }
        // Reject overlong encodings with excessive leading zeroes.
        if (v.size > 1 && v[0].toInt() == 0 && (v[1].toInt() and 0x80) == 0) return null
        return v
    }
}
