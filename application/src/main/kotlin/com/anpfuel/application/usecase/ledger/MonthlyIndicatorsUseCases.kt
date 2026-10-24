package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.port.EstimatedEfficiencyStore
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.premium.ChargeDraft
import com.anpfuel.domain.premium.EstimatedEfficiency
import com.anpfuel.domain.premium.ExpenseCategory
import com.anpfuel.domain.premium.ExpenseDraft
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerFill
import com.anpfuel.domain.premium.PremiumBounds
import com.anpfuel.domain.premium.PremiumConsumption
import com.anpfuel.domain.premium.PremiumCostResult
import com.anpfuel.domain.premium.PremiumConsumptionResult
import com.anpfuel.domain.premium.PremiumFill
import com.anpfuel.domain.premium.PremiumFuel
import com.anpfuel.domain.premium.PremiumInterval
import com.anpfuel.domain.premium.PremiumValidationException
import com.anpfuel.domain.premium.RefuelDraft
import com.anpfuel.domain.repository.LedgerDraftStore
import com.anpfuel.domain.repository.PremiumCommandStore
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId

data class MonthlyWindowNote(
    val recordCount: Int,
    val anchorFills: Int,
    val hasMissingOdometer: Boolean,
    val distanceMetres: Long,
    val hasMixedCarrier: Boolean,
)

data class MonthlyVehicleIndicators(
    val vehicleId: DomainId,
    val vehicleName: String,
    val year: Int,
    val month: Int,
    val timezone: String,
    val purchaseCentavos: Long,
    val expenseCentavos: Long,
    val chargeCostCentavos: Long,
    val chargeEnergyWh: Long,
    val chargeFeesCentavos: Long,
    val byCategory: Map<ExpenseCategory, Long>,
    val recordCount: Int,
    val measured: PremiumConsumptionResult,
    val costPerKm: String,
    val costPerKmMethod: String,
    val combinedCentavosPerKm: String,
    val combinedMethod: String,
    val combinedUnknown: String,
    val estimatedKmPerL: String?,
    val comparisonChangePercent: String,
    val comparisonUnknown: String,
    val energyMeasured: com.anpfuel.domain.premium.PremiumEnergyResult = com.anpfuel.domain.premium.PremiumEnergyResult(),
    val energyReference: com.anpfuel.domain.premium.PremiumEnergyResult = com.anpfuel.domain.premium.PremiumEnergyResult(),
    val energyComparison: com.anpfuel.domain.premium.PremiumEnergyComparison = com.anpfuel.domain.premium.PremiumEnergyComparison(),
    val windowNote: MonthlyWindowNote,
)

/**
 * MEX-13 monthly indicators over local drafts (PRM09-T06 screens).
 *
 * Month bounds follow the caller's timezone; measured intervals reuse
 * the exact full-to-full engine on in-month fills (first fill anchors
 * the window); a queued gap touching the month forces
 * declared-fill-gap; PHEV months share one distance (per-carrier
 * ratios refused, combined money/km exact); estimated km/L fills only
 * unknown measured windows; comparison always runs against the previous
 * month (empty baselines stay unknown, never zero-filled). No caching:
 * corrections recompute from the current store.
 */
class GetMonthlyIndicatorsUseCase(
    private val drafts: LedgerDraftStore,
    private val vehicles: VehicleRepository,
    private val efficiency: EstimatedEfficiencyStore,
    private val commands: PremiumCommandStore,
    private val sessions: AuthSessionStore,
    private val origin: String,
) {

    suspend operator fun invoke(vehicleId: DomainId?, year: Int, month: Int, timezone: String): List<MonthlyVehicleIndicators> {
        val zone = try {
            ZoneId.of(timezone)
        } catch (_: Exception) {
            throw PremiumValidationException("invalid-timezone")
        }
        if (year < 2000 || year > 2100 || month !in 1..12) throw PremiumValidationException("invalid-month")
        val accountId = sessions.load()?.accountId ?: "local"
        val scope = com.anpfuel.domain.premium.PremiumOwnerScope(accountId, origin)
        val all = drafts.list(accountId, origin)
        val queuedGaps = try {
            commands.listPending(scope, 50)
        } catch (_: Exception) {
            emptyList()
        }.filter { it.dataset == "ledger-gap" }
        val targets = if (vehicleId == null) {
            vehicles.listAll()
        } else {
            listOfNotNull(vehicles.findById(vehicleId))
        }
        return targets.map { vehicle ->
            indicators(vehicle, year, month, zone, all, queuedGaps)
        }
    }

    private fun indicators(
        vehicle: Vehicle,
        year: Int,
        month: Int,
        zone: ZoneId,
        all: List<com.anpfuel.domain.repository.StoredLedgerDraft>,
        queuedGaps: List<com.anpfuel.domain.premium.PremiumCommand>,
    ): MonthlyVehicleIndicators {
        val start = LocalDate.of(year, month, 1).atStartOfDay(zone).toInstant()
        val end = if (month == 12) LocalDate.of(year + 1, 1, 1) else LocalDate.of(year, month + 1, 1)
        val endInstant = end.atStartOfDay(zone).toInstant()
        val rows = all.filter { it.draft.let { d -> rowVehicle(d) == vehicle.id.value } }
            .filter { val at = rowOccurred(it.draft); !at.isBefore(start) && at.isBefore(endInstant) }
        return build(vehicle, year, month, zone.id, rows, queuedGaps, all, zone)
    }

    private fun build(
        vehicle: Vehicle,
        year: Int,
        month: Int,
        timezone: String,
        rows: List<com.anpfuel.domain.repository.StoredLedgerDraft>,
        queuedGaps: List<com.anpfuel.domain.premium.PremiumCommand>,
        all: List<com.anpfuel.domain.repository.StoredLedgerDraft>,
        zone: ZoneId,
    ): MonthlyVehicleIndicators {
        var purchase = 0L
        var expense = 0L
        var chargeCost = 0L
        var chargeEnergy = 0L
        var chargeFees = 0L
        val byCategory = mutableMapOf<ExpenseCategory, Long>()
        val fills = mutableListOf<Pair<java.time.Instant, RefuelDraft>>()
        val chargeOdos = mutableListOf<Pair<java.time.Instant, Long>>()
        var energyStated = 0
        var chargeSessions = 0
        var socBad = false
        var missingOdo = false
        var energyOdoMissing = false
        for (stored in rows) {
            when (val d = stored.draft) {
                is RefuelDraft -> {
                    purchase = checkedAdd(purchase, d.totalCentavos)
                        ?: throw PremiumValidationException("input-out-of-bounds")
                    fills.add(d.occurredAt to d)
                    if (d.odometerMetres == null) missingOdo = true
                }
                is ExpenseDraft -> {
                    expense = checkedAdd(expense, d.amountCentavos)
                        ?: throw PremiumValidationException("input-out-of-bounds")
                    byCategory[d.category] = checkedAdd(byCategory[d.category] ?: 0L, d.amountCentavos)
                        ?: throw PremiumValidationException("input-out-of-bounds")
                }
                is ChargeDraft -> {
                    chargeSessions++
                    val energyWh = d.energyWh
                    if (energyWh != null) {
                        energyStated++
                        chargeEnergy = checkedAdd(chargeEnergy, energyWh)
                            ?: throw PremiumValidationException("input-out-of-bounds")
                        if (d.odometerMetres == null) {
                            missingOdo = true
                            energyOdoMissing = true
                        }
                    }
                    val socStart = d.socStart
                    val socEnd = d.socEnd
                    if (socStart != null && socEnd != null && socEnd < socStart) socBad = true
                    val chargeOdo = d.odometerMetres
                    if (chargeOdo != null) chargeOdos.add(d.occurredAt to chargeOdo)
                    val chargeTotal = d.totalCentavos
                    if (chargeTotal != null) {
                        chargeCost = checkedAdd(chargeCost, chargeTotal)
                            ?: throw PremiumValidationException("input-out-of-bounds")
                    }
                    val fees = d.feesCentavos
                    if (fees != null) {
                        chargeFees = checkedAdd(chargeFees, fees)
                            ?: throw PremiumValidationException("input-out-of-bounds")
                    }
                }
            }
        }
        val hasMixedCarrier = fills.isNotEmpty() && energyStated > 0
        val recordIds = rows.map { it.draft.recordId }.toSet()
        val gapHit = queuedGaps.any { gap ->
            val payload = gap.payloadJson
            recordIds.any { id -> payload.contains(id) }
        }
        val ordered = fills.sortedBy { it.first }
        val odos = ordered.mapNotNull { it.second.odometerMetres }
        val sharedOdos = (ordered.map { it.first to it.second.odometerMetres } + chargeOdos)
            .sortedBy { it.first }.mapNotNull { it.second }
        val distance = if (sharedOdos.size >= 2) sharedOdos.last() - sharedOdos.first() else 0L
        val measured = if (fills.isEmpty()) {
            PremiumConsumptionResult(unknown = "no-records")
        } else if (hasMixedCarrier) {
            PremiumConsumptionResult(unknown = "phev-shared-distance")
        } else {
            val interval = PremiumInterval(
                startingFillExcluded = ordered.first().second.toFill(),
                fills = ordered.drop(1).map { it.second.toFill() },
                distanceMetres = distance,
                hasGap = gapHit,
                odometerMissing = missingOdo || odos.size < ordered.size,
            )
            PremiumConsumption.compute(interval)
        }
        val cost = if (measured.unknown.isEmpty()) {
            PremiumConsumption.fuelPurchasePerKm(measured.eligibleCostCentavos, measured.distanceMetres)
        } else {
            PremiumCostResult(unknown = measured.unknown)
        }
        val combined = if (fills.isEmpty() && chargeSessions == 0) {
            Triple("", "", "no-records")
        } else if (missingOdo) {
            Triple("", "", "odometer-missing")
        } else if (gapHit) {
            Triple("", "", "declared-fill-gap")
        } else {
            // Shared distance, counted once: exact money even for PHEV.
            val result = com.anpfuel.domain.premium.PremiumEnergy
                .combinedPurchasePerKm(purchase, chargeCost, distance)
            Triple(result.centavosPerKm, result.method, result.unknown)
        }
        // EE-23 energy bases: purchased sums are exact; measured needs an
        // unshared odometer distance; reference reuses the same engine over
        // the whole history; PHEV months refuse per-carrier ratios.
        val energyMeasured = if (chargeSessions == 0 && fills.isEmpty()) {
            // No movement at all: unknown basis, never a zero-filled ratio.
            com.anpfuel.domain.premium.PremiumEnergyResult(unknown = "no-records")
        } else {
            energyWindow(
                purchasedWh = chargeEnergy, costCentavos = chargeCost,
                sharedOdos = sharedOdos, hasEnergy = energyStated > 0,
                complete = energyStated == chargeSessions,
                hasGap = gapHit, gapOdoMissing = energyOdoMissing, socBad = socBad, mixed = hasMixedCarrier,
            )
        }
        val energyReference = energyReference(vehicle, all, queuedGaps)
        val energyComparison = com.anpfuel.domain.premium.PremiumEnergy.compare(energyReference, energyMeasured)
        val estimated = if (measured.unknown.isNotEmpty()) {
            runCatching { efficiency.get(vehicle.id.value)?.let { EstimatedEfficiency.of(it).toPlainString() } }.getOrNull()
        } else {
            null
        }
        val previous = previousMonth(vehicle, year, month, zone, all, queuedGaps)
        val comparison = PremiumConsumption.compare(previous, measured)
        return MonthlyVehicleIndicators(
            vehicleId = vehicle.id, vehicleName = vehicle.displayName, year = year, month = month, timezone = timezone,
            purchaseCentavos = purchase, expenseCentavos = expense, chargeCostCentavos = chargeCost,
            chargeEnergyWh = chargeEnergy, chargeFeesCentavos = chargeFees, byCategory = byCategory,
            recordCount = rows.size, measured = measured,
            costPerKm = cost.centavosPerKm, costPerKmMethod = cost.method,
            combinedCentavosPerKm = combined.first, combinedMethod = combined.second, combinedUnknown = combined.third,
            estimatedKmPerL = estimated,
            comparisonChangePercent = comparison.changePercent, comparisonUnknown = comparison.unknown,
            energyMeasured = energyMeasured, energyReference = energyReference, energyComparison = energyComparison,
            windowNote = MonthlyWindowNote(rows.size, fills.size, missingOdo, distance, hasMixedCarrier),
        )
    }

    private fun energyReference(
        vehicle: Vehicle,
        all: List<com.anpfuel.domain.repository.StoredLedgerDraft>,
        queuedGaps: List<com.anpfuel.domain.premium.PremiumCommand>,
    ): com.anpfuel.domain.premium.PremiumEnergyResult {
        val vehicleRows = all.filter { rowVehicle(it.draft) == vehicle.id.value }
        if (vehicleRows.isEmpty()) return com.anpfuel.domain.premium.PremiumEnergyResult(unknown = "no-records")
        var purchased = 0L
        var cost = 0L
        var stated = 0
        var sessions = 0
        var socBad = false
        var odoMissing = false
        var fuelEver = false
        val odos = mutableListOf<Pair<java.time.Instant, Long>>()
        for (stored in vehicleRows) {
            when (val d = stored.draft) {
                is RefuelDraft -> fuelEver = true
                is ChargeDraft -> {
                    sessions++
                    val energyWh = d.energyWh
                    if (energyWh != null) {
                        stated++
                        purchased = checkedAdd(purchased, energyWh) ?: return com.anpfuel.domain.premium.PremiumEnergyResult(unknown = "input-out-of-bounds")
                        if (d.odometerMetres == null) odoMissing = true
                    }
                    val total = d.totalCentavos
                    if (total != null) {
                        cost = checkedAdd(cost, total) ?: return com.anpfuel.domain.premium.PremiumEnergyResult(unknown = "input-out-of-bounds")
                    }
                    val socStart = d.socStart
                    val socEnd = d.socEnd
                    if (socStart != null && socEnd != null && socEnd < socStart) socBad = true
                    val odo = d.odometerMetres
                    if (odo != null) odos.add(d.occurredAt to odo)
                }
                is ExpenseDraft -> Unit
            }
        }
        val ids = vehicleRows.map { it.draft.recordId }.toSet()
        val gapHit = queuedGaps.any { gap -> ids.any { id -> gap.payloadJson.contains(id) } }
        // Historical reference over the whole held history; never a guarantee.
        return energyWindow(
            purchasedWh = purchased, costCentavos = cost,
            sharedOdos = odos.sortedBy { it.first }.map { it.second },
            hasEnergy = stated > 0, complete = stated == sessions,
            hasGap = gapHit, gapOdoMissing = odoMissing, socBad = socBad,
            mixed = fuelEver && stated > 0,
        )
    }

    private fun energyWindow(
        purchasedWh: Long,
        costCentavos: Long,
        sharedOdos: List<Long>,
        hasEnergy: Boolean,
        complete: Boolean,
        hasGap: Boolean,
        gapOdoMissing: Boolean,
        socBad: Boolean,
        mixed: Boolean,
    ): com.anpfuel.domain.premium.PremiumEnergyResult {
        val distance = if (sharedOdos.size >= 2) sharedOdos.last() - sharedOdos.first() else 0L
        return com.anpfuel.domain.premium.PremiumEnergy.compute(
            com.anpfuel.domain.premium.PremiumEnergyInterval(
                purchasedWh = purchasedWh, costCentavos = costCentavos,
                distanceMetres = distance, hasEnergy = hasEnergy, complete = complete,
                hasGap = hasGap, odometerMissing = gapOdoMissing,
                socInconsistent = socBad, mixedCarrier = mixed,
            ),
        )
    }

    private fun previousMonth(
        vehicle: Vehicle, year: Int, month: Int, zone: ZoneId,
        all: List<com.anpfuel.domain.repository.StoredLedgerDraft>,
        queuedGaps: List<com.anpfuel.domain.premium.PremiumCommand>,
    ): PremiumConsumptionResult {
        val prev = if (month == 1) year - 1 to 12 else year to month - 1
        if (prev.first < 2000) return PremiumConsumptionResult(unknown = "no-records")
        val start = LocalDate.of(prev.first, prev.second, 1).atStartOfDay(zone).toInstant()
        val endMonth = if (prev.second == 12) LocalDate.of(prev.first + 1, 1, 1) else LocalDate.of(prev.first, prev.second + 1, 1)
        val end = endMonth.atStartOfDay(zone).toInstant()
        val rows = all.filter { it.draft.let { d -> rowVehicle(d) == vehicle.id.value } }
            .filter { val at = rowOccurred(it.draft); !at.isBefore(start) && at.isBefore(end) }
        val fills = rows.mapNotNull { (it.draft as? RefuelDraft) }.sortedBy { it.occurredAt }
        if (fills.isEmpty()) return PremiumConsumptionResult(unknown = "no-records")
        val odos = fills.mapNotNull { it.odometerMetres }
        return PremiumConsumption.compute(
            PremiumInterval(
                startingFillExcluded = fills.first().toFill(),
                fills = fills.drop(1).map { it.toFill() },
                distanceMetres = if (odos.size >= 2) odos.last() - odos.first() else 0L,
                odometerMissing = odos.size < fills.size,
            ),
        )
    }

    private fun RefuelDraft.toFill() = PremiumFill(
        quantityMilli = quantityMilli, totalCentavos = totalCentavos,
        full = fill == LedgerFill.FULL, fuel = fuel,
        unit = fuel.unit,
    )

    private fun rowVehicle(draft: com.anpfuel.domain.premium.LedgerDraft): String = when (draft) {
        is RefuelDraft -> draft.vehicleId
        is ExpenseDraft -> draft.vehicleId
        is ChargeDraft -> draft.vehicleId
    }

    private fun rowOccurred(draft: com.anpfuel.domain.premium.LedgerDraft): Instant = when (draft) {
        is RefuelDraft -> draft.occurredAt
        is ExpenseDraft -> draft.occurredAt
        is ChargeDraft -> draft.occurredAt
    }
}

object MonthlyIndicatorsCsv {

    fun build(vehicleName: String, rows: List<MonthlyVehicleIndicators>): String {
        val out = StringBuilder("vehicle,metric,value\n")
        for (row in rows) {
            val name = escape(vehicleName.ifEmpty { row.vehicleName })
            fun line(metric: String, value: String) {
                out.append(name).append(',').append(metric).append(',').append(value).append('\n')
            }
            line("purchase_centavos", row.purchaseCentavos.toString())
            line("expense_centavos", row.expenseCentavos.toString())
            line("charge_energy_wh", row.chargeEnergyWh.toString())
            line("charge_cost_centavos", row.chargeCostCentavos.toString())
            for ((category, total) in row.byCategory.toSortedMap()) {
                out.append(name).append(",expense:").append(category.wire).append(',').append(total.toString()).append('\n')
            }
            line("distance_metres", row.windowNote.distanceMetres.toString())
            if (row.measured.unknown.isEmpty()) {
                line("km_per_unit", row.measured.kmPerUnit)
                line("centavos_per_km", row.measured.centavosPerKm)
            } else {
                line("unknown", escape(row.measured.unknown))
            }
            if (row.combinedUnknown.isNotEmpty()) line("combined_unknown", escape(row.combinedUnknown))
            else line("combined_centavos_per_km", row.combinedCentavosPerKm)
            if (row.energyMeasured.unknown.isNotEmpty()) line("energy_unknown", escape(row.energyMeasured.unknown))
            else {
                line("energy_purchased_wh", row.energyMeasured.purchasedWh.toString())
                line("energy_measured_wh_per_km", row.energyMeasured.whPerKm)
            }
            if (row.energyReference.unknown.isNotEmpty()) line("energy_reference_unknown", escape(row.energyReference.unknown))
            else line("energy_reference_wh_per_km", row.energyReference.whPerKm)
            if (row.energyComparison.unknown.isNotEmpty()) line("energy_comparison_unknown", escape(row.energyComparison.unknown))
            else line("energy_comparison_change_percent", row.energyComparison.changePercent)
            line("charge_fees_centavos", row.chargeFeesCentavos.toString())
        }
        return out.toString()
    }

    /** Formula-injection-safe escaping: leading =,+,-,@ (or quotes) force
     * a quoted cell prefixed with a literal single quote; embedded quotes
     * double up. Numbers pass through untouched. */
    fun escape(field: String): String {
        if (field.isEmpty()) return field
        val first = field[0]
        val risky = first == '=' || first == '+' || first == '-' || first == '@' ||
            field.contains(',') || field.contains('"') || field.contains('\n')
        if (!risky) return field
        return "\"'" + field.replace("\"", "\"\"") + "\""
    }
}

private fun checkedAdd(a: Long, b: Long): Long? =
    if (a < 0 || b < 0 || a > Long.MAX_VALUE - b) null else a + b
