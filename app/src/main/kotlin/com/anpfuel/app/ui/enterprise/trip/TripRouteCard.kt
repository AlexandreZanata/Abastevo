package com.anpfuel.app.ui.enterprise.trip

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.material3.Button
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
import com.anpfuel.application.enterprise.route.TripNavigationSessionState
import com.anpfuel.domain.enterprise.route.DrivingModePolicy
import com.anpfuel.domain.enterprise.route.GuidanceDegradedState

/**
 * EMP-ROUTE-006..009: trip-aware operational route card inside the open
 * enterprise trip without exposing coordinates in accessibility logs or touching
 * personal/tutorial navigation files.
 */
@Composable
internal fun TripRouteCard(
    state: TripNavigationSessionState,
    speedCmPerSecond: Int?,
    onConfirmArrival: () -> Unit,
    onApplyPendingRevision: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val isDriving = DrivingModePolicy.isDriving(speedCmPerSecond)
    TripCard(modifier.testTag("enterprise-trip-route-card")) {
        Text(
            text = stringResource(R.string.enterprise_route_card_title),
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.semantics { heading() },
        )
        val total = state.activePlan.waypoints.size.coerceAtLeast(1)
        val stopOrdinal = (state.currentWaypointIndex + 1).coerceIn(1, total)
        Text(
            text = stringResource(R.string.enterprise_route_waypoints_summary, stopOrdinal, total),
            modifier = Modifier.testTag("enterprise-trip-route-progress"),
        )

        if (state.heavyVehicleWarning.required) {
            Notice(
                message = stringResource(R.string.enterprise_route_heavy_vehicle_warning),
                modifier = Modifier.testTag("enterprise-trip-route-heavy-warning"),
            )
        }

        if (isDriving) {
            Notice(
                message = stringResource(R.string.enterprise_route_driving_locked),
                modifier = Modifier.testTag("enterprise-trip-route-driving-lock"),
            )
        }

        if (state.recalculationCount > 0) {
            Text(
                text = stringResource(R.string.enterprise_route_recalculations, state.recalculationCount),
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.testTag("enterprise-trip-route-recalc-count"),
            )
        }

        state.textStopPrompt?.let { prompt ->
            Notice(
                message = stringResource(R.string.enterprise_route_text_stop_prompt, prompt.label),
                modifier = Modifier.testTag("enterprise-trip-route-text-stop"),
            )
        }

        state.degradedState?.let { degraded ->
            val msgRes = when (degraded) {
                GuidanceDegradedState.MAP_REGION_MISSING -> R.string.enterprise_route_degraded_map_missing
                GuidanceDegradedState.ROUTE_NOT_FOUND -> R.string.enterprise_route_degraded_no_route
                GuidanceDegradedState.NO_USABLE_FIX -> R.string.enterprise_route_degraded_no_fix
                GuidanceDegradedState.VOICE_UNAVAILABLE_VISUAL_ONLY -> R.string.enterprise_route_degraded_voice_unavailable
            }
            Notice(
                message = stringResource(msgRes),
                modifier = Modifier.testTag("enterprise-trip-route-degraded"),
            )
        }

        state.arrivalProposal?.let { proposal ->
            Notice(
                message = stringResource(R.string.enterprise_route_arrival_prompt, proposal.waypointLabel),
                modifier = Modifier.testTag("enterprise-trip-route-arrival-prompt"),
            )
            Button(
                onClick = onConfirmArrival,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 56.dp)
                    .testTag("enterprise-trip-route-arrival-confirm"),
            ) {
                Text(stringResource(R.string.enterprise_route_arrival_confirm))
            }
        }

        state.pendingPlanRevision?.let { pending ->
            Notice(
                message = stringResource(R.string.enterprise_route_pending_revision, pending.revision),
                modifier = Modifier.testTag("enterprise-trip-route-pending-revision"),
            )
            if (!isDriving) {
                OutlinedButton(
                    onClick = onApplyPendingRevision,
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(min = 48.dp)
                        .testTag("enterprise-trip-route-apply-revision"),
                ) {
                    Text(stringResource(R.string.enterprise_route_apply_revision))
                }
            }
        }

        Text(
            text = stringResource(R.string.enterprise_route_attribution),
            style = MaterialTheme.typography.bodySmall,
            modifier = Modifier.testTag("enterprise-trip-route-attribution"),
        )
    }
}
