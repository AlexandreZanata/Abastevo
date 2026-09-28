package com.anpfuel.app.ui.settings

import android.content.Intent
import android.net.Uri
import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.AppErrorMapper
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.model.StorageUsage
import com.anpfuel.domain.model.UserPreferences
import java.text.NumberFormat

@Composable
fun SettingsScreen(
    onNavigateBack: (() -> Unit)? = null,
    onNavigateToOnboarding: () -> Unit,
    onNavigateToWeekPicker: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: SettingsViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val activity = LocalContext.current as? ComponentActivity
    val context = LocalContext.current

    LaunchedEffect(viewModel) {
        viewModel.effects.collect { effect ->
            when (effect) {
                SettingsEffect.RecreateActivity -> activity?.recreate()
                SettingsEffect.NavigateToOnboarding -> onNavigateToOnboarding()
            }
        }
    }

    SettingsContent(
        uiState = uiState,
        onNavigateBack = onNavigateBack,
        onLocaleSelected = viewModel::onLocaleSelected,
        onSyncStationDetailChanged = viewModel::onSyncStationDetailChanged,
        onAutoDownloadLatestWeekChanged = viewModel::onAutoDownloadLatestWeekChanged,
        onAutoSyncOnWifiChanged = viewModel::onAutoSyncOnWifiChanged,
        onShowPriceHistoryChanged = viewModel::onShowPriceHistoryChanged,
        onRetentionWeeksSelected = viewModel::onRetentionWeeksSelected,
        onNearestStationRadiusSelected = viewModel::onNearestStationRadiusSelected,
        onSyncNow = viewModel::syncNow,
        onClearStationCache = viewModel::clearStationCache,
        onRequestClearAllCache = viewModel::requestClearAllCache,
        onConfirmClearAllCache = viewModel::confirmClearAllCache,
        onDismissClearAllDialog = viewModel::dismissClearAllDialog,
        onRetry = viewModel::load,
        onNavigateToWeekPicker = onNavigateToWeekPicker,
        onOpenNotificationSettings = { viewModel.openNotificationSettings(context) },
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
private fun SettingsContent(
    uiState: SettingsUiState,
    onNavigateBack: (() -> Unit)? = null,
    onLocaleSelected: (String) -> Unit,
    onSyncStationDetailChanged: (Boolean) -> Unit,
    onAutoDownloadLatestWeekChanged: (Boolean) -> Unit,
    onAutoSyncOnWifiChanged: (Boolean) -> Unit,
    onShowPriceHistoryChanged: (Boolean) -> Unit,
    onRetentionWeeksSelected: (Int) -> Unit,
    onNearestStationRadiusSelected: (Int) -> Unit,
    onSyncNow: () -> Unit,
    onClearStationCache: () -> Unit,
    onRequestClearAllCache: () -> Unit,
    onConfirmClearAllCache: () -> Unit,
    onDismissClearAllDialog: () -> Unit,
    onRetry: () -> Unit,
    onNavigateToWeekPicker: () -> Unit,
    onOpenNotificationSettings: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    val numberFormat = NumberFormat.getIntegerInstance()

    if (uiState.showClearAllDialog) {
        AlertDialog(
            onDismissRequest = onDismissClearAllDialog,
            title = { Text(text = stringResource(R.string.settings_clear_cache)) },
            text = { Text(text = stringResource(R.string.settings_clear_cache_confirm)) },
            confirmButton = {
                TextButton(onClick = onConfirmClearAllCache) {
                    Text(text = stringResource(R.string.settings_clear_cache_confirm_action))
                }
            },
            dismissButton = {
                TextButton(onClick = onDismissClearAllDialog) {
                    Text(text = stringResource(R.string.action_back))
                }
            },
        )
    }

    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(
                onNavigateUp = onNavigateBack,
                title = { Text(text = stringResource(R.string.settings_title)) },
            )
        },
    ) { innerPadding ->
        when {
            uiState.isLoading -> {
                LoadingState(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(innerPadding),
                )
            }

            uiState.error != null && !uiState.hasLoaded -> {
                ErrorState(
                    message = stringResource(AppErrorMapper.toStringRes(uiState.error)),
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(innerPadding),
                    onRetry = onRetry,
                )
            }

            else -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(innerPadding)
                        .verticalScroll(rememberScrollState())
                        .padding(horizontal = 16.dp, vertical = 8.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    SettingsSection(title = stringResource(R.string.settings_language)) {
                        LanguageFlagGrid(
                            selectedLocaleTag = uiState.preferences.localeTag,
                            onLocaleSelected = onLocaleSelected,
                        )
                    }

                    SettingsSection(title = stringResource(R.string.settings_preferences_section)) {
                        SettingsToggleRow(
                            label = stringResource(R.string.settings_sync_station_detail),
                            checked = uiState.preferences.syncStationDetail,
                            onCheckedChange = onSyncStationDetailChanged,
                        )
                        SettingsToggleRow(
                            label = stringResource(R.string.settings_auto_download_latest_week),
                            checked = uiState.preferences.autoDownloadLatestWeek,
                            onCheckedChange = onAutoDownloadLatestWeekChanged,
                        )
                        SettingsToggleRow(
                            label = stringResource(R.string.settings_sync_wifi_only),
                            checked = uiState.preferences.autoSyncOnWifi,
                            onCheckedChange = onAutoSyncOnWifiChanged,
                        )
                        SettingsToggleRow(
                            label = stringResource(R.string.settings_show_price_history),
                            checked = uiState.preferences.showPriceHistory,
                            onCheckedChange = onShowPriceHistoryChanged,
                        )
                    }

                    if (uiState.showNotificationPermissionHint) {
                        SettingsSection(title = stringResource(R.string.settings_notifications_section)) {
                            Text(
                                text = stringResource(R.string.notification_permission_denied_hint),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                            OutlinedButton(
                                onClick = onOpenNotificationSettings,
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Text(text = stringResource(R.string.settings_open_notification_settings))
                            }
                        }
                    }

                    SettingsSection(title = stringResource(R.string.settings_retention_weeks)) {
                        FlowRow(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            for (weeks in SettingsViewModel.retentionWeekOptions) {
                                FilterChip(
                                    selected = uiState.preferences.stationDetailRetentionWeeks == weeks,
                                    onClick = { onRetentionWeeksSelected(weeks) },
                                    label = {
                                        Text(
                                            text = stringResource(
                                                R.string.settings_retention_weeks_option,
                                                weeks,
                                            ),
                                        )
                                    },
                                )
                            }
                        }
                    }

                    SettingsSection(title = stringResource(R.string.settings_nearest_station_radius)) {
                        FlowRow(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            for (radiusKm in UserPreferences.NEAREST_STATION_RADIUS_KM_OPTIONS) {
                                FilterChip(
                                    selected = uiState.preferences.nearestStationRadiusKm == radiusKm,
                                    onClick = { onNearestStationRadiusSelected(radiusKm) },
                                    label = {
                                        Text(
                                            text = stringResource(
                                                R.string.settings_nearest_station_radius_option,
                                                radiusKm,
                                            ),
                                        )
                                    },
                                )
                            }
                        }
                    }

                    SettingsSection(title = stringResource(R.string.settings_storage_usage)) {
                        Text(
                            text = stringResource(
                                R.string.settings_storage_summary_rows,
                                numberFormat.format(uiState.storageUsage.summaryRowCount),
                            ),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                        Text(
                            text = stringResource(
                                R.string.settings_storage_station_rows,
                                numberFormat.format(uiState.storageUsage.stationRowCount),
                            ),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                        Text(
                            text = stringResource(
                                R.string.settings_storage_week_count,
                                numberFormat.format(uiState.storageUsage.importedWeekCount),
                            ),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                    }

                    SettingsSection(title = stringResource(R.string.settings_sync_section)) {
                        Button(
                            onClick = onSyncNow,
                            enabled = !uiState.isSyncing,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(
                                text = stringResource(
                                    if (uiState.isSyncing) {
                                        R.string.banner_syncing
                                    } else {
                                        R.string.onboarding_action_sync_now
                                    },
                                ),
                            )
                        }
                        OutlinedButton(
                            onClick = onNavigateToWeekPicker,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(text = stringResource(R.string.settings_change_survey_week))
                        }
                        uiState.syncMessage?.let { messageKey ->
                            if (messageKey == SettingsViewModel.SYNC_COMPLETED_MESSAGE) {
                                Text(
                                    text = stringResource(R.string.sync_completed),
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.primary,
                                )
                            }
                        }
                    }

                    SettingsSection(title = stringResource(R.string.settings_share_section)) {
                        OutlinedButton(
                            onClick = {
                                val sendIntent = Intent().apply {
                                    action = Intent.ACTION_SEND
                                    putExtra(
                                        Intent.EXTRA_TEXT,
                                        context.getString(
                                            R.string.settings_share_app_message,
                                            context.getString(R.string.app_release_apk_url),
                                        ),
                                    )
                                    type = "text/plain"
                                }
                                val shareIntent = Intent.createChooser(sendIntent, null)
                                context.startActivity(shareIntent)
                            },
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(text = stringResource(R.string.settings_share_app))
                        }
                    }

                    SettingsSection(title = stringResource(R.string.settings_clear_cache)) {
                        OutlinedButton(
                            onClick = onClearStationCache,
                            enabled = !uiState.isClearingCache,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(text = stringResource(R.string.settings_clear_station_cache))
                        }
                        OutlinedButton(
                            onClick = onRequestClearAllCache,
                            enabled = !uiState.isClearingCache,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(text = stringResource(R.string.settings_clear_all_cache))
                        }
                    }

                    SettingsSection(
                        title = stringResource(R.string.settings_attribution_section),
                        showBottomDivider = false,
                    ) {
                        Text(
                            text = stringResource(R.string.prices_anp_attribution),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        TextButton(
                            onClick = {
                                val intent = Intent(
                                    Intent.ACTION_VIEW,
                                    Uri.parse(context.getString(R.string.anp_official_url)),
                                )
                                context.startActivity(intent)
                            },
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(text = stringResource(R.string.settings_anp_attribution_link))
                        }
                    }

                    uiState.error?.let { error ->
                        Text(
                            text = stringResource(AppErrorMapper.toStringRes(error)),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.error,
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun SettingsSection(
    title: String,
    modifier: Modifier = Modifier,
    showBottomDivider: Boolean = true,
    content: @Composable () -> Unit,
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            text = title,
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        content()
        if (showBottomDivider) {
            HorizontalDivider()
        }
    }
}

@Composable
private fun SettingsToggleRow(
    label: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        androidx.compose.foundation.layout.Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = label,
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier
                    .weight(1f)
                    .padding(end = 12.dp),
            )
            Switch(
                checked = checked,
                onCheckedChange = onCheckedChange,
            )
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun SettingsScreenPreview() {
    AnpFuelTheme {
        SettingsContent(
            uiState = SettingsUiState(
                isLoading = false,
                hasLoaded = true,
                preferences = UserPreferences(
                    localeTag = "en",
                    syncStationDetail = true,
                    autoDownloadLatestWeek = true,
                    autoSyncOnWifi = true,
                    showPriceHistory = true,
                    stationDetailRetentionWeeks = 12,
                ),
                storageUsage = StorageUsage(
                    summaryRowCount = 2344,
                    stationRowCount = 19676,
                    importedWeekCount = 3,
                ),
            ),
            onNavigateBack = {},
            onLocaleSelected = {},
            onSyncStationDetailChanged = {},
            onAutoDownloadLatestWeekChanged = {},
            onAutoSyncOnWifiChanged = {},
            onShowPriceHistoryChanged = {},
            onRetentionWeeksSelected = {},
            onNearestStationRadiusSelected = {},
            onSyncNow = {},
            onClearStationCache = {},
            onRequestClearAllCache = {},
            onConfirmClearAllCache = {},
            onDismissClearAllDialog = {},
            onRetry = {},
            onNavigateToWeekPicker = {},
            onOpenNotificationSettings = {},
        )
    }
}
