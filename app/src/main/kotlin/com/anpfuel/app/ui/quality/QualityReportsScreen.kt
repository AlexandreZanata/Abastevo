package com.anpfuel.app.ui.quality

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.LocalGasStation
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.outlined.RateReview
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.ui.auth.SecureAccountWindow
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.FilterChip
import com.anpfuel.app.ui.designsystem.OutlinedButton
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.domain.quality.QualityReport
import com.anpfuel.domain.quality.QualityReportFailure
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * Owner's private quality reports (B-BR-FQ-07, BUC-FQ-05).
 *
 * Opened from a station it carries the report form; opened from Perfil it is
 * the history only, and an empty history explains that a report belongs to a
 * station and offers [onChooseStation]. Content cards follow the container
 * guidelines; report text never leaves memory and the window stays secure.
 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun QualityReportsScreen(
    onBack: () -> Unit,
    onSignIn: () -> Unit,
    viewModel: QualityReportsViewModel = hiltViewModel(),
    onChooseStation: () -> Unit = {},
) {
    SecureAccountWindow()
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { viewModel.resume() }
    LifecycleEventEffect(Lifecycle.Event.ON_STOP) { viewModel.pause() }
    val state by viewModel.state.collectAsStateWithLifecycle()
    var guidance by remember { mutableStateOf(false) }
    var browserFailed by remember { mutableStateOf(false) }
    val uri = LocalUriHandler.current
    AnpScaffold(
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.quality_reports_title)) },
                onNavigateUp = onBack,
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            item {
                ReportsCard {
                    Text(
                        text = stringResource(R.string.quality_reports_note),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    TextButton(onClick = { guidance = true }) {
                        Text(stringResource(R.string.quality_reports_anp))
                    }
                    if (browserFailed) {
                        Text(
                            text = stringResource(R.string.quality_reports_browser_failed),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.error,
                        )
                    }
                }
            }
            if (state.busy) item { LinearProgressIndicator(Modifier.fillMaxWidth()) }
            val error = state.error?.takeIf { state.visible }
            val signedOut = state.visible && state.owner == null
            if (error != null || signedOut) {
                item {
                    ReportsCard {
                        if (error != null) {
                            Text(
                                text = stringResource(failureText(error)),
                                style = MaterialTheme.typography.bodyLarge,
                                color = MaterialTheme.colorScheme.error,
                            )
                        }
                        if (signedOut) {
                            Button(onClick = onSignIn, modifier = Modifier.fillMaxWidth()) {
                                Text(stringResource(R.string.quality_reports_login))
                            }
                        }
                    }
                }
            }
            if (state.visible && (state.pending != null || state.receipt != null)) {
                item {
                    ReportsCard {
                        if (state.pending != null) {
                            Text(
                                text = stringResource(R.string.quality_reports_pending),
                                style = MaterialTheme.typography.bodyLarge,
                            )
                            Button(
                                onClick = viewModel::retry,
                                enabled = !state.busy,
                                modifier = Modifier.fillMaxWidth(),
                            ) { Text(stringResource(R.string.action_retry)) }
                        }
                        if (state.receipt != null) {
                            Text(
                                text = stringResource(R.string.quality_reports_received),
                                style = MaterialTheme.typography.bodyLarge,
                            )
                        }
                    }
                }
            }
            if (state.owner != null && state.visible) {
                val selected = state.selected
                val editable = !state.busy && state.pending == null && state.confirmation == null
                if (selected != null || !viewModel.historyOnly) {
                    item {
                        ReportsCard {
                            Text(
                                text = stringResource(
                                    if (selected == null && viewModel.linkageReview) R.string.quality_reports_link
                                    else R.string.quality_reports_experience,
                                ),
                                style = MaterialTheme.typography.titleMedium,
                                modifier = Modifier.semantics { heading() },
                            )
                            if (selected != null) {
                                StatusPill(stringResource(reportStatus(selected.status)))
                                Text(
                                    text = stringResource(evidenceText(selected.evidenceLevel)),
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                                if (selected.reviewReason.isNotEmpty()) {
                                    Text(
                                        text = stringResource(reviewReason(selected.reviewReason)),
                                        style = MaterialTheme.typography.bodyMedium,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    )
                                }
                            }
                            OutlinedTextField(
                                value = state.day,
                                onValueChange = viewModel::day,
                                label = { Text(stringResource(R.string.quality_reports_day)) },
                                enabled = editable,
                                singleLine = true,
                                modifier = Modifier.fillMaxWidth(),
                            )
                            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                SYMPTOMS.forEach { symptom ->
                                    FilterChip(
                                        selected = state.symptom == symptom,
                                        onClick = { viewModel.symptom(symptom) },
                                        enabled = editable,
                                        label = { Text(stringResource(symptomText(symptom))) },
                                    )
                                }
                            }
                            OutlinedTextField(
                                value = state.text,
                                onValueChange = viewModel::text,
                                label = { Text(stringResource(R.string.quality_reports_text)) },
                                supportingText = { Text(stringResource(R.string.quality_reports_text_limit)) },
                                enabled = editable,
                                minLines = 3,
                                maxLines = 8,
                                modifier = Modifier.fillMaxWidth(),
                            )
                            val canSend = selected == null && viewModel.canSubmit ||
                                selected != null && viewModel.canEdit && selected.status in setOf("PENDING_REVIEW", "REVIEWED")
                            if (canSend) {
                                Button(
                                    onClick = { viewModel.request(if (selected == null) "SUBMIT" else "EDIT") },
                                    enabled = editable,
                                    modifier = Modifier.fillMaxWidth(),
                                ) {
                                    Text(stringResource(if (selected == null) R.string.quality_reports_submit else R.string.quality_reports_edit))
                                }
                            }
                            if (selected != null && selected.status in setOf("HIDDEN", "REVIEWED")) {
                                OutlinedButton(
                                    onClick = { viewModel.request("APPEAL") },
                                    enabled = editable,
                                    modifier = Modifier.fillMaxWidth(),
                                ) { Text(stringResource(R.string.quality_reports_appeal)) }
                            }
                            if (selected != null && selected.status in setOf("PENDING_REVIEW", "REVIEWED", "HIDDEN")) {
                                OutlinedButton(
                                    onClick = { viewModel.request("WITHDRAW") },
                                    enabled = editable,
                                    modifier = Modifier.fillMaxWidth(),
                                ) { Text(stringResource(R.string.quality_reports_withdraw)) }
                            }
                            if (selected != null && !viewModel.historyOnly && viewModel.canSubmit) {
                                TextButton(onClick = viewModel::newReport, enabled = editable) {
                                    Text(stringResource(R.string.quality_reports_new))
                                }
                            }
                        }
                    }
                }
                item {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            text = stringResource(R.string.quality_reports_own),
                            style = MaterialTheme.typography.titleMedium,
                            modifier = Modifier.weight(1f).semantics { heading() },
                        )
                        IconButton(onClick = viewModel::resume, enabled = !state.busy) {
                            Icon(
                                Icons.Default.Refresh,
                                contentDescription = stringResource(R.string.quality_reports_refresh),
                            )
                        }
                    }
                }
                if (state.reports.isEmpty() && !state.busy) {
                    item {
                        EmptyReports(
                            historyOnly = viewModel.historyOnly,
                            onChooseStation = onChooseStation,
                        )
                    }
                }
                items(state.reports, key = { it.id }) { report ->
                    ReportRow(
                        report = report,
                        selected = report.id == selected?.id,
                        enabled = editable,
                        onClick = { viewModel.select(report) },
                    )
                }
                if (state.nextAfterId != null) {
                    item {
                        OutlinedButton(
                            onClick = viewModel::nextPage,
                            enabled = !state.busy && state.pending == null,
                            modifier = Modifier.fillMaxWidth(),
                        ) { Text(stringResource(R.string.quality_reports_more)) }
                    }
                }
            }
        }
    }
    state.confirmation?.takeIf { state.visible }?.let { command ->
        AlertDialog(
            onDismissRequest = viewModel::dismissConfirmation,
            title = { Text(stringResource(R.string.quality_reports_confirm_title)) },
            text = {
                Text(
                    stringResource(
                        when (command.action) {
                            "WITHDRAW" -> R.string.quality_reports_confirm_withdraw
                            "APPEAL" -> R.string.quality_reports_confirm_appeal
                            else -> R.string.quality_reports_confirm_submit
                        },
                    ),
                )
            },
            confirmButton = {
                TextButton(onClick = viewModel::confirm) { Text(stringResource(R.string.quality_reports_confirm)) }
            },
            dismissButton = {
                TextButton(onClick = viewModel::dismissConfirmation) { Text(stringResource(R.string.action_cancel)) }
            },
        )
    }
    if (guidance) {
        AlertDialog(
            onDismissRequest = { guidance = false },
            title = { Text(stringResource(R.string.quality_reports_anp)) },
            text = { Text(stringResource(R.string.quality_reports_anp_handoff)) },
            confirmButton = {
                TextButton(onClick = {
                    guidance = false
                    browserFailed = runCatching { uri.openUri(ANP_CHANNELS_URL) }.isFailure
                }) { Text(stringResource(R.string.quality_reports_open)) }
            },
            dismissButton = {
                TextButton(onClick = { guidance = false }) { Text(stringResource(R.string.action_cancel)) }
            },
        )
    }
}

/** Standard content container for this screen (UI_CONTAINER_GUIDELINES). */
@Composable
private fun ReportsCard(
    modifier: Modifier = Modifier,
    containerColor: Color = MaterialTheme.colorScheme.surfaceContainerLow,
    content: @Composable () -> Unit,
) {
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) { content() }
    }
}

@Composable
private fun EmptyReports(historyOnly: Boolean, onChooseStation: () -> Unit) {
    ReportsCard {
        Column(
            modifier = Modifier.fillMaxWidth(),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Surface(
                shape = CircleShape,
                color = MaterialTheme.colorScheme.surfaceContainerHigh,
                modifier = Modifier.size(56.dp),
            ) {
                Box(contentAlignment = Alignment.Center) {
                    Icon(
                        Icons.Outlined.RateReview,
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Text(
                text = stringResource(R.string.quality_reports_empty),
                style = MaterialTheme.typography.titleMedium,
                textAlign = TextAlign.Center,
            )
            Text(
                text = stringResource(
                    if (historyOnly) R.string.quality_reports_empty_how
                    else R.string.quality_reports_empty_here,
                ),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )
            if (historyOnly) {
                Button(onClick = onChooseStation, modifier = Modifier.fillMaxWidth()) {
                    Icon(Icons.Default.LocalGasStation, null, Modifier.padding(end = 8.dp))
                    Text(stringResource(R.string.quality_reports_choose_station))
                }
            }
        }
    }
}

@Composable
private fun ReportRow(report: QualityReport, selected: Boolean, enabled: Boolean, onClick: () -> Unit) {
    val containerColor =
        if (selected) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        onClick = onClick,
        enabled = enabled,
        modifier = Modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(
            containerColor = containerColor,
            disabledContainerColor = containerColor,
            disabledContentColor = MaterialTheme.colorScheme.onSurface,
        ),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    text = displayDate(report.occurredOn),
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.weight(1f),
                )
                StatusPill(stringResource(reportStatus(report.status)))
            }
            WireFuelMapper.fromWire(report.fuel).getOrNull()?.let { product ->
                Text(
                    text = stringResource(FuelProductI18n.toStringRes(product)) + " · " + stringResource(symptomText(report.symptom)),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (report.text.isNotBlank()) {
                Text(
                    text = report.text,
                    style = MaterialTheme.typography.bodyLarge,
                    maxLines = 4,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            Text(
                text = stringResource(evidenceText(report.evidenceLevel)),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun StatusPill(label: String) {
    Surface(shape = CircleShape, color = MaterialTheme.colorScheme.surfaceContainerHighest) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelMedium,
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
        )
    }
}

/** ISO day shown in the device locale; an unparseable value is shown as received. */
internal fun displayDate(isoDay: String): String = runCatching {
    LocalDate.parse(isoDay).format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM))
}.getOrDefault(isoDay)

private const val ANP_CHANNELS_URL = "https://www.gov.br/anp/pt-br/canais_atendimento/ouvidoria"
private val SYMPTOMS = listOf("STARTING", "PERFORMANCE", "CONSUMPTION", "OTHER")

private fun symptomText(symptom: String): Int = when (symptom) {
    "STARTING" -> R.string.quality_reports_starting
    "PERFORMANCE" -> R.string.quality_reports_performance
    "CONSUMPTION" -> R.string.quality_reports_consumption
    else -> R.string.quality_reports_other
}

private fun failureText(error: QualityReportFailure): Int = when (error) {
    QualityReportFailure.INVALID -> R.string.quality_reports_invalid
    QualityReportFailure.AUTH_REQUIRED, QualityReportFailure.OWNER_CHANGED -> R.string.quality_reports_account
    QualityReportFailure.CONFLICT -> R.string.quality_reports_conflict
    QualityReportFailure.NOT_FOUND -> R.string.quality_reports_missing
    QualityReportFailure.QUOTA -> R.string.quality_reports_quota
    else -> R.string.quality_reports_failed
}

private fun evidenceText(level: String): Int =
    if (level == "USER_CLAIM") R.string.quality_reports_claim else R.string.quality_reports_evidence_unknown

private fun reportStatus(status: String): Int = when (status) {
    "PENDING_REVIEW" -> R.string.quality_reports_status_pending
    "REVIEWED" -> R.string.quality_reports_status_reviewed
    "HIDDEN" -> R.string.quality_reports_status_hidden
    "WITHDRAWN" -> R.string.quality_reports_status_withdrawn
    else -> R.string.quality_reports_status_unknown
}

private fun reviewReason(reason: String): Int = when (reason) {
    "TRIAGE" -> R.string.quality_reports_reason_triage
    "CLAIM_RECORDED" -> R.string.quality_reports_reason_recorded
    "DUPLICATE" -> R.string.quality_reports_reason_duplicate
    "INSUFFICIENT_EVIDENCE" -> R.string.quality_reports_reason_insufficient
    "UNSUPPORTED_LINK" -> R.string.quality_reports_reason_link
    "ABUSE" -> R.string.quality_reports_reason_abuse
    "PRIVACY" -> R.string.quality_reports_reason_privacy
    else -> R.string.quality_reports_reason_unknown
}
