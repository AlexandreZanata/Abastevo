package com.anpfuel.app.ui.stationprofile

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.domain.profile.ProfileBadge
import com.anpfuel.domain.profile.ProfileBadgeInput
import com.anpfuel.domain.profile.ProfileBadgeRule

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun StationProfileScreen(
    onBack: () -> Unit,
    onClaim: (String) -> Unit,
    viewModel: StationProfileViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    Scaffold(topBar = {
        AnpTopAppBar(title = { Text(stringResource(R.string.station_profile_title)) }, onNavigateUp = onBack)
    }) { padding ->
        StationProfileContent(state, viewModel::refresh, { onClaim(viewModel.stationId) }, Modifier.padding(padding))
    }
}

@Composable
internal fun StationProfileContent(
    state: StationProfileState,
    onRetry: () -> Unit,
    onClaim: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        if (state.loading) CircularProgressIndicator()
        if (state.unavailable) {
            Text(stringResource(R.string.station_profile_unavailable))
            OutlinedButton(onClick = onRetry, enabled = !state.loading) { Text(stringResource(R.string.action_retry)) }
        }
        state.profile?.let { profile ->
            Text(profile.displayName, style = MaterialTheme.typography.headlineSmall, modifier = Modifier.semantics { heading() })
            val badge = ProfileBadgeRule.resolve(ProfileBadgeInput(
                profile.stationId, if (profile.hasBadge) "verified" else "unclaimed",
                profile.operatorSource, profile.hasBadge, state.stale,
            ))
            Text(stringResource(when (badge) {
                is ProfileBadge.Verified -> R.string.station_profile_verified
                ProfileBadge.StaleUnverified -> R.string.station_profile_stale
                else -> R.string.station_profile_unclaimed
            }))
            Text(stringResource(R.string.station_profile_representation_note))
            if (profile.business.isEmpty()) Text(stringResource(R.string.station_profile_empty))
            profile.business.forEach { (key, value) ->
                val label = when (key) {
                    "opening_hours" -> R.string.station_profile_hours
                    "services" -> R.string.station_profile_services
                    "phone" -> R.string.station_profile_phone
                    "website" -> R.string.station_profile_website
                    else -> R.string.station_profile_description
                }
                Text(stringResource(label), style = MaterialTheme.typography.titleSmall, modifier = Modifier.semantics { heading() })
                Text(value)
            }
            OutlinedButton(onClick = onClaim, modifier = Modifier.fillMaxWidth(), enabled = !state.stale && !state.loading) {
                Text(stringResource(R.string.station_profile_claim))
            }
        }
    }
}
