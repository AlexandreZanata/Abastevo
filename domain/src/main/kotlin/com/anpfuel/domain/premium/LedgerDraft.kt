package com.anpfuel.domain.premium

import java.nio.charset.StandardCharsets
import java.time.Instant
import java.time.ZoneId
import java.util.UUID

/**
 * MEX-11 ledger drafts and exact command bodies (PRM09-T01/T05).
 *
 * Drafts are validated owner-side values for the backend ledger
 * (`refuel|expense|charge`): bounds mirror the backend engine, fuel and
 * unit travel together, tariff references stay atomic, nulls stay out of
 * the wire (never zero-filled). Bodies use a fixed key order so
 * operation IDs are reproducible and double-saves converge.
 */
enum class LedgerFill(val wire: String) {
    FULL("full"),
    PARTIAL("partial"),
    UNKNOWN("unknown"),
}

private val canonicalId = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
private const val MAX_TARIFF_TEXT = 128
private const val MAX_UNIT_PRICE_MILLI = 1_000_000L
private const val MAX_CHARGE_WH = 1_000_000_000L
private const val MAX_CHARGE_ODOMETER = 1_000_000_000L

sealed interface LedgerDraft {
    val dataset: LedgerDataset
    val recordId: String
    fun bodyJson(): String
}

private fun checkId(value: String, code: String): String {
    if (!canonicalId.matches(value)) throw PremiumValidationException(code)
    return value
}

private fun checkTimezone(value: String): String {
    if (value.isEmpty() || value.length > 64) throw PremiumValidationException("invalid-timezone")
    try {
        ZoneId.of(value)
    } catch (_: Exception) {
        throw PremiumValidationException("invalid-timezone")
    }
    return value
}

private fun checkNoteText(note: String?) {
    // Backend parity: notes stay plain text (no control runes); length
    // follows the shared premium bound.
    if (note != null && note.any { it.isISOControl() }) {
        throw PremiumValidationException("invalid-note")
    }
    PremiumBounds.checkNote(note)
}

private fun checkOdometer(value: Long?, code: String = "input-out-of-bounds"): Long? {
    if (value == null) return null
    if (value < 0 || value > PremiumBounds.MAX_DISTANCE_METRES) throw PremiumValidationException(code)
    return value
}

private fun escapeJson(value: String): String {
    val out = StringBuilder()
    for (c in value) {
        when (c) {
            '"' -> out.append("\\\"")
            '\\' -> out.append("\\\\")
            else -> if (c < ' ') out.append("\\u%04x".format(c.code)) else out.append(c)
        }
    }
    return out.toString()
}

private fun optText(key: String, value: String?): String =
    if (value == null) "" else ",\"$key\":\"${escapeJson(value)}\""

private fun optLong(key: String, value: Long?): String =
    if (value == null) "" else ",\"$key\":$value"

private fun optInt(key: String, value: Int?): String =
    if (value == null) "" else ",\"$key\":$value"

data class RefuelDraft(
    val id: String,
    val vehicleId: String,
    val occurredAt: Instant,
    val timezone: String,
    val fuel: PremiumFuel,
    val quantityMilli: Long,
    val totalCentavos: Long,
    val unitPriceMilli: Long?,
    val odometerMetres: Long?,
    val fill: LedgerFill,
    val stationId: String?,
    val note: String?,
) : LedgerDraft {
    override val dataset = LedgerDataset.REFUEL
    override val recordId = id

    init {
        checkId(id, "invalid-id")
        checkId(vehicleId, "invalid-vehicle")
        checkTimezone(timezone)
        PremiumBounds.checkQuantityMilli(quantityMilli)
        PremiumBounds.checkTotalCentavos(totalCentavos)
        if (unitPriceMilli != null && (unitPriceMilli < 0 || unitPriceMilli > MAX_UNIT_PRICE_MILLI)) {
            throw PremiumValidationException("input-out-of-bounds")
        }
        checkOdometer(odometerMetres)
        if (stationId != null) checkId(stationId, "invalid-station")
        checkNoteText(note)
    }

    override fun bodyJson(): String = buildString {
        append("{\"id\":\"$id\",\"vehicle_id\":\"$vehicleId\",\"occurred_at\":\"$occurredAt\",")
        append("\"timezone\":\"${escapeJson(timezone)}\",\"fuel\":\"${fuel.name}\",\"unit\":\"${fuel.unit.wire}\",")
        append("\"quantity_milli\":$quantityMilli,\"total_centavos\":$totalCentavos")
        append(optLong("unit_price_milli", unitPriceMilli))
        append(optLong("odometer_metres", odometerMetres))
        append(",\"fill\":\"${fill.wire}\"")
        if (stationId != null) append(",\"station_id\":\"$stationId\"")
        append(optText("note", note))
        append("}")
    }
}

data class ExpenseDraft(
    val id: String,
    val vehicleId: String,
    val occurredAt: Instant,
    val timezone: String,
    val amountCentavos: Long,
    val category: ExpenseCategory,
    val odometerMetres: Long?,
    val note: String?,
) : LedgerDraft {
    override val dataset = LedgerDataset.EXPENSE
    override val recordId = id

    init {
        checkId(id, "invalid-id")
        checkId(vehicleId, "invalid-vehicle")
        checkTimezone(timezone)
        PremiumBounds.checkTotalCentavos(amountCentavos)
        checkOdometer(odometerMetres)
        checkNoteText(note)
    }

    override fun bodyJson(): String = buildString {
        append("{\"id\":\"$id\",\"vehicle_id\":\"$vehicleId\",\"occurred_at\":\"$occurredAt\",")
        append("\"timezone\":\"${escapeJson(timezone)}\",\"amount_centavos\":$amountCentavos,")
        append("\"category\":\"${category.wire}\"")
        append(optLong("odometer_metres", odometerMetres))
        append(optText("note", note))
        append("}")
    }
}

data class ChargeDraft(
    val id: String,
    val vehicleId: String,
    val occurredAt: Instant,
    val timezone: String,
    val energyWh: Long?,
    val totalCentavos: Long?,
    val socStart: Int?,
    val socEnd: Int?,
    val odometerMetres: Long?,
    val feesCentavos: Long?,
    val tariffId: String?,
    val tariffRevision: String?,
    val note: String?,
) : LedgerDraft {
    override val dataset = LedgerDataset.CHARGE
    override val recordId = id

    init {
        checkId(id, "invalid-id")
        checkId(vehicleId, "invalid-vehicle")
        checkTimezone(timezone)
        if (energyWh != null && (energyWh < 0 || energyWh > MAX_CHARGE_WH)) {
            throw PremiumValidationException("input-out-of-bounds")
        }
        if (totalCentavos != null) PremiumBounds.checkTotalCentavos(totalCentavos)
        if (feesCentavos != null) PremiumBounds.checkTotalCentavos(feesCentavos)
        for (soc in listOf(socStart, socEnd)) {
            if (soc != null && (soc < 0 || soc > 100)) throw PremiumValidationException("input-out-of-bounds")
        }
        if (odometerMetres != null && (odometerMetres < 0 || odometerMetres > MAX_CHARGE_ODOMETER)) {
            throw PremiumValidationException("input-out-of-bounds")
        }
        if ((tariffId == null) != (tariffRevision == null)) {
            throw PremiumValidationException("invalid-tariff")
        }
        if (tariffId != null) checkId(tariffId, "invalid-tariff")
        if (tariffRevision != null && (tariffRevision.isEmpty() || tariffRevision.length > MAX_TARIFF_TEXT)) {
            throw PremiumValidationException("invalid-tariff")
        }
        checkNoteText(note)
    }

    override fun bodyJson(): String = buildString {
        append("{\"id\":\"$id\",\"vehicle_id\":\"$vehicleId\",\"occurred_at\":\"$occurredAt\",")
        append("\"timezone\":\"${escapeJson(timezone)}\"")
        append(optLong("energy_wh", energyWh))
        append(optLong("total_centavos", totalCentavos))
        append(optInt("soc_start_percent", socStart))
        append(optInt("soc_end_percent", socEnd))
        append(optLong("odometer_metres", odometerMetres))
        append(optLong("fees_centavos", feesCentavos))
        if (tariffId != null) append(",\"tariff_id\":\"$tariffId\"")
        append(optText("tariff_revision", tariffRevision))
        append(optText("note", note))
        append("}")
    }
}

/**
 * Exact command factory: one draft plus expected revision becomes one
 * dispatchable [PremiumCommand]. The operation ID is a deterministic
 * function of owner, dataset, record, revision and body, so double-saves
 * converge to one command and lost acknowledgements retry with the same
 * ID (the backend replays by operation ID instead of duplicating).
 */
object LedgerCommandFactory {

    fun toCommand(
        scope: PremiumOwnerScope,
        draft: LedgerDraft,
        expectedRevision: Long,
    ): PremiumCommand {
        if (expectedRevision < 0) throw PremiumValidationException("invalid-revision")
        val body = draft.bodyJson()
        val seed = "ledger-command|${scope.accountId}|${scope.origin}|" +
            "${draft.dataset.wire}|${draft.recordId}|$expectedRevision|$body"
        val operationId = UUID.nameUUIDFromBytes(seed.toByteArray(StandardCharsets.UTF_8)).toString()
        return PremiumCommand(
            operationId = operationId,
            scope = scope,
            dataset = draft.dataset.wire,
            recordId = draft.recordId,
            expectedRevision = expectedRevision,
            payloadJson = body,
        )
    }
}
