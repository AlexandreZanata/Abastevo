package com.anpfuel.domain.enterprise.expense

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseValueException
import com.anpfuel.domain.enterprise.EnterpriseValues
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncJson
import com.anpfuel.domain.enterprise.sync.SyncProtocolException

/*
 * The Android producer's signed DIRECT expense operation (EMP-EXP-002, B-BR-EMP-447, EDR-289): the exact schema 1
 * payload of `expense.submit`, pinned by contracts/testdata/enterprise/expense/expense-submit-v1.psv.
 */

/** The intake refusal class a builder or decoder refusal corresponds to. */
enum class ExpenseOperationFailure { MALFORMED, UNSUPPORTED }

class ExpenseOperationException(val failure: ExpenseOperationFailure = ExpenseOperationFailure.MALFORMED) :
    IllegalArgumentException("expense operation: ${failure.name.lowercase()}")

/** The envelope facts of one device-produced operation; the binding is the creation partition (BR-OFFL-003). */
class ExpenseEnvelope(val binding: SyncBinding, val membershipId: String, val enrollmentId: String, val operationId: String,
    val declared: TimeDeclaration) {
    override fun toString(): String = "EnterpriseExpenseEnvelope(redacted)"
}

/** One decoded `expense.submit`; it proves only its own shape, never acceptance. */
class ExpenseSubmitFact(val operationId: String, val declared: TimeDeclaration, val expenseId: String, val assetId: String,
    val category: String, val paidCentavos: Long, val companyCentavos: Long, val payer: String) {
    override fun toString(): String = "EnterpriseExpenseSubmitFact(redacted)"
}

private fun malformed() = ExpenseOperationException(ExpenseOperationFailure.MALFORMED)
private fun unsupported() = ExpenseOperationException(ExpenseOperationFailure.UNSUPPORTED)
private fun identity(raw: String): String = try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw malformed() }
private fun centavos(raw: String): Long = try { EnterpriseValues.parseCentavos(raw) } catch (_: EnterpriseValueException) { throw malformed() }
private val CATEGORY = Regex("^[A-Z][A-Z0-9_]{0,63}$")
private val KEYS = setOf("asset_id", "category", "paid_total", "company_amount", "payer", "trip_id")
private val PAYERS = setOf("COMPANY", "EMPLOYEE")
private fun text(value: String) = SyncJson.Text(value)

/** The intake's own checks, in its order: every final refusal before any recognised later form (EDR-289). */
private class Checked(val asset: String, val category: String, val paid: Long, val company: Long, val payer: String)

private fun check(assetId: String, category: String, paidTotal: String, companyAmount: String, payer: String, tripId: String?): Checked {
    val asset = identity(assetId)
    if (!CATEGORY.matches(category) || category == "FUEL" || payer !in PAYERS) throw malformed()
    val paid = centavos(paidTotal)
    val company = centavos(companyAmount)
    if (company > paid) throw malformed()
    if (tripId != null) identity(tripId)
    if (category !in ExpenseOperations.DIRECT_CATEGORIES || tripId != null) throw unsupported()
    return Checked(asset, category, paid, company, payer)
}

object ExpenseOperations {
    const val SUBMIT = "expense.submit"
    const val ENTITY = "expense"

    /** The v1 direct categories until EMP-EXP-003 (EDR-288); FUEL is a fueling, never a direct expense. */
    val DIRECT_CATEGORIES: List<String> = listOf("FINE", "INSURANCE", "LEASE", "MAINTENANCE", "OTHER", "PARKING", "TOLL")

    /** `expense.submit`: exact centavos texts, a v1 direct category, the payer, the asset and an optional trip. */
    fun submit(envelope: ExpenseEnvelope, expenseId: String, assetId: String, category: String, paidTotal: String,
        companyAmount: String, payer: String, tripId: String?): EnterpriseOperation {
        val expense = identity(expenseId)
        val checked = check(assetId, category, paidTotal, companyAmount, payer, tripId)
        return try {
            val declared = envelope.declared
            val root = SyncJson.Object(mapOf(
                "schema_version" to SyncJson.Integer("1"), "operation_id" to text(envelope.operationId),
                "owner_account_id" to text(envelope.binding.ownerAccountId), "api_origin" to text(envelope.binding.apiOrigin),
                "org_id" to text(envelope.binding.organizationId), "membership_id" to text(envelope.membershipId),
                "enrollment_id" to text(envelope.enrollmentId), "type" to text(SUBMIT), "operation_schema" to SyncJson.Integer("1"),
                "entity" to SyncJson.Object(mapOf("type" to text(ENTITY), "id" to text(expense), "expected_revision" to SyncJson.Null)),
                "occurred_at" to text(EnterpriseTime.formatInstant(declared.occurredAt)), "entry_zone" to text(declared.entryZone),
                "device_offset_seconds" to SyncJson.Integer(declared.deviceOffsetSeconds.toString()),
                "depends_on" to SyncJson.Array(emptyList()), "soft_references" to SyncJson.Array(emptyList()),
                "attachments" to SyncJson.Array(emptyList()),
                "payload" to SyncJson.Object(mapOf("asset_id" to text(checked.asset), "category" to text(checked.category),
                    "paid_total" to text(checked.paid.toString()), "company_amount" to text(checked.company.toString()),
                    "payer" to text(checked.payer), "trip_id" to SyncJson.Null)),
            ))
            EnterpriseOperation.seal(CanonicalJson.encode(root, CanonicalJson.MAX_OPERATION_DEPTH), envelope.binding)
        } catch (_: SyncProtocolException) { throw malformed() }
    }

    private fun field(payload: SyncJson.Object, key: String): String? = when (val value = payload.fields[key]) {
        SyncJson.Null -> null
        is SyncJson.Text -> value.value
        else -> throw malformed()
    }

    /** Strict decode of a device expense operation; anything else is refused with its intake class. */
    fun read(operation: EnterpriseOperation): ExpenseSubmitFact {
        val entity = operation.entity
        if (operation.type != SUBMIT || !operation.supportsSchema || entity.reference.type != ENTITY || entity.expectedRevision != null) throw malformed()
        val payload = operation.payload
        if (payload.fields.keys != KEYS) throw malformed()
        val asset = field(payload, "asset_id") ?: throw malformed()
        val category = field(payload, "category") ?: throw malformed()
        val paid = field(payload, "paid_total") ?: throw malformed()
        val company = field(payload, "company_amount") ?: throw malformed()
        val payer = field(payload, "payer") ?: throw malformed()
        val trip = field(payload, "trip_id")
        val extras = operation.attachments.isNotEmpty() || operation.softReferences.isNotEmpty() || operation.dependsOn.isNotEmpty()
        // Every final refusal wins over a recognised later form, as in the Go intake.
        val checked = check(asset, category, paid, company, payer, trip)
        if (extras) throw unsupported()
        return ExpenseSubmitFact(operation.operationId, operation.declaration, entity.reference.id, checked.asset, checked.category,
            checked.paid, checked.company, checked.payer)
    }
}
