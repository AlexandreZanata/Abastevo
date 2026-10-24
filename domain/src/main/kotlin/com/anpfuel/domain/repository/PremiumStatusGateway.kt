package com.anpfuel.domain.repository

import com.anpfuel.domain.premium.PremiumStatus

/** MEX-16 private Premium status read bound to the current session. */
interface PremiumStatusGateway {
    suspend fun status(familyId: String, accessToken: String): PremiumStatus
}
