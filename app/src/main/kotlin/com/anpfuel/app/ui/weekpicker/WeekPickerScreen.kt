package com.anpfuel.app.ui.weekpicker

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.BorderStroke
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.TaskAlt
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.AppErrorMapper
import com.anpfuel.app.mapper.SurveyWeekFormatter
import com.anpfuel.app.ui.accessibility.accessibilityDescription
import com.anpfuel.app.ui.accessibility.headingSemantics
import com.anpfuel.app.ui.components.ErrorState
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.components.SkeletonLine
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeek
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale

@Composable
fun WeekPickerRoute(
    onNavigateBack: (() -> Unit)?,
    onWeekSelected: () -> Unit = { onNavigateBack?.invoke() },
    modifier: Modifier = Modifier,
    viewModel: WeekPickerViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    LaunchedEffect(viewModel) {
        viewModel.navigation.collect { destination ->
            when (destination) {
                WeekPickerNavigation.Completed -> onWeekSelected()
            }
        }
    }

    WeekPickerScreen(
        uiState = uiState,
        onRetryCatalog = viewModel::loadCatalog,
        onUseLatestWeek = viewModel::useLatestWeek,
        onSelectWeek = viewModel::onWeekRowTapped,
        onDownloadWeek = viewModel::downloadWeek,
        onRetrySync = viewModel::retrySync,
        onCancelSync = viewModel::cancelSync,
        onNavigateBack = onNavigateBack,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WeekPickerScreen(
    uiState: WeekPickerUiState,
    onRetryCatalog: () -> Unit,
    onNavigateBack: (() -> Unit)? = null,
    onUseLatestWeek: (() -> Unit)? = null,
    onSelectWeek: ((SurveyWeekCatalogEntry) -> Unit)? = null,
    onDownloadWeek: ((SurveyWeekCatalogEntry) -> Unit)? = null,
    onRetrySync: (() -> Unit)? = null,
    onCancelSync: (() -> Unit)? = null,
    onSkip: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(text = stringResource(R.string.week_picker_title)) },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { innerPadding ->
        WeekPickerContent(
            uiState = uiState,
            onRetryCatalog = onRetryCatalog,
            onUseLatestWeek = onUseLatestWeek,
            onSelectWeek = onSelectWeek,
            onDownloadWeek = onDownloadWeek,
            onRetrySync = onRetrySync,
            onCancelSync = onCancelSync,
            onSkip = onSkip,
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding),
        )
    }
}

@Composable
fun WeekPickerContent(
    uiState: WeekPickerUiState,
    onRetryCatalog: () -> Unit,
    onUseLatestWeek: (() -> Unit)? = null,
    onSelectWeek: ((SurveyWeekCatalogEntry) -> Unit)? = null,
    onDownloadWeek: ((SurveyWeekCatalogEntry) -> Unit)? = null,
    onRetrySync: (() -> Unit)? = null,
    onCancelSync: (() -> Unit)? = null,
    onSkip: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    val locale = LocalConfiguration.current.locales[0]
    val sections = groupSurveyWeekCatalogByYear(uiState.catalog)
    val latestWeek = uiState.catalog.firstOrNull()?.surveyWeek

    Column(
        modifier = modifier
            .fillMaxHeight()
            .padding(horizontal = 16.dp, vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        when {
            uiState.isLoadingCatalog -> {
                WeekPickerLoadingSkeleton()
            }

            uiState.catalogError != null -> {
                ErrorState(
                    message = stringResource(AppErrorMapper.toStringRes(uiState.catalogError)),
                    onRetry = onRetryCatalog,
                    modifier = Modifier.fillMaxWidth(),
                )
            }

            else -> {
                if (
                    uiState.showLatestCta &&
                    onUseLatestWeek != null &&
                    uiState.catalog.isNotEmpty()
                ) {
                    Button(
                        onClick = onUseLatestWeek,
                        enabled = !uiState.isSyncing,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.week_picker_latest))
                    }
                }

                if (uiState.syncError != null && onRetrySync != null) {
                    ErrorState(
                        message = stringResource(AppErrorMapper.toStringRes(uiState.syncError)),
                        onRetry = onRetrySync,
                        modifier = Modifier.fillMaxWidth(),
                    )
                }

                LazyColumn(
                    modifier = Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    sections.forEach { section ->
                        item(key = "year-${section.year}") {
                            Text(
                                text = section.year.toString(),
                                style = MaterialTheme.typography.titleSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(top = 8.dp, bottom = 4.dp)
                                    .headingSemantics(),
                            )
                        }

                        items(
                            items = section.entries,
                            key = { entry -> entry.surveyWeek.endDate.toString() },
                        ) { entry ->
                            WeekPickerRow(
                                entry = entry,
                                isLatest = entry.surveyWeek == latestWeek,
                                isDownloaded = entry.surveyWeek in uiState.downloadedWeeks,
                                locale = locale,
                                onClick = onSelectWeek?.let { callback -> { callback(entry) } },
                                onDownload = onDownloadWeek?.let { callback -> { callback(entry) } },
                            )
                            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                        }
                    }
                }

                if (uiState.showSkipAction && onSkip != null) {
                    TextButton(
                        onClick = onSkip,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.onboarding_action_skip))
                    }
                }
            }
        }

        if (uiState.isSyncing && onCancelSync != null) {
            uiState.pendingWeekSelection?.surveyWeek?.let { downloadingWeek ->
                WeekDownloadingDialog(
                    weekLabel = SurveyWeekFormatter.formatRange(downloadingWeek, locale),
                    onCancel = onCancelSync,
                )
            }
        }
    }
}

@Composable
private fun WeekPickerLoadingSkeleton() {
    SkeletonGroup(modifier = Modifier.fillMaxWidth()) {
        repeat(3) {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                SkeletonLine(width = 180.dp, height = 20.dp)
                SkeletonLine(height = 16.dp)
                SkeletonLine(height = 16.dp)
                SkeletonLine(width = 140.dp, height = 14.dp)
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
        }
    }
}

/**
 * Compact tag sharing one standard across Atual/Baixada/Baixar: tight lateral
 * padding, small icon close to the label.
 */
@Composable
private fun WeekTag(
    label: String,
    icon: ImageVector?,
    containerColor: Color,
    contentColor: Color,
    border: BorderStroke?,
    onClick: (() -> Unit)?,
    modifier: Modifier = Modifier,
) {
    Surface(
        onClick = onClick ?: {},
        enabled = onClick != null,
        modifier = modifier,
        shape = RoundedCornerShape(8.dp),
        color = containerColor,
        contentColor = contentColor,
        border = border,
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            icon?.let {
                Icon(
                    imageVector = it,
                    contentDescription = null,
                    modifier = Modifier.size(16.dp),
                )
            }
            Text(
                text = label,
                style = MaterialTheme.typography.labelLarge,
                maxLines = 1,
                softWrap = false,
            )
        }
    }
}

@Composable
fun WeekDownloadingDialog(
    weekLabel: String,
    onCancel: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Dialog(
        onDismissRequest = onCancel,
        properties = DialogProperties(usePlatformDefaultWidth = false),
    ) {
        Surface(
            modifier = modifier
                .fillMaxWidth()
                .padding(horizontal = 24.dp),
            shape = RoundedCornerShape(28.dp),
            tonalElevation = 6.dp,
        ) {
            Column(
                modifier = Modifier.padding(24.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                CircularProgressIndicator(
                    modifier = Modifier
                        .size(48.dp)
                        .accessibilityDescription(stringResource(R.string.a11y_loading)),
                    color = MaterialTheme.colorScheme.primary,
                )
                Text(
                    text = stringResource(R.string.week_picker_downloading_title),
                    style = MaterialTheme.typography.titleLarge,
                    color = MaterialTheme.colorScheme.onSurface,
                    textAlign = TextAlign.Center,
                )
                Text(
                    text = stringResource(R.string.week_picker_downloading_message, weekLabel),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                )
                TextButton(onClick = onCancel) {
                    Text(text = stringResource(R.string.action_cancel))
                }
            }
        }
    }
}

@Composable
fun WeekPickerConfirmDialog(
    entry: SurveyWeekCatalogEntry,
    locale: Locale,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    val weekLabel = SurveyWeekFormatter.formatRange(entry.surveyWeek, locale)

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(text = stringResource(R.string.week_picker_confirm_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(text = stringResource(R.string.week_picker_confirm_message, weekLabel))
                entry.operationalNote?.let { note ->
                    OperationalNoteBanner(note = note)
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text(text = stringResource(R.string.week_picker_download_week))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text(text = stringResource(R.string.action_cancel))
            }
        },
    )
}

@Composable
fun WeekPickerRow(
    entry: SurveyWeekCatalogEntry,
    isLatest: Boolean,
    locale: Locale,
    onClick: (() -> Unit)?,
    modifier: Modifier = Modifier,
    isDownloaded: Boolean = false,
    onDownload: (() -> Unit)? = null,
) {
    Column(modifier = modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .then(
                    if (onClick != null) {
                        Modifier.clickable(onClick = onClick)
                    } else {
                        Modifier
                    },
                )
                .padding(vertical = 12.dp, horizontal = 8.dp),
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    text = SurveyWeekFormatter.formatRangeCompact(entry.surveyWeek, locale),
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.onSurface,
                    modifier = Modifier.weight(1f),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    if (isLatest) {
                        WeekTag(
                            label = stringResource(R.string.active_week_label),
                            icon = null,
                            containerColor = Color.Transparent,
                            contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
                            border = BorderStroke(
                                1.dp,
                                MaterialTheme.colorScheme.outline,
                            ),
                            onClick = onClick,
                        )
                    }
                    if (isDownloaded) {
                        WeekTag(
                            label = stringResource(R.string.week_picker_downloaded_tag),
                            icon = Icons.Default.TaskAlt,
                            containerColor = MaterialTheme.colorScheme.secondaryContainer,
                            contentColor = MaterialTheme.colorScheme.onSecondaryContainer,
                            border = null,
                            onClick = onClick,
                        )
                    } else {
                        WeekTag(
                            label = stringResource(R.string.week_picker_download_tag),
                            icon = Icons.Default.Download,
                            containerColor = MaterialTheme.colorScheme.primaryContainer,
                            contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
                            border = null,
                            onClick = onDownload ?: onClick,
                        )
                    }
                }
            }

            Text(
                text = "• ${stringResource(R.string.week_picker_weekly_summary_line)}",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(start = 8.dp, top = 8.dp),
            )

            Text(
                text = "• ${stringResource(R.string.week_picker_station_detail_line)}",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(start = 8.dp, top = 4.dp),
            )

            entry.publishedAt?.let { publishedAt ->
                val formatter = DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM).withLocale(locale)
                Text(
                    text = stringResource(
                        R.string.week_picker_updated_at,
                        publishedAt.format(formatter),
                    ),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(start = 16.dp, top = 4.dp),
                )
            }
        }

        entry.operationalNote?.let { note ->
            OperationalNoteBanner(
                note = note,
                modifier = Modifier.padding(start = 8.dp, end = 8.dp, bottom = 8.dp),
            )
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun WeekPickerContentPreview() {
    val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    val olderWeek = SurveyWeek.fromIsoDates("2025-12-28", "2026-01-03")
    AnpFuelTheme {
        WeekPickerScreen(
            uiState = WeekPickerUiState(
                catalog = listOf(
                    SurveyWeekCatalogEntry.create(
                        surveyWeek = week,
                        summaryUrl = "https://example.com/summary-latest.xlsx",
                        stationUrl = "https://example.com/station-latest.xlsx",
                        publishedAt = LocalDate.parse("2026-06-12"),
                    ),
                    SurveyWeekCatalogEntry.create(
                        surveyWeek = olderWeek,
                        summaryUrl = "https://example.com/summary-older.xlsx",
                        stationUrl = "https://example.com/station-older.xlsx",
                        publishedAt = LocalDate.parse("2026-01-02"),
                        operationalNote =
                            "Os preços médios de Belo Horizonte não foram publicados entre 26/04/2026 e 16/05/2026.",
                    ),
                ),
            ),
            onRetryCatalog = {},
            onNavigateBack = {},
            onUseLatestWeek = {},
        )
    }
}
