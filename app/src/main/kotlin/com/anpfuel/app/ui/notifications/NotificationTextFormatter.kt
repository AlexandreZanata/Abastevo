package com.anpfuel.app.ui.notifications

import android.content.Context
import com.anpfuel.app.R
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.mapper.PriceFormatter
import com.anpfuel.app.mapper.SurveyWeekFormatter
import com.anpfuel.domain.notification.AppNotificationContent
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import java.math.BigDecimal
import java.util.Locale

/** Localized title and body of one notification kind. */
data class NotificationText(val title: String, val body: String)

/**
 * One wording per B-BR-NOTIF kind, shared by the Notifications screen and the
 * Android notifications, so both always say the same thing.
 */
object NotificationTextFormatter {

    private const val MAX_FUELS_IN_BODY = 3

    fun format(context: Context, content: AppNotificationContent, locale: Locale): NotificationText = when (content) {
        is AppNotificationContent.ForumReply -> NotificationText(
            context.getString(R.string.notifications_forum_reply_title),
            // Replies without a public alias carry an empty author name.
            if (content.authorName.isBlank()) {
                context.getString(R.string.notifications_forum_reply_body_anonymous, content.topicTitle)
            } else {
                context.getString(R.string.notifications_forum_reply_body, content.authorName, content.topicTitle)
            },
        )
        is AppNotificationContent.ContributionDecided -> {
            val fuel = content.fuel?.let { context.getString(FuelProductI18n.toStringRes(it)) }
            val what = listOfNotNull(fuel, content.stationName).joinToString(" · ")
            if (content.validated) {
                NotificationText(
                    context.getString(R.string.notifications_contribution_validated_title),
                    context.getString(R.string.notifications_contribution_validated_body, what),
                )
            } else {
                NotificationText(
                    context.getString(R.string.notifications_contribution_rejected_title),
                    context.getString(R.string.notifications_contribution_rejected_body, what),
                )
            }
        }
        is AppNotificationContent.CityAverageDrop -> NotificationText(
            context.getString(R.string.notifications_city_drop_title, content.municipality, content.stateAbbreviation),
            content.drops.take(MAX_FUELS_IN_BODY).joinToString(", ") { drop ->
                context.getString(
                    R.string.notifications_city_drop_fuel,
                    context.getString(FuelProductI18n.toStringRes(drop.fuel)),
                    milliBrl(drop.currentMilli, locale),
                    milliBrl(drop.dropMilli, locale),
                )
            },
        )
        is AppNotificationContent.VehiclePriceDrop -> NotificationText(
            context.getString(R.string.notifications_vehicle_drop_title),
            context.getString(R.string.notifications_vehicle_drop_body, content.vehicleName, content.priceFormatted),
        )
        is AppNotificationContent.NewSurveyWeek -> NotificationText(
            context.getString(R.string.notifications_new_week_title),
            context.getString(
                R.string.notifications_new_week_body,
                runCatching { SurveyWeekFormatter.formatRange(SurveyWeek.fromIsoDates(content.startIso, content.endIso), locale) }
                    .getOrDefault("${content.startIso} – ${content.endIso}"),
            ),
        )
    }

    private fun milliBrl(milli: Long, locale: Locale): String =
        PriceFormatter.formatAmount(PriceAmount.of(BigDecimal.valueOf(milli).movePointLeft(3)), locale).orEmpty()
}
