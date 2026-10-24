package com.anpfuel.domain.charging

import java.time.LocalDate

/** EE-22 condition vocabulary, exactly the backend enum. */
enum class ChargingCondition { STANDARD, OFF_PEAK, PEAK, MEMBER, UNKNOWN }

enum class ChargingVote { HELPFUL, NOT_HELPFUL }

enum class ChargingReportReason { INACCURATE, SPAM, PRIVATE_DATA, OTHER }

/**
 * One signed community EV observation draft. No GPS, route, home context,
 * photo or source key is representable here; the wire carries only the
 * observed place/connector/price/condition/day/note.
 */
data class ChargingObservationDraft(
    val placeId: String?,
    val connector: ChargingConnector,
    val priceMilliBrlPerKwh: Long?,
    val condition: ChargingCondition,
    val observedDay: String,
    val note: String,
) {
    init {
        if (placeId != null) require(CANONICAL_ID.matches(placeId))
        require(priceMilliBrlPerKwh == null || priceMilliBrlPerKwh in 1..10_000_000)
        require(observedDay.matches(DAY)) { "invalid day" }
        LocalDate.parse(observedDay)
        require(note.length <= 1024 && note.none { it.isISOControl() })
    }

    private companion object {
        val CANONICAL_ID = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
        val DAY = Regex("[0-9]{4}-[0-9]{2}-[0-9]{2}")
    }
}

data class CommunityChargingObservation(
    val id: String,
    val connector: ChargingConnector,
    val priceMilliBrlPerKwh: Long?,
    val condition: ChargingCondition,
    val observedDay: String,
    val note: String,
    val helpful: Long,
    val notHelpful: Long,
)

data class OwnedChargingObservation(
    val id: String,
    val placeId: String?,
    val connector: ChargingConnector,
    val priceMilliBrlPerKwh: Long?,
    val condition: ChargingCondition,
    val observedDay: String,
    val note: String,
    val revision: Long,
)

data class ChargingTally(val helpful: Long, val notHelpful: Long)

sealed class ChargingObservationException(message: String, cause: Throwable? = null) : Exception(message, cause) {
    class SignedOut : ChargingObservationException("charging.signed-out")
    class Denied(message: String) : ChargingObservationException(message)
    class Conflict(message: String) : ChargingObservationException(message)
    class Invalid(message: String, cause: Throwable? = null) : ChargingObservationException(message, cause)
    class Unavailable(message: String, cause: Throwable? = null) : ChargingObservationException(message, cause)
}

interface ChargingObservationGateway {
    suspend fun history(placeId: String, limit: Int): List<CommunityChargingObservation>
    suspend fun mine(familyId: String, accessToken: String, limit: Int): List<OwnedChargingObservation>
    suspend fun submit(familyId: String, accessToken: String, id: String, draft: ChargingObservationDraft): String
    suspend fun correct(
        familyId: String,
        accessToken: String,
        id: String,
        expectedRevision: Long,
        draft: ChargingObservationDraft,
    ): Long
    suspend fun withdraw(familyId: String, accessToken: String, id: String, expectedRevision: Long): Long
    suspend fun vote(
        familyId: String,
        accessToken: String,
        observationId: String,
        choice: ChargingVote,
        submissionId: String,
    ): ChargingTally
    suspend fun report(
        familyId: String,
        accessToken: String,
        observationId: String,
        reason: ChargingReportReason,
        detail: String,
    )
}
