package com.anpfuel.app.ui.privacy

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.OpenInNew
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
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.theme.AnpContainerDefaults

/** Public static documents: opening a link never submits consent or erasure. */
internal object PrivacyDocumentLinks {
    const val POLICY = "https://teste.abastevo.com.br/privacidade/"
    const val DELETION = "https://teste.abastevo.com.br/excluir-conta/"
}

@Composable
internal fun PrivacyDocumentLinksCard(onOpen: (String) -> Unit, modifier: Modifier = Modifier) {
    var failedUrl by rememberSaveable { mutableStateOf<String?>(null) }
    val openDocument: (String) -> Unit = { url ->
        failedUrl = null
        try {
            onOpen(url)
        } catch (_: IllegalArgumentException) {
            // Android UriHandler reports an absent browser with this exception.
            failedUrl = url
        }
    }
    val color = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = CardDefaults.shape,
        colors = CardDefaults.cardColors(containerColor = color),
        border = AnpContainerDefaults.borderStroke(color),
    ) {
        Column(Modifier.fillMaxWidth().padding(12.dp)) {
            TextButton(
                onClick = { openDocument(PrivacyDocumentLinks.POLICY) },
                modifier = Modifier.fillMaxWidth().testTag("privacy-full-policy"),
            ) {
                Text(stringResource(R.string.privacy_full_policy), modifier = Modifier.weight(1f))
                Icon(Icons.Default.OpenInNew, contentDescription = null)
            }
            TextButton(
                onClick = { openDocument(PrivacyDocumentLinks.DELETION) },
                modifier = Modifier.fillMaxWidth().testTag("privacy-external-deletion"),
            ) {
                Text(stringResource(R.string.privacy_external_deletion), modifier = Modifier.weight(1f))
                Icon(Icons.Default.OpenInNew, contentDescription = null)
            }
            failedUrl?.let { url ->
                Text(
                    text = stringResource(R.string.privacy_document_open_failed, url),
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.testTag("privacy-document-error")
                        .semantics { liveRegion = LiveRegionMode.Polite },
                )
            }
        }
    }
}
