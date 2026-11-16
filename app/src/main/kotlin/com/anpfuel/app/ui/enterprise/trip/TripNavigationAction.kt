package com.anpfuel.app.ui.enterprise.trip

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.domain.enterprise.trip.TripNavigationTarget
import com.anpfuel.domain.enterprise.trip.TripNavigationUnsupported

/*
 * EMP-ROUTE-004 (B-BR-EMP-532..535, SCR-ROUTE-06): "Navegar" hands only the approved destination text to the app's own
 * route preview through [onNavigate]; it records, queues and sends nothing and never opens another app.
 */

private fun unsupportedText(reason: TripNavigationUnsupported): Int? = when (reason) {
    TripNavigationUnsupported.UNIT_DESTINATION -> R.string.enterprise_route_unsupported_unit
    TripNavigationUnsupported.NO_PLAN -> R.string.enterprise_route_unsupported_no_plan
    TripNavigationUnsupported.NO_MISSION -> R.string.enterprise_route_unsupported_no_mission
    TripNavigationUnsupported.MISSION_NOT_IN_COPY -> R.string.enterprise_route_unsupported_not_in_copy
    // Never offered for another partition's record; nothing is said about it.
    TripNavigationUnsupported.NOT_OWN -> null
}

@Composable
internal fun TripNavigationAction(target: TripNavigationTarget, busy: Boolean, onNavigate: (String) -> Unit) {
    when (target) {
        is TripNavigationTarget.Search -> {
            Notice(stringResource(R.string.enterprise_route_not_recorded), Modifier.testTag("enterprise-route-notice"))
            OutlinedButton(onClick = { onNavigate(target.query) }, enabled = !busy,
                modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-route-navigate")) {
                Text(stringResource(R.string.enterprise_route_navigate))
            }
        }
        is TripNavigationTarget.Unsupported -> unsupportedText(target.reason)?.let {
            Text(stringResource(it), modifier = Modifier.testTag("enterprise-route-unsupported"))
        }
    }
}
