package com.anpfuel.app.ui.profile

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
import androidx.compose.material.icons.filled.CalendarMonth
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.DarkMode
import androidx.compose.material.icons.filled.DirectionsCar
import androidx.compose.material.icons.filled.History
import androidx.compose.material.icons.filled.HelpOutline
import androidx.compose.material.icons.filled.LightMode
import androidx.compose.material.icons.filled.LocalGasStation
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.QueryStats
import androidx.compose.material.icons.filled.Settings
import android.content.Intent
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
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.AnnotatedString
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
    statusViewModel: ContributionStatusViewModel = hiltViewModel(),
    authViewModel: AuthViewModel = hiltViewModel(),
) {
    val statusState by statusViewModel.uiState.collectAsStateWithLifecycle()
    LaunchedEffect(statusViewModel) {
        statusViewModel.load()
    }
    // P22-T01: same shared session store as the auth flow; rehydrate runs
    // once here so Perfil reflects sign-in without duplicating ceremony.
    val authState by authViewModel.uiState.collectAsStateWithLifecycle()
    val signedIn = authState.step == AuthStep.AUTHENTICATED
    LaunchedEffect(authViewModel, signedIn) {
        if (signedIn) authViewModel.loadBackup()
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
            // Account section
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
                            if (signedIn) {
                                R.string.profile_signed_in_title
                            } else {
                                R.string.profile_guest_title
                            },
                        ),
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.semantics { heading() },
                    )
                    Text(
                        text = stringResource(
                            if (signedIn) {
                                R.string.profile_signed_in_subtitle
                            } else {
                                R.string.profile_guest_subtitle
                            },
                        ),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Button(
                        onClick = { onNavigate(Routes.AUTH) },
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(top = 4.dp),
                    ) {
                        Text(
                            text = stringResource(
                                if (signedIn) {
                                    R.string.profile_action_manage_account
                                } else {
                                    R.string.profile_action_sign_in
                                },
                            ),
                        )
                    }
                }
            }

            // Account-key backup: the only login secret, shown for saving
            // elsewhere (WhatsApp/safe place) plus one-tap copy.
            if (signedIn) {
                authState.keyBackup?.let { backup ->
                    AccountKeyBackupCard(backup = backup)
                }
            }

            // P21-T03 private owner status section
            Text(
                text = stringResource(R.string.profile_contributions_section),
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.primary,
                modifier = Modifier.semantics { heading() },
            )
            ContributionStatusCard(
                state = statusState,
                onRetry = statusViewModel::load,
                onCancel = statusViewModel::onCancel,
                modifier = Modifier.fillMaxWidth(),
            )

            // Preserved Expert Tools section
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

/**
 * Account-key backup card: username plus the only login secret with
 * reveal, one-tap copy and system share (WhatsApp/safe place). The key
 * renders only here and on its one-display signup screen.
 */
@Composable
private fun AccountKeyBackupCard(
    backup: com.anpfuel.application.portable.AuthFlow.KeyBackup,
    modifier: Modifier = Modifier,
) {
    var revealed by remember { mutableStateOf(false) }
    val clipboard = LocalClipboardManager.current
    val context = LocalContext.current
    Card(
        modifier = modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceVariant,
        ),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = stringResource(R.string.profile_backup_title),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.semantics { heading() },
            )
            Text(
                text = stringResource(R.string.profile_backup_subtitle),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = stringResource(R.string.profile_backup_username, backup.username),
                style = MaterialTheme.typography.bodyMedium,
            )
            Text(
                text = if (revealed) {
                    backup.accountKey.chunked(4).joinToString(" ")
                } else {
                    "•••• •••• •••• ••••"
                },
                style = MaterialTheme.typography.titleMedium,
            )
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                OutlinedButton(onClick = { revealed = !revealed }) {
                    Text(
                        text = stringResource(
                            if (revealed) {
                                R.string.profile_backup_hide
                            } else {
                                R.string.profile_backup_show
                            },
                        ),
                    )
                }
                OutlinedButton(
                    onClick = {
                        clipboard.setText(AnnotatedString(backup.accountKey))
                    },
                ) {
                    Text(text = stringResource(R.string.profile_backup_copy))
                }
                Button(
                    onClick = {
                        val message = context.getString(
                            R.string.profile_backup_share_text,
                            backup.username,
                            backup.accountKey,
                        )
                        val send = Intent(Intent.ACTION_SEND).apply {
                            type = "text/plain"
                            putExtra(Intent.EXTRA_TEXT, message)
                        }
                        context.startActivity(Intent.createChooser(send, null))
                    },
                ) {
                    Text(text = stringResource(R.string.profile_backup_share))
                }
            }
        }
    }
}
