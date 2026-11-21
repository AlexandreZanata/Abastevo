package com.anpfuel.application.enterprise.management

import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.maintenance.ComplianceState
import com.anpfuel.domain.enterprise.maintenance.EnterpriseMaintenanceCalculator
import com.anpfuel.domain.enterprise.stock.StockOutcome
import com.anpfuel.domain.enterprise.stock.StockPool
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.DataInputStream
import java.io.DataOutputStream
import java.time.Instant
import java.time.LocalDate
import java.time.temporal.ChronoUnit

/**
 * Field-safe driver/operator management slices (EMP-MAINT-004/012,
 * EMP-STOCK-003): offline compliance notices, field maintenance request drafts,
 * and internal fuel dispense checks without exposing org-wide financial, audit,
 * or multi-company consolidation data.
 */
object EnterpriseManagementProducers {
    const val COMPLIANCE_BOOK_RECORD_ID = "f05e0b00-0000-4000-8000-0000000000c1"
    const val MAINTENANCE_REQUESTS_RECORD_ID = "f05e0b00-0000-4000-8000-0000000000c2"
}

data class FieldComplianceNotice(
    val itemId: String,
    val subjectKind: String,
    val itemKind: String,
    val expiryDay: LocalDate,
    val state: ComplianceState,
    val leadDays: Int,
) {
    init {
        EnterprisePartition.requireId(itemId)
        require(subjectKind in setOf("ASSET", "EMPLOYEE") && itemKind.isNotBlank() && leadDays >= 1)
    }
}

data class FieldMaintenanceRequestDraft(
    val requestId: String,
    val assetId: String,
    val description: String,
    val odometerM: Long?,
    val createdAt: Instant,
    val state: String = "QUEUED",
) {
    init {
        EnterprisePartition.requireId(requestId)
        EnterprisePartition.requireId(assetId)
        require(description.isNotBlank() && description.length <= 500)
        require(odometerM == null || odometerM >= 0L)
    }

    override fun toString(): String = "FieldMaintenanceRequestDraft(redacted)"
}

data class FieldInternalDispenseDraft(
    val tankId: String,
    val tankName: String,
    val assetId: String,
    val product: String,
    val quantityMl: Long,
) {
    override fun toString(): String = "FieldInternalDispenseDraft(redacted)"
}

data class FieldManagementBook(
    val membershipId: String,
    val enrollmentId: String,
    val complianceNotices: List<FieldComplianceNotice> = emptyList(),
    val maintenanceRequests: List<FieldMaintenanceRequestDraft> = emptyList(),
    val internalDispenseQueueCount: Int = 0,
    val updatedAt: Instant = Instant.EPOCH,
) {
    init {
        EnterprisePartition.requireId(membershipId)
        EnterprisePartition.requireId(enrollmentId)
        require(internalDispenseQueueCount >= 0)
    }

    fun requireBinding(expectedMembershipId: String, expectedEnrollmentId: String): FieldManagementBook {
        require(membershipId == expectedMembershipId && enrollmentId == expectedEnrollmentId) {
            "FieldManagementBook partition binding mismatch"
        }
        return this
    }

    val canonicalBytes: ByteArray by lazy {
        val baos = ByteArrayOutputStream()
        DataOutputStream(baos).use { out ->
            out.writeInt(FORMAT_VERSION)
            out.writeUTF(membershipId)
            out.writeUTF(enrollmentId)
            out.writeInt(complianceNotices.size)
            for (c in complianceNotices) {
                out.writeUTF(c.itemId)
                out.writeUTF(c.subjectKind)
                out.writeUTF(c.itemKind)
                out.writeUTF(c.expiryDay.toString())
                out.writeUTF(c.state.name)
                out.writeInt(c.leadDays)
            }
            out.writeInt(maintenanceRequests.size)
            for (r in maintenanceRequests) {
                out.writeUTF(r.requestId)
                out.writeUTF(r.assetId)
                out.writeUTF(r.description)
                out.writeBoolean(r.odometerM != null)
                if (r.odometerM != null) out.writeLong(r.odometerM)
                out.writeLong(r.createdAt.toEpochMilli())
                out.writeUTF(r.state)
            }
            out.writeInt(internalDispenseQueueCount)
            out.writeLong(updatedAt.toEpochMilli())
        }
        baos.toByteArray()
    }

    val maintenanceManifestBytes: ByteArray by lazy {
        val baos = ByteArrayOutputStream()
        DataOutputStream(baos).use { out ->
            out.writeInt(FORMAT_VERSION)
            out.writeUTF(membershipId)
            out.writeUTF(enrollmentId)
            out.writeInt(maintenanceRequests.size)
            for (r in maintenanceRequests) {
                out.writeUTF(r.requestId)
                out.writeUTF(r.state)
            }
        }
        baos.toByteArray()
    }

    override fun toString(): String = "FieldManagementBook(redacted)"

    companion object {
        private const val FORMAT_VERSION = 1

        fun empty(membershipId: String, enrollmentId: String, now: Instant = Instant.EPOCH) =
            FieldManagementBook(membershipId = membershipId, enrollmentId = enrollmentId, updatedAt = now)

        fun decode(bytes: ByteArray): FieldManagementBook {
            DataInputStream(ByteArrayInputStream(bytes)).use { inp ->
                val ver = inp.readInt()
                require(ver == FORMAT_VERSION) { "Unsupported FieldManagementBook version" }
                val member = inp.readUTF()
                val enrollment = inp.readUTF()
                val cCount = inp.readInt()
                require(cCount in 0..1000)
                val notices = (0 until cCount).map {
                    FieldComplianceNotice(
                        itemId = inp.readUTF(),
                        subjectKind = inp.readUTF(),
                        itemKind = inp.readUTF(),
                        expiryDay = LocalDate.parse(inp.readUTF()),
                        state = ComplianceState.valueOf(inp.readUTF()),
                        leadDays = inp.readInt(),
                    )
                }
                val rCount = inp.readInt()
                require(rCount in 0..1000)
                val requests = (0 until rCount).map {
                    val reqId = inp.readUTF()
                    val assetId = inp.readUTF()
                    val desc = inp.readUTF()
                    val hasOdo = inp.readBoolean()
                    val odo = if (hasOdo) inp.readLong() else null
                    val at = Instant.ofEpochMilli(inp.readLong())
                    val st = inp.readUTF()
                    FieldMaintenanceRequestDraft(reqId, assetId, desc, odo, at, st)
                }
                val qCount = inp.readInt()
                val updatedAt = Instant.ofEpochMilli(inp.readLong())
                require(inp.available() == 0) { "Trailing bytes in FieldManagementBook" }
                return FieldManagementBook(member, enrollment, notices, requests, qCount, updatedAt)
            }
        }
    }
}

interface EnterpriseFieldManagementStore {
    suspend fun read(attempt: EnterpriseQueueAttempt): FieldManagementBook?
    suspend fun save(attempt: EnterpriseQueueAttempt, book: FieldManagementBook)
}

data class FieldManagementAdvisory(
    val expiredComplianceCount: Int,
    val dueSoonComplianceCount: Int,
    val nearestDaysRemaining: Int?,
    val openMaintenanceRequestCount: Int,
    val offlineInternalDispenseQueued: Int,
    val hasOrgFinancialDataExposed: Boolean = false,
)

object EnterpriseFieldManagementService {
    fun summarizeForField(
        book: FieldManagementBook,
        licenceControlEnabled: Boolean,
        evalDay: LocalDate,
    ): FieldManagementAdvisory {
        var expired = 0
        var dueSoon = 0
        var nearest: Int? = null
        for (notice in book.complianceNotices) {
            val activeCtrl = notice.subjectKind != "EMPLOYEE" || licenceControlEnabled
            val state = EnterpriseMaintenanceCalculator.evaluateCompliance(
                expiryDay = notice.expiryDay,
                evalDay = evalDay,
                leadDays = notice.leadDays,
                controlEnabled = activeCtrl,
            )
            if (state == ComplianceState.EXPIRED) expired++
            if (state == ComplianceState.DUE_SOON) dueSoon++
            if (state != ComplianceState.CONTROL_DISABLED) {
                val days = ChronoUnit.DAYS.between(evalDay, notice.expiryDay).toInt()
                if (nearest == null || days < nearest) nearest = days
            }
        }
        return FieldManagementAdvisory(
            expiredComplianceCount = expired,
            dueSoonComplianceCount = dueSoon,
            nearestDaysRemaining = nearest,
            openMaintenanceRequestCount = book.maintenanceRequests.count { it.state != "CANCELLED" && it.state != "CLOSED" },
            offlineInternalDispenseQueued = book.internalDispenseQueueCount,
            hasOrgFinancialDataExposed = false,
        )
    }

    fun prepareMaintenanceRequest(
        requestId: String,
        assetId: String,
        description: String,
        odometerM: Long?,
        now: Instant,
    ): FieldMaintenanceRequestDraft = FieldMaintenanceRequestDraft(
        requestId = requestId,
        assetId = assetId,
        description = description.trim(),
        odometerM = odometerM,
        createdAt = now,
    )

    fun prepareInternalDispense(
        tankPool: StockPool,
        tankId: String,
        tankName: String,
        assetId: String,
        product: String,
        quantityMl: Long,
    ): Pair<FieldInternalDispenseDraft, StockOutcome> {
        EnterprisePartition.requireId(tankId)
        EnterprisePartition.requireId(assetId)
        require(tankName.isNotBlank())
        val outcome = tankPool.dispense(product, quantityMl)
        // Redact cost/valuation figures from driver-facing dispense draft summary.
        val redactedOutcome = outcome.copy(movedCentavos = 0L, recognizedExpenseCentavos = 0L, afterValueCentavos = 0L)
        return FieldInternalDispenseDraft(tankId, tankName.trim(), assetId, product, quantityMl) to redactedOutcome
    }
}
