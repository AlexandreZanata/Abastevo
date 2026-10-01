package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * P10-T05 validated contribution draft (BUC-003, B-BR-003/005).
 *
 * One stable `clientSubmissionId` carries exactly one observation intent:
 * retries reuse the same id, corrections are a new id with
 * `supersedesObservationId`. Money is exact integer thousandths of a real
 * (no float); contributor/confidence/reputation are never accepted from
 * the client (server derives them). Photo is optional: metadata-only is
 * allowed with weaker confidence, a pending photo waits, never blocks
 * validation of the fact itself here.
 */
class ContributionDraft private constructor(
    val clientSubmissionId: String,
    val stationId: String,
    val fuelProduct: FuelProduct,
    val amountMilliBrl: Long,
    val currency: String,
    val unit: String,
    val conditionKind: String,
    val capturedAtMillis: Long,
    val photoId: String?,
    val supersedesObservationId: String?,
) {
    companion object {
        fun create(
            clientSubmissionId: String,
            stationId: String,
            fuelProduct: FuelProduct,
            amountMilliBrl: Long,
            currency: String,
            unit: String,
            conditionKind: String,
            capturedAtMillis: Long,
            photoId: String? = null,
            supersedesObservationId: String? = null,
        ): ContributionDraft {
            if (clientSubmissionId.isBlank()) {
                throw DomainException("client_submission_id is blank")
            }
            if (!isUuid(stationId)) {
                throw DomainException("malformed station_id")
            }
            if (amountMilliBrl !in 1..1_000_000) {
                throw DomainException("amount_milli_brl out of range")
            }
            if (currency != "BRL") {
                throw DomainException("currency must be BRL")
            }
            if (unit.isBlank()) {
                throw DomainException("unit is blank")
            }
            if (conditionKind.isBlank()) {
                throw DomainException("condition kind is blank")
            }
            if (capturedAtMillis <= 0) {
                throw DomainException("captured_at is missing")
            }
            if (photoId != null && photoId.isBlank()) {
                throw DomainException("photo_id is blank")
            }
            if (supersedesObservationId != null && supersedesObservationId.isBlank()) {
                throw DomainException("supersedes_observation_id is blank")
            }
            return ContributionDraft(
                clientSubmissionId = clientSubmissionId,
                stationId = stationId.lowercase(),
                fuelProduct = fuelProduct,
                amountMilliBrl = amountMilliBrl,
                currency = currency,
                unit = unit,
                conditionKind = conditionKind,
                capturedAtMillis = capturedAtMillis,
                photoId = photoId,
                supersedesObservationId = supersedesObservationId,
            )
        }

        internal fun isUuid(value: String): Boolean {
            if (value.length != 36) return false
            for ((index, char) in value.withIndex()) {
                when (index) {
                    8, 13, 18, 23 -> if (char != '-') return false
                    else -> if (!char.isDigit() && char.lowercaseChar() !in 'a'..'f') return false
                }
            }
            return true
        }
    }
}
