package com.anpfuel.app.ui.profile

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AssignmentTurnedIn
import androidx.compose.material.icons.filled.CalendarMonth
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.DarkMode
import androidx.compose.material.icons.filled.DirectionsCar
import androidx.compose.material.icons.filled.History
import androidx.compose.material.icons.filled.HelpOutline
import androidx.compose.material.icons.filled.LightMode
import androidx.compose.material.icons.filled.LocalGasStation
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.PersonOutline
import androidx.compose.material.icons.filled.QueryStats
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.auth.AuthStep
import com.anpfuel.app.ui.auth.AuthViewModel
import com.anpfuel.app.navigation.Routes
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.SkeletonButton
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.components.SkeletonLine

/**
 * P19-T04: Profile screen (Perfil).
 *
 * Implements the frozen IA Perfil hub (P19-T02 / B-BR-C05 / BUC-C01 / BUC-C03):
 * - Free account / guest exploration status and sign-in entry point
 * - Preserves all legacy expert tools under clearly named entries (none removed):
 *   - Vehicles (3 free vehicles, tank fill estimation) -> Routes.VEHICLES
 *   - ANP historical series -> Routes.HISTORY
 *   - Full price analysis table -> Routes.PRICES
 *   - Surveyed stations -> Routes.STATIONS
 *   - Survey week picker -> Routes.WEEK_PICKER
 *   - App settings & offline cache -> Routes.SETTINGS
 * - Rights & privacy guarantees (no public precise GPS, 24h photo expiry, no paid ranking)
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProfileScreen(
    darkTheme: Boolean,
    onToggleTheme: () -> Unit,
    onNavigate: (String) -> Unit,
    modifier: Modifier = Modifier,
    authViewModel: AuthViewModel = hiltViewModel(),
) {
    // P22-T01: same shared session store as the auth flow; rehydrate runs
    // again on resume so Perfil reflects shared sign-in/logout changes.
    val authState by authViewModel.uiState.collectAsStateWithLifecycle()
    androidx.lifecycle.compose.LifecycleEventEffect(androidx.lifecycle.Lifecycle.Event.ON_RESUME) {
        authViewModel.refreshAccount()
    }
    val signedIn = authState.step in listOf(AuthStep.AUTHENTICATED, AuthStep.OFFLINE_ACCOUNT)
    // Crossfade slot: a skeleton holds the account space while the session
    // resolves, so logged-out containers never flash before sign-in lands.
    val accountSlot = when {
        authState.step == AuthStep.CHECKING -> AccountSlot.RESOLVING
        signedIn -> AccountSlot.SIGNED_IN
        else -> AccountSlot.GUEST
    }
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                title = { Text(text = stringResource(R.string.profile_screen_title)) },
                actions = {
                    IconButton(onClick = onToggleTheme) {
                        Icon(
                            imageVector = if (darkTheme) Icons.Default.LightMode else Icons.Default.DarkMode,
                            contentDescription = stringResource(
                                if (darkTheme) R.string.action_switch_light_theme else R.string.action_switch_dark_theme,
                            ),
                        )
                    }
                },
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            // Account header crossfades between resolving skeleton, guest
            // entry and the signed-in summary; signed-in users still reach
            // full management via "Gerenciar conta" under expert tools.
            AnimatedContent(
                targetState = accountSlot,
                transitionSpec = { fadeIn() togetherWith fadeOut() },
                label = "profile-account",
            ) { slot ->
                when (slot) {
                    AccountSlot.RESOLVING -> {
                        Card(
                            modifier = Modifier.fillMaxWidth(),
                            colors = CardDefaults.cardColors(
                                containerColor = MaterialTheme.colorScheme.surfaceVariant,
                            ),
                        ) {
                            Column(
                                modifier = Modifier.padding(16.dp),
                                verticalArrangement = Arrangement.spacedBy(8.dp),
                            ) {
                                SkeletonGroup(modifier = Modifier.fillMaxWidth()) {
                                    SkeletonLine(width = 160.dp, height = 24.dp)
                                    SkeletonLine()
                                    SkeletonButton()
                                }
                            }
                        }
                    }
                    AccountSlot.GUEST -> {
                        Card(
                            modifier = Modifier.fillMaxWidth(),
                            colors = CardDefaults.cardColors(
                                containerColor = MaterialTheme.colorScheme.surfaceVariant,
                            ),
                        ) {
                            Column(
                                modifier = Modifier.padding(16.dp),
                                verticalArrangement = Arrangement.spacedBy(8.dp),
                            ) {
                                Text(
                                    text = stringResource(
                                        R.string.profile_guest_title,
                                    ),
                                    style = MaterialTheme.typography.titleMedium,
                                    modifier = Modifier.semantics { heading() },
                                )
                                Text(
                                    text = stringResource(
                                        R.string.profile_guest_subtitle,
                                    ),
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                                Button(
                                    onClick = {
                                        onNavigate(Routes.AUTH)
                                    },
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .padding(top = 4.dp),
                                ) {
                                    Text(
                                        text = stringResource(
                                            R.string.profile_action_sign_in,
                                        ),
                                    )
                                }
                            }
                        }
                    }
                    AccountSlot.SIGNED_IN -> {
                        Card(
                            modifier = Modifier.fillMaxWidth(),
                            colors = CardDefaults.cardColors(
                                containerColor = MaterialTheme.colorScheme.primaryContainer,
                            ),
                        ) {
                            Row(
                                modifier = Modifier.padding(16.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(12.dp),
                            ) {
                                Icon(
                                    imageVector = Icons.Default.PersonOutline,
                                    contentDescription = null,
                                    tint = MaterialTheme.colorScheme.onPrimaryContainer,
                                )
                                Column(
                                    modifier = Modifier.weight(1f),
                                    verticalArrangement = Arrangement.spacedBy(2.dp),
                                ) {
                                    Text(
                                        text = stringResource(R.string.profile_signed_in_title),
                                        style = MaterialTheme.typography.titleMedium,
                                        color = MaterialTheme.colorScheme.onPrimaryContainer,
                                        modifier = Modifier.semantics { heading() },
                                    )
                                    Text(
                                        text = stringResource(R.string.profile_signed_in_subtitle),
                                        style = MaterialTheme.typography.bodySmall,
                                        color = MaterialTheme.colorScheme.onPrimaryContainer,
                                    )
                                }
                            }
                        }
                    }
                }
            }

            // Preserved Expert Tools section (contributions now live here
            // as a dedicated page instead of an inline card).
            Text(
                text = stringResource(R.string.profile_expert_tools_section),
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.primary,
                modifier = Modifier
                    .padding(top = 8.dp)
                    .semantics { heading() },
            )

            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            ) {
                Column {
                    ProfileToolItem(
                        icon = Icons.Default.AssignmentTurnedIn,
                        title = stringResource(R.string.profile_tool_contributions_title),
                        subtitle = stringResource(R.string.profile_tool_contributions_subtitle),
                        onClick = { onNavigate(Routes.CONTRIBUTIONS) },
                    )
                    HorizontalDivider()
                    AnimatedVisibility(
                        visible = signedIn,
                        enter = fadeIn() + expandVertically(),
                        exit = fadeOut() + shrinkVertically(),
                    ) {
                        Column {
                            ProfileToolItem(
                                icon = Icons.Default.PersonOutline,
                                title = stringResource(R.string.profile_action_manage_account),
                                subtitle = stringResource(R.string.profile_signed_in_title),
                                onClick = { onNavigate(Routes.ACCOUNT) },
                            )
                            HorizontalDivider()
                        }
                    }
                    ProfileToolItem(
                        icon = Icons.Default.DirectionsCar,
                        title = stringResource(R.string.profile_tool_vehicles_title),
                        subtitle = stringResource(R.string.profile_tool_vehicles_subtitle),
                        onClick = { onNavigate(Routes.VEHICLES) },
                    )
                    HorizontalDivider()
                    ProfileToolItem(
                        icon = Icons.Default.History,
                        title = stringResource(R.string.profile_tool_history_title),
                        subtitle = stringResource(R.string.profile_tool_history_subtitle),
                        onClick = { onNavigate(Routes.HISTORY) },
                    )
                    HorizontalDivider()
                    ProfileToolItem(
                        icon = Icons.Default.QueryStats,
                        title = stringResource(R.string.profile_tool_prices_title),
                        subtitle = stringResource(R.string.profile_tool_prices_subtitle),
                        onClick = { onNavigate(Routes.PRICES) },
                    )
                    HorizontalDivider()
                    ProfileToolItem(
                        icon = Icons.Default.LocalGasStation,
                        title = stringResource(R.string.profile_tool_stations_title),
                        subtitle = stringResource(R.string.profile_tool_stations_subtitle),
                        onClick = { onNavigate(Routes.STATIONS) },
                    )
                    HorizontalDivider()
                    ProfileToolItem(
                        icon = Icons.Default.CalendarMonth,
                        title = stringResource(R.string.profile_tool_week_picker_title),
                        subtitle = stringResource(R.string.profile_tool_week_picker_subtitle),
                        onClick = { onNavigate(Routes.WEEK_PICKER) },
                    )
                    HorizontalDivider()
                    ProfileToolItem(
                        icon = Icons.Default.Settings,
                        title = stringResource(R.string.profile_tool_settings_title),
                        subtitle = stringResource(R.string.profile_tool_settings_subtitle),
                        onClick = { onNavigate(Routes.SETTINGS) },
                    )
                    HorizontalDivider()
                    ProfileToolItem(
                        icon = Icons.Default.HelpOutline,
                        title = stringResource(R.string.profile_tool_help_title),
                        subtitle = stringResource(R.string.profile_tool_help_subtitle),
                        onClick = { onNavigate(Routes.HELP) },
                    )
                }
            }

            // Rights and privacy section
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        Icon(
                            imageVector = Icons.Default.Lock,
                            contentDescription = null,
                            tint = MaterialTheme.colorScheme.primary,
                        )
                        Text(
                            text = stringResource(R.string.profile_privacy_section),
                            style = MaterialTheme.typography.titleSmall,
                            modifier = Modifier.semantics { heading() },
                        )
                    }
                    Text(
                        text = stringResource(R.string.profile_privacy_body),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

/** Account header slot for the profile crossfade. */
private enum class AccountSlot {
    RESOLVING,
    GUEST,
    SIGNED_IN,
}

@Composable
private fun ProfileToolItem(
    icon: ImageVector,
    title: String,
    subtitle: String,
    onClick: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            Text(
                text = title,
                style = MaterialTheme.typography.bodyLarge,
            )
            Text(
                text = subtitle,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Icon(
            imageVector = Icons.Default.ChevronRight,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.outline,
        )
    }
}
