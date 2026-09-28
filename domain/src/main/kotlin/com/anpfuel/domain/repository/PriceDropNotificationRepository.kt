package com.anpfuel.domain.repository

import com.anpfuel.domain.model.PriceDropAlertNotification
import com.anpfuel.domain.valueobject.DomainId

/**
 * Port for UC-014 local price drop notifications.
 */
interface PriceDropNotificationRepository {

    fun hasPostNotificationsPermission(): Boolean

    suspend fun showPriceDropAlert(notification: PriceDropAlertNotification)

    suspend fun cancelForVehicle(vehicleId: DomainId)
}
