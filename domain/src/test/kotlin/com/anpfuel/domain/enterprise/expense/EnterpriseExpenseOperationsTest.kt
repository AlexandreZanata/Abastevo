package com.anpfuel.domain.enterprise.expense

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncJson
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Test

/** Decoder refusals the builder cannot produce: envelope shape, exact keys and recognised later forms (B-BR-EMP-447). */
class EnterpriseExpenseOperationsTest {
    private fun id(n: Int) = "9e5b0000-0000-4000-8000-%012d".format(n)
    private val binding = SyncBinding.of(id(9501), "https://api.example.invalid", id(9502))
    private fun text(v: String) = SyncJson.Text(v)

    private fun payload(change: (MutableMap<String, SyncJson>) -> Unit = {}): Map<String, SyncJson> {
        val p = mutableMapOf<String, SyncJson>("asset_id" to text(id(9721)), "category" to text("TOLL"), "paid_total" to text("1250"),
            "company_amount" to text("1250"), "payer" to text("EMPLOYEE"), "trip_id" to SyncJson.Null)
        change(p)
        return p
    }

    private fun operation(type: String = ExpenseOperations.SUBMIT, entityType: String = ExpenseOperations.ENTITY, revision: SyncJson = SyncJson.Null,
        dependsOn: List<SyncJson> = emptyList(), attachments: List<SyncJson> = emptyList(), soft: List<SyncJson> = emptyList(),
        body: Map<String, SyncJson> = payload()): EnterpriseOperation {
        val root = SyncJson.Object(mapOf(
            "schema_version" to SyncJson.Integer("1"), "operation_id" to text(id(9701)), "owner_account_id" to text(id(9501)),
            "api_origin" to text("https://api.example.invalid"), "org_id" to text(id(9502)), "membership_id" to text(id(9503)),
            "enrollment_id" to text(id(9504)), "type" to text(type), "operation_schema" to SyncJson.Integer("1"),
            "entity" to SyncJson.Object(mapOf("type" to text(entityType), "id" to text(id(9711)), "expected_revision" to revision)),
            "occurred_at" to text("2026-11-15T12:00:00.123Z"), "entry_zone" to text("America/Cuiaba"), "device_offset_seconds" to SyncJson.Integer("-14400"),
            "depends_on" to SyncJson.Array(dependsOn), "soft_references" to SyncJson.Array(soft), "attachments" to SyncJson.Array(attachments),
            "payload" to SyncJson.Object(body)))
        return EnterpriseOperation.seal(CanonicalJson.encode(root, CanonicalJson.MAX_OPERATION_DEPTH), binding)
    }

    private fun refusal(operation: EnterpriseOperation): ExpenseOperationFailure? =
        try { ExpenseOperations.read(operation); null } catch (e: ExpenseOperationException) { e.failure }

    @Test fun `envelope shape and exact keys are malformed`() {
        val malformed = listOf(
            operation(type = "expense.record"),
            operation(entityType = "fueling"),
            operation(revision = SyncJson.Integer("1")),
            operation(body = payload { it["note"] = text("pedágio") }),
            operation(body = payload { it.remove("trip_id") }),
            operation(body = payload { it["asset_id"] = SyncJson.Null }),
            operation(body = payload { it["paid_total"] = SyncJson.Integer("1250") }),
            operation(body = payload { it["category"] = text("FUEL") }),
            operation(body = payload { it["category"] = text("FUEL"); it["trip_id"] = text(id(9731)) }),
        )
        for (op in malformed) assertEquals(ExpenseOperationFailure.MALFORMED, refusal(op))
    }

    @Test fun `recognised later forms are unsupported`() {
        val attachment = SyncJson.Object(mapOf("id" to text(id(9801)), "sha256" to text("a".repeat(64)), "bytes" to SyncJson.Integer("10"), "mime" to text("image/jpeg")))
        val unsupported = listOf(
            operation(attachments = listOf(attachment)),
            operation(dependsOn = listOf(text(id(9700)))),
            operation(body = payload { it["trip_id"] = text(id(9731)) }),
            operation(body = payload { it["category"] = text("MEALS") }),
        )
        for (op in unsupported) assertEquals(ExpenseOperationFailure.UNSUPPORTED, refusal(op))
    }

    @Test fun `an accepted fact keeps the envelope declaration and redacts values`() {
        val fact = ExpenseOperations.read(operation())
        assertEquals(TimeDeclaration(EnterpriseTime.parseInstant("2026-11-15T12:00:00.123Z"), "America/Cuiaba", -14400), fact.declared)
        assertEquals(1250L, fact.paidCentavos)
        assertFalse(fact.toString().contains("1250"))
    }
}
