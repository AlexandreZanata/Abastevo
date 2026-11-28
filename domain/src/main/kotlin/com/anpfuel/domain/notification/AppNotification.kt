package com.anpfuel.domain.notification

import com.anpfuel.domain.valueobject.FuelProduct

/** Where an item of the Notifications screen came from (B-BR-NOTIF-01). */
enum class AppNotificationSource { ACCOUNT, LOCAL }

/** What happened; each kind renders its own localized title and body on the device. */
sealed interface AppNotificationContent {
    /** `forum.reply`: someone replied to a topic of the account. */
    data class ForumReply(val topicId: String, val topicTitle: String, val authorName: String) : AppNotificationContent

    /** `contribution.validated` / `contribution.rejected`. */
    data class ContributionDecided(
        val stationName: String,
        val fuel: FuelProduct?,
        val validated: Boolean,
    ) : AppNotificationContent

    /** `local.city_average_drop`: fuels whose weekly city average fell, largest drop first. */
    data class CityAverageDrop(
        val municipality: String,
        val stateAbbreviation: String,
        val drops: List<FuelDrop>,
    ) : AppNotificationContent

    /** `local.vehicle_price_drop`: the UC-014 per-vehicle alert. */
    data class VehiclePriceDrop(val vehicleName: String, val priceFormatted: String) : AppNotificationContent

    /** `local.new_week`: a newer ANP survey week was imported. */
    data class NewSurveyWeek(val startIso: String, val endIso: String) : AppNotificationContent
}

/** One fuel that got cheaper: the new average and the drop, both in milli-BRL. */
data class FuelDrop(val fuel: FuelProduct, val currentMilli: Long, val dropMilli: Long) {
    init {
        require(currentMilli > 0 && dropMilli > 0) { "a drop needs a positive price and a positive fall" }
    }
}

data class AppNotification(
    val id: String,
    val source: AppNotificationSource,
    val content: AppNotificationContent,
    val createdAtMillis: Long,
    val read: Boolean,
)

/** One page of the account inbox and the server's unread count. */
data class AccountNotificationPage(
    val items: List<AppNotification>,
    val nextBefore: String?,
    val unread: Int,
)
