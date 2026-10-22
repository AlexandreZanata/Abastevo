package com.anpfuel.domain.premium

import java.math.BigDecimal
import java.math.RoundingMode

/**
 * MEX-12 gap/reset intents and estimated efficiency (PRM09-T05).
 *
 * Intents are validated owner-side values held in the outbox until the
 * sync transport applies them (MEX-14 maps them onto the backend gap and
 * segment operations). Estimated efficiency is a configured km/L used
 * when measured intervals are unknown; it never rewrites history.
 */

private val canonicalId = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
private fun checkIntentId(value: String, code: String): String {
    if (!canonicalId.matches(value)) throw PremiumValidationException(code)
    return value
}

/** Declares an odometer gap between two records of one vehicle. */
data class LedgerGapIntent(
    val afterRecordId: String,
    val beforeRecordId: String?,
    val note: String?,
) {
    init {
        checkIntentId(afterRecordId, "invalid-gap")
        if (beforeRecordId != null) checkIntentId(beforeRecordId, "invalid-gap")
        if (note != null && (note.isEmpty() || note.length > 280 || note.any { it.isISOControl() })) {
            throw PremiumValidationException("invalid-note")
        }
    }

    fun bodyJson(): String = buildString {
        append("{\"after_id\":\"$afterRecordId\"")
        if (beforeRecordId != null) append(",\"before_id\":\"$beforeRecordId\"")
        if (note != null) {
            val escaped = note.replace("\\", "\\\\").replace("\"", "\\\"")
            append(",\"note\":\"$escaped\"")
        }
        append("}")
    }
}

/** Opens a new odometer segment (cluster reset or sensor replacement). */
data class LedgerSegmentReset(val reason: String) {
    init {
        if (reason != "reset" && reason != "replacement") {
            throw PremiumValidationException("invalid-reset")
        }
    }

    fun bodyJson(): String = "{\"reason\":\"$reason\"}"
}

/**
 * EE-20 private charging session context: coarse home/work/public/other
 * label kept local-only with the draft. Never coordinates, never a
 * photo, never synced (the backend body carries no context), and never
 * auto-posted anywhere public.
 */
enum class LedgerSessionContext(val wire: String) {
    HOME("home"),
    WORK("work"),
    PUBLIC("public"),
    OTHER("other"),
    ;

    companion object {
        fun parse(wire: String): LedgerSessionContext =
            entries.firstOrNull { it.wire == wire }
                ?: throw PremiumValidationException("unknown-context")
    }
}

/**
 * Stable intent commands: same intent converges to one queued command
 * (process-death double-submit safe); a different note or reason is a
 * different intent. Held for the sync transport (MEX-14).
 */
object LedgerIntentCommandFactory {

    fun gapCommand(
        scope: PremiumOwnerScope,
        vehicleId: String,
        intent: LedgerGapIntent,
    ): PremiumCommand {
        val seed = "ledger-gap|${scope.accountId}|${scope.origin}|$vehicleId|" +
            "${intent.afterRecordId}|${intent.beforeRecordId}|${intent.note}"
        return PremiumCommand(
            operationId = java.util.UUID.nameUUIDFromBytes(seed.toByteArray(Charsets.UTF_8)).toString(),
            scope = scope,
            dataset = "ledger-gap",
            recordId = intent.afterRecordId,
            expectedRevision = 0,
            payloadJson = intent.bodyJson(),
        )
    }

    fun resetCommand(
        scope: PremiumOwnerScope,
        vehicleId: String,
        reset: LedgerSegmentReset,
    ): PremiumCommand {
        val seed = "ledger-reset|${scope.accountId}|${scope.origin}|$vehicleId|${reset.reason}"
        return PremiumCommand(
            operationId = java.util.UUID.nameUUIDFromBytes(seed.toByteArray(Charsets.UTF_8)).toString(),
            scope = scope,
            dataset = "ledger-reset",
            recordId = vehicleId,
            expectedRevision = 0,
            payloadJson = reset.bodyJson(),
        )
    }
}

/**
 * Configured km/L estimate for one vehicle, used only when measured
 * intervals are unknown. Localized comma or dot decimals; at most two
 * fraction digits; (0, 1000] range refuses absurd or non-positive input.
 */
class EstimatedEfficiency private constructor(val kmPerLitre: BigDecimal) {

    fun toPlainString(): String = kmPerLitre.stripTrailingZeros().toPlainString()

    override fun equals(other: Any?): Boolean =
        other is EstimatedEfficiency && kmPerLitre.compareTo(other.kmPerLitre) == 0

    override fun hashCode(): Int = kmPerLitre.hashCode()

    override fun toString(): String = "EstimatedEfficiency(kmPerLitre=$kmPerLitre)"

    companion object {
        fun of(input: String): EstimatedEfficiency {
            val parsed = try {
                BigDecimal(input.trim().replace(',', '.'))
            } catch (_: Exception) {
                throw PremiumValidationException("invalid-estimate")
            }
            if (parsed.signum() <= 0 || parsed > BigDecimal(1000) || parsed.scale() > 2) {
                throw PremiumValidationException("invalid-estimate")
            }
            return EstimatedEfficiency(parsed.setScale(2, RoundingMode.HALF_UP))
        }
    }
}
