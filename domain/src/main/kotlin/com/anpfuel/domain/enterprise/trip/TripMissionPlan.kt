package com.anpfuel.domain.enterprise.trip

/** The organization's approved customer or project reference of a plan: one of its cost centres (EMP-TRIP-009, EDR-247). */
class CostCentreRef(organizationId: String, id: String) : ScopedTripRef(organizationId, id)

/** The closed load vocabulary of a plan. */
enum class TripMissionLoad {
    NONE, CARGO, PASSENGERS;

    companion object {
        /** Exact tokens only; anything else is INVALID. */
        fun parse(raw: String): TripMissionLoad = entries.firstOrNull { it.name == raw } ?: throw TripMissionException()
    }
}

/** The plan text profile: the METER-001 profile and no standalone CPF-shaped token (B-BR-EMP-382). */
object TripMissionPlanText {
    /** The kernel.HasCPFShapedToken shape: 11 digits or NNN.NNN.NNN-NN, not touching an ASCII letter or digit. */
    private val cpfShapedToken = Regex("(^|[^0-9A-Za-z])([0-9]{11}|[0-9]{3}\\.[0-9]{3}\\.[0-9]{3}-[0-9]{2})([^0-9A-Za-z]|$)")

    fun validate(raw: String) {
        TripMissionText.validate(raw)
        if (cpfShapedToken.containsMatchIn(raw)) throw TripMissionException()
    }
}

/** A plan as the planner submitted it; the load is the raw token. */
data class TripMissionPlanInput(
    val destinationText: String? = null,
    val destinationUnit: UnitRef? = null,
    val reason: String = "",
    val costCentre: CostCentreRef? = null,
    val load: String = "",
    val cargoDescription: String? = null,
    val passengers: Int? = null,
) {
    override fun toString(): String = "EnterpriseTripMissionPlanInput(redacted)"
}

/** One validated planning revision's content (B-BR-EMP-382); exactly one destination is set. */
class TripMissionPlan private constructor(
    val destinationText: String?,
    val destinationUnit: UnitRef?,
    val reason: String,
    val costCentre: CostCentreRef?,
    val load: TripMissionLoad,
    val cargoDescription: String?,
    val passengers: Int?,
) {
    override fun toString(): String = "EnterpriseTripMissionPlan(redacted)"

    companion object {
        const val MAX_PASSENGERS: Int = 999

        /**
         * Validates a plan in the scope, first failure first: the scope; exactly one destination (the unit reference,
         * else the text profile); the reason; the optional cost centre; the load token and its only field (NONE none,
         * CARGO a description, PASSENGERS a count 1..999).
         */
        fun create(scope: String, input: TripMissionPlanInput): TripMissionPlan {
            requireMissionScope(scope)
            if ((input.destinationText == null) == (input.destinationUnit == null)) throw TripMissionException()
            if (input.destinationUnit != null) ownedByScope(scope, input.destinationUnit) else TripMissionPlanText.validate(input.destinationText!!)
            TripMissionPlanText.validate(input.reason)
            if (input.costCentre != null) ownedByScope(scope, input.costCentre)
            val load = TripMissionLoad.parse(input.load)
            val description = input.cargoDescription
            val count = input.passengers
            when (load) {
                TripMissionLoad.NONE -> if (description != null || count != null) throw TripMissionException()
                TripMissionLoad.CARGO -> {
                    if (description == null || count != null) throw TripMissionException()
                    TripMissionPlanText.validate(description)
                }
                TripMissionLoad.PASSENGERS ->
                    if (description != null || count == null || count < 1 || count > MAX_PASSENGERS) throw TripMissionException()
            }
            return TripMissionPlan(input.destinationText, input.destinationUnit, input.reason, input.costCentre, load, description, count)
        }
    }
}
