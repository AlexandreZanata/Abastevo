package com.anpfuel.data.mapper

import com.anpfuel.domain.premium.ChargeDraft
import com.anpfuel.domain.premium.ExpenseCategory
import com.anpfuel.domain.premium.ExpenseDraft
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerDraft
import com.anpfuel.domain.premium.LedgerFill
import com.anpfuel.domain.premium.PremiumFuel
import com.anpfuel.domain.premium.PremiumValidationException
import com.anpfuel.domain.premium.RefuelDraft
import java.time.Instant
import org.json.JSONObject

/**
 * MEX-11 strict draft codec: decodes stored bodies back into validated
 * drafts through the same constructors the UI path uses, so a corrupt or
 * drifted row fails closed instead of dispatching invented values.
 */
object LedgerDraftCodec {

    fun decode(dataset: LedgerDataset, body: String): LedgerDraft {
        try {
            val root = JSONObject(body)
            return when (dataset) {
                LedgerDataset.REFUEL -> RefuelDraft(
                    id = root.getString("id"),
                    vehicleId = root.getString("vehicle_id"),
                    occurredAt = Instant.parse(root.getString("occurred_at")),
                    timezone = root.getString("timezone"),
                    fuel = PremiumFuel.valueOf(root.getString("fuel")),
                    quantityMilli = root.getLong("quantity_milli"),
                    totalCentavos = root.getLong("total_centavos"),
                    unitPriceMilli = optLong(root, "unit_price_milli"),
                    odometerMetres = optLong(root, "odometer_metres"),
                    fill = LedgerFill.entries.first { it.wire == root.getString("fill") },
                    stationId = optText(root, "station_id"),
                    note = optText(root, "note"),
                )
                LedgerDataset.EXPENSE -> ExpenseDraft(
                    id = root.getString("id"),
                    vehicleId = root.getString("vehicle_id"),
                    occurredAt = Instant.parse(root.getString("occurred_at")),
                    timezone = root.getString("timezone"),
                    amountCentavos = root.getLong("amount_centavos"),
                    category = ExpenseCategory.entries.first { it.wire == root.getString("category") },
                    odometerMetres = optLong(root, "odometer_metres"),
                    note = optText(root, "note"),
                )
                LedgerDataset.CHARGE -> ChargeDraft(
                    id = root.getString("id"),
                    vehicleId = root.getString("vehicle_id"),
                    occurredAt = Instant.parse(root.getString("occurred_at")),
                    timezone = root.getString("timezone"),
                    energyWh = optLong(root, "energy_wh"),
                    totalCentavos = optLong(root, "total_centavos"),
                    socStart = optInt(root, "soc_start_percent"),
                    socEnd = optInt(root, "soc_end_percent"),
                    odometerMetres = optLong(root, "odometer_metres"),
                    feesCentavos = optLong(root, "fees_centavos"),
                    tariffId = optText(root, "tariff_id"),
                    tariffRevision = optText(root, "tariff_revision"),
                    note = optText(root, "note"),
                )
            }
        } catch (e: PremiumValidationException) {
            throw e
        } catch (e: Exception) {
            throw PremiumValidationException("corrupt-draft")
        }
    }

    private fun optText(root: JSONObject, key: String): String? =
        if (root.isNull(key)) null else root.optString(key, null)

    private fun optLong(root: JSONObject, key: String): Long? =
        if (root.isNull(key)) null else root.getLong(key)

    private fun optInt(root: JSONObject, key: String): Int? =
        if (root.isNull(key)) null else root.getInt(key)
}
