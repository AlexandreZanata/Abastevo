package com.anpfuel.application.enterprise.trip

import com.anpfuel.application.enterprise.EnterpriseContextSummary
import com.anpfuel.application.enterprise.agreement.FieldVoucherNotice
import com.anpfuel.application.enterprise.management.FieldManagementAdvisory
import com.anpfuel.domain.enterprise.route.EnterpriseRoutePlan

/** Scoped enrichment of an accepted trip. The raw voucher and company accounts are absent. */
data class TripFieldReference(
    val tripId: String,
    val plan: EnterpriseRoutePlan?,
    val management: FieldManagementAdvisory,
    val voucher: FieldVoucherNotice?,
) {
    override fun toString(): String = "TripFieldReference(redacted)"
}

fun interface TripFieldReader {
    suspend fun read(context: EnterpriseContextSummary, tripId: String): TripFieldReference?
}
