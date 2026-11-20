package com.anpfuel.app.ui.enterprise.trip

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.application.enterprise.management.FieldManagementAdvisory
import com.anpfuel.domain.enterprise.route.DrivingModePolicy

/**
 * EMP-MAINT-004 / EMP-STOCK-003: Driver-safe management summary inside the
 * trip card showing compliance expiry notices, open maintenance requests, and
 * offline internal dispense count without org-wide financial figures.
 */
@Composable
internal fun TripFieldManagementCard(
    advisory: FieldManagementAdvisory,
    speedCmPerSecond: Int?,
    onRequestMaintenance: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    val isDriving = DrivingModePolicy.isDriving(speedCmPerSecond)
    TripCard(modifier.testTag("enterprise-trip-field-mgmt-card")) {
        Text(
            text = stringResource(R.string.enterprise_mgmt_card_title),
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.semantics { heading() },
        )

        val nearestDays = advisory.nearestDaysRemaining
        when {
            advisory.expiredComplianceCount > 0 -> {
                Notice(
                    message = stringResource(
                        R.string.enterprise_mgmt_compliance_expired,
                        advisory.expiredComplianceCount,
                    ),
                    modifier = Modifier.testTag("enterprise-mgmt-compliance-expired"),
                )
            }
            advisory.dueSoonComplianceCount > 0 && nearestDays != null -> {
                Notice(
                    message = stringResource(
                        R.string.enterprise_mgmt_compliance_due_soon,
                        advisory.dueSoonComplianceCount,
                        nearestDays,
                    ),
                    modifier = Modifier.testTag("enterprise-mgmt-compliance-due-soon"),
                )
            }
            else -> {
                Text(
                    text = stringResource(R.string.enterprise_mgmt_compliance_ok),
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.testTag("enterprise-mgmt-compliance-ok"),
                )
            }
        }

        if (advisory.openMaintenanceRequestCount > 0) {
            Text(
                text = stringResource(
                    R.string.enterprise_mgmt_maint_requests_open,
                    advisory.openMaintenanceRequestCount,
                ),
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("enterprise-mgmt-maint-requests"),
            )
        }

        if (advisory.offlineInternalDispenseQueued > 0) {
            Text(
                text = stringResource(
                    R.string.enterprise_mgmt_internal_dispense_queued,
                    advisory.offlineInternalDispenseQueued,
                ),
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("enterprise-mgmt-internal-dispense"),
            )
        }

        if (onRequestMaintenance != null) OutlinedButton(
            onClick = onRequestMaintenance,
            enabled = !isDriving,
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(min = 48.dp)
                .testTag("enterprise-mgmt-request-maint-btn"),
        ) {
            Text(stringResource(R.string.enterprise_mgmt_request_maintenance))
        }
    }
}
