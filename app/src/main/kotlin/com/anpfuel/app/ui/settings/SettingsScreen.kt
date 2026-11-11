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
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CalendarMonth
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.LocationCity
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.OpenInNew
import androidx.compose.material.icons.filled.PersonOutline
import androidx.compose.material.icons.filled.Share
import androidx.compose.material.icons.filled.ShowChart
import androidx.compose.material.icons.filled.Storage
import androidx.compose.material.icons.filled.Storefront
import androidx.compose.material.icons.filled.Sync
import androidx.compose.material.icons.filled.Wifi
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.AppErrorMapper
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.SettingsDivider
import com.anpfuel.app.ui.components.SettingsGroup
import com.anpfuel.app.ui.components.SettingsRow
import com.anpfuel.app.ui.components.SettingsSwitchRow
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.components.SkeletonLine
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.FilterChip
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.model.StorageUsage
import com.anpfuel.domain.model.UserPreferences
import java.text.NumberFormat

@Composable
fun SettingsScreen(
    onNavigateBack: (() -> Unit)? = null,
    onNavigateToOnboarding: () -> Unit,
    onNavigateToWeekPicker: () -> Unit,
    onNavigateToAuth: () -> Unit = {},
    onNavigateToPrivacy: () -> Unit = {},
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
        onSyncPublicCityCatalogChanged = { viewModel.onSyncPublicCityCatalogChanged(it, context) },
        onClearPublicCatalog = viewModel::clearPublicCatalog,
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
        onNavigateToAuth = onNavigateToAuth,
        onNavigateToPrivacy = onNavigateToPrivacy,
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
    onSyncPublicCityCatalogChanged: (Boolean) -> Unit,
    onClearPublicCatalog: () -> Unit,
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
    onNavigateToAuth: () -> Unit = {},
    onNavigateToPrivacy: () -> Unit = {},
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
                SkeletonGroup(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(innerPadding)
                        .padding(horizontal = 16.dp, vertical = 8.dp),
                ) {
                    SkeletonLine(width = 160.dp, height = 20.dp)
                    SkeletonCard(height = 120.dp)
                    SkeletonLine(width = 160.dp, height = 20.dp)
                    SkeletonCard(height = 180.dp)
                }
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
                        .padding(horizontal = 16.dp, vertical = 16.dp),
                    verticalArrangement = Arrangement.spacedBy(24.dp),
                ) {
                    // Sync first: it is the action people open this page for.
                    SettingsGroup(title = stringResource(R.string.settings_sync_section)) {
                        Column(
                            modifier = Modifier.padding(16.dp),
                            verticalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            Button(
                                onClick = onSyncNow,
                                enabled = !uiState.isSyncing,
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Icon(
                                    imageVector = Icons.Default.Sync,
                                    contentDescription = null,
                                    modifier = Modifier.size(20.dp),
                                )
                                Text(
                                    text = stringResource(
                                        if (uiState.isSyncing) R.string.banner_syncing else R.string.onboarding_action_sync_now,
                                    ),
                                    modifier = Modifier.padding(start = 8.dp),
                                    maxLines = 1,
                                )
                            }
                            uiState.syncMessage?.let { messageKey ->
                                if (messageKey == SettingsViewModel.SYNC_COMPLETED_MESSAGE) {
                                    SettingsNote(stringResource(R.string.sync_completed))
                                }
                            }
                        }
                        SettingsDivider()
                        SettingsRow(
                            icon = Icons.Default.CalendarMonth,
                            title = stringResource(R.string.settings_change_survey_week),
                            onClick = onNavigateToWeekPicker,
                        )
                    }

                    SettingsGroup(title = stringResource(R.string.settings_preferences_section)) {
                        SettingsSwitchRow(
                            icon = Icons.Default.Storefront,
                            title = stringResource(R.string.settings_sync_station_detail),
                            checked = uiState.preferences.syncStationDetail,
                            onCheckedChange = onSyncStationDetailChanged,
                        )
                        SettingsDivider()
                        SettingsSwitchRow(
                            icon = Icons.Default.Download,
                            title = stringResource(R.string.settings_auto_download_latest_week),
                            checked = uiState.preferences.autoDownloadLatestWeek,
                            onCheckedChange = onAutoDownloadLatestWeekChanged,
                        )
                        SettingsDivider()
                        SettingsSwitchRow(
                            icon = Icons.Default.Wifi,
                            title = stringResource(R.string.settings_sync_wifi_only),
                            checked = uiState.preferences.autoSyncOnWifi,
                            onCheckedChange = onAutoSyncOnWifiChanged,
                        )
                        SettingsDivider()
                        SettingsSwitchRow(
                            icon = Icons.Default.ShowChart,
                            title = stringResource(R.string.settings_show_price_history),
                            checked = uiState.preferences.showPriceHistory,
                            onCheckedChange = onShowPriceHistoryChanged,
                        )
                    }

                    if (uiState.showNotificationPermissionHint) {
                        SettingsGroup(title = stringResource(R.string.settings_notifications_section)) {
                            SettingsRow(
                                icon = Icons.Default.Notifications,
                                title = stringResource(R.string.settings_open_notification_settings),
                                subtitle = stringResource(R.string.notification_permission_denied_hint),
                                onClick = onOpenNotificationSettings,
                            )
                        }
                    }

                    SettingsGroup(title = stringResource(R.string.settings_language)) {
                        Column(modifier = Modifier.padding(12.dp)) {
                            LanguageFlagGrid(
                                selectedLocaleTag = uiState.preferences.localeTag,
                                onLocaleSelected = onLocaleSelected,
                            )
                        }
                    }

                    SettingsGroup(title = stringResource(R.string.settings_section_search_data)) {
                        SettingsChoice(
                            title = stringResource(R.string.settings_nearest_station_radius),
                            options = UserPreferences.NEAREST_STATION_RADIUS_KM_OPTIONS,
                            selected = uiState.preferences.nearestStationRadiusKm,
                            label = { stringResource(R.string.settings_nearest_station_radius_option, it) },
                            onSelected = onNearestStationRadiusSelected,
                        )
                        SettingsDivider()
                        SettingsChoice(
                            title = stringResource(R.string.settings_retention_weeks),
                            options = SettingsViewModel.retentionWeekOptions,
                            selected = uiState.preferences.stationDetailRetentionWeeks,
                            label = { stringResource(R.string.settings_retention_weeks_option, it) },
                            onSelected = onRetentionWeeksSelected,
                        )
                    }

                    SettingsGroup(title = stringResource(R.string.settings_public_catalog_section)) {
                        SettingsSwitchRow(
                            icon = Icons.Default.LocationCity,
                            title = stringResource(R.string.settings_public_catalog_toggle),
                            subtitle = stringResource(R.string.settings_public_catalog_description),
                            checked = uiState.preferences.syncPublicCityCatalog,
                            onCheckedChange = onSyncPublicCityCatalogChanged,
                        )
                        val lastSynced = uiState.preferences.publicCityCatalogLastSyncedAtMillis
                        val statusText = if (uiState.preferences.syncPublicCityCatalog && lastSynced != null) {
                            val formatted = remember(lastSynced) {
                                java.text.DateFormat.getDateTimeInstance().format(java.util.Date(lastSynced))
                            }
                            stringResource(R.string.settings_public_catalog_last_sync, formatted)
                        } else if (uiState.preferences.syncPublicCityCatalog) {
                            stringResource(R.string.settings_public_catalog_never_synced)
                        } else {
                            stringResource(R.string.settings_public_catalog_off_note)
                        }
                        Column(
                            modifier = Modifier.padding(start = 54.dp, end = 16.dp, bottom = 12.dp),
                            verticalArrangement = Arrangement.spacedBy(2.dp),
                        ) {
                            SettingsNote(statusText)
                            SettingsNote(
                                stringResource(
                                    R.string.settings_public_catalog_size,
                                    android.text.format.Formatter.formatShortFileSize(
                                        context,
                                        uiState.storageUsage.publicCatalogEstimatedBytes,
                                    ),
                                ),
                            )
                            uiState.syncMessage?.let { messageKey ->
                                if (messageKey == SettingsViewModel.PUBLIC_CATALOG_CLEARED_MESSAGE) {
                                    SettingsNote(stringResource(R.string.public_catalog_cleared), emphasized = true)
                                }
                            }
                        }
                        SettingsDivider()
                        SettingsRow(
                            icon = Icons.Default.Storage,
                            title = stringResource(R.string.settings_clear_public_catalog),
                            onClick = onClearPublicCatalog,
                            enabled = !uiState.isClearingPublicCatalog,
                        )
                    }

                    SettingsGroup(title = stringResource(R.string.settings_storage_usage)) {
                        SettingsStat(
                            stringResource(
                                R.string.settings_storage_summary_rows,
                                numberFormat.format(uiState.storageUsage.summaryRowCount),
                            ),
                        )
                        SettingsStat(
                            stringResource(
                                R.string.settings_storage_station_rows,
                                numberFormat.format(uiState.storageUsage.stationRowCount),
                            ),
                        )
                        SettingsStat(
                            stringResource(
                                R.string.settings_storage_week_count,
                                numberFormat.format(uiState.storageUsage.importedWeekCount),
                            ),
                        )
                        SettingsDivider()
                        SettingsRow(
                            icon = Icons.Default.Storage,
                            title = stringResource(R.string.settings_clear_station_cache),
                            onClick = onClearStationCache,
                            enabled = !uiState.isClearingCache,
                        )
                        SettingsDivider()
                        SettingsRow(
                            icon = Icons.Default.DeleteOutline,
                            title = stringResource(R.string.settings_clear_all_cache),
                            onClick = onRequestClearAllCache,
                            enabled = !uiState.isClearingCache,
                            destructive = true,
                        )
                    }

                    SettingsGroup(title = stringResource(R.string.settings_section_account_privacy)) {
                        SettingsRow(
                            icon = Icons.Default.PersonOutline,
                            title = stringResource(R.string.auth_account_row),
                            onClick = onNavigateToAuth,
                        )
                        SettingsDivider()
                        SettingsRow(
                            icon = Icons.Default.Lock,
                            title = stringResource(R.string.settings_privacy_title),
                            subtitle = stringResource(R.string.settings_privacy_subtitle),
                            onClick = onNavigateToPrivacy,
                        )
                    }

                    SettingsGroup(title = stringResource(R.string.settings_section_about)) {
                        SettingsRow(
                            icon = Icons.Default.Share,
                            title = stringResource(R.string.settings_share_app),
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
                                context.startActivity(Intent.createChooser(sendIntent, null))
                            },
                        )
                        SettingsDivider()
                        SettingsRow(
                            icon = Icons.Default.OpenInNew,
                            title = stringResource(R.string.settings_anp_attribution_link),
                            subtitle = stringResource(R.string.prices_anp_attribution),
                            onClick = {
                                val intent = Intent(
                                    Intent.ACTION_VIEW,
                                    Uri.parse(context.getString(R.string.anp_official_url)),
                                )
                                context.startActivity(intent)
                            },
                        )
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
private fun SettingsNote(text: String, emphasized: Boolean = false) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodySmall,
        color = if (emphasized) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

/** Read-only line inside a group, aligned with the row text. */
@Composable
private fun SettingsStat(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurface,
        modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 10.dp, bottom = 2.dp),
    )
}

/** Single-choice option set inside a group: a title and pill chips. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun SettingsChoice(
    title: String,
    options: List<Int>,
    selected: Int,
    label: @Composable (Int) -> String,
    onSelected: (Int) -> Unit,
) {
    Column(
        modifier = Modifier.padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Text(
            text = title,
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurface,
        )
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            for (option in options) {
                FilterChip(
                    selected = selected == option,
                    onClick = { onSelected(option) },
                    label = { Text(text = label(option)) },
                )
            }
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
            onSyncPublicCityCatalogChanged = {},
            onClearPublicCatalog = {},
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
