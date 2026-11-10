package com.anpfuel.app.ui.terms

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.designsystem.OutlinedButton
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.usecase.legal.TermsStatus
import com.anpfuel.domain.legal.TermsAcceptance
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * Privacy > stored first-use acceptance (B-BR-TA04): what was accepted, when,
 * and the revocation that returns the app to the terms screen.
 */
@Composable
fun TermsAcceptanceCard(
    onOpen: (String) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: TermsGateViewModel = hiltViewModel(),
) {
    val status by viewModel.status.collectAsStateWithLifecycle()
    val record = (status as? TermsStatus.Accepted)?.record ?: return
    TermsAcceptanceCardContent(record = record, onOpen = onOpen, onRevoke = viewModel::revoke, modifier = modifier)
}

@Composable
internal fun TermsAcceptanceCardContent(
    record: TermsAcceptance,
    onOpen: (String) -> Unit,
    onRevoke: () -> Boolean,
    modifier: Modifier = Modifier,
) {
    var confirming by rememberSaveable { mutableStateOf(false) }
    var revokeFailed by rememberSaveable { mutableStateOf(false) }
    var failedUrl by rememberSaveable { mutableStateOf<String?>(null) }
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = CardDefaults.shape,
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                text = stringResource(R.string.terms_record_heading),
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.semantics { heading() },
            )
            Text(
                text = stringResource(R.string.terms_record_accepted, acceptedAtText(record.acceptedAtMillis)),
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("terms-record"),
            )
            Text(
                text = stringResource(R.string.terms_gate_versions, record.termsVersion, record.privacyVersion),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            TextButton(
                onClick = {
                    failedUrl = null
                    try {
                        onOpen(TermsDocumentLinks.TERMS)
                    } catch (_: IllegalArgumentException) {
                        failedUrl = TermsDocumentLinks.TERMS
                    }
                },
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(stringResource(R.string.terms_gate_read_terms), modifier = Modifier.weight(1f))
                Icon(Icons.AutoMirrored.Filled.OpenInNew, contentDescription = null)
            }
            failedUrl?.let { url ->
                Text(
                    text = stringResource(R.string.terms_gate_open_failed, url),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                )
            }
            OutlinedButton(
                onClick = { confirming = true },
                modifier = Modifier.fillMaxWidth().testTag("terms-revoke"),
            ) { Text(stringResource(R.string.terms_revoke)) }
            if (revokeFailed) {
                Text(
                    text = stringResource(R.string.terms_revoke_failed),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                )
            }
        }
    }
    if (confirming) {
        AlertDialog(
            onDismissRequest = { confirming = false },
            title = { Text(stringResource(R.string.terms_revoke_title)) },
            text = { Text(stringResource(R.string.terms_revoke_body)) },
            confirmButton = {
                TextButton(onClick = {
                    confirming = false
                    revokeFailed = !onRevoke()
                }) { Text(stringResource(R.string.terms_revoke_confirm)) }
            },
            dismissButton = {
                TextButton(onClick = { confirming = false }) { Text(stringResource(R.string.action_cancel)) }
            },
        )
    }
}

/** Local date and time of the stored instant, in the device locale and zone. */
internal fun acceptedAtText(epochMillis: Long, zone: ZoneId = ZoneId.systemDefault()): String =
    DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT)
        .format(Instant.ofEpochMilli(epochMillis).atZone(zone))
