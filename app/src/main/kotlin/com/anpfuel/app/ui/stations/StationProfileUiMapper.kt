package com.anpfuel.app.ui.stations

import com.anpfuel.domain.profile.ProfileBadge

/**
 * P32-T01 — Representation-badge presentation labels.
 *
 * Labels describe representation state only and never assert fuel or
 * price quality. Screen readers receive the same text via content
 * description bindings at the call site.
 */
data class StationProfileBadgeUi(
    val label: String,
)

object StationProfileUiMapper {

    fun toUi(badge: ProfileBadge): StationProfileBadgeUi = when (badge) {
        ProfileBadge.Unclaimed -> StationProfileBadgeUi(label = "Perfil não reivindicado")
        ProfileBadge.PendingReview -> StationProfileBadgeUi(label = "Em análise")
        is ProfileBadge.Verified -> StationProfileBadgeUi(label = "Representação verificada")
        ProfileBadge.StaleUnverified -> StationProfileBadgeUi(label = "Verificação desatualizada")
    }
}
