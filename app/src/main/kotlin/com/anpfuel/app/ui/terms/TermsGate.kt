package com.anpfuel.app.ui.terms

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.usecase.legal.TermsAcceptanceUseCase
import com.anpfuel.application.usecase.legal.TermsStatus
import com.anpfuel.domain.legal.LegalDocuments
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/** Public documents the first-use screen links to; opening a link accepts nothing. */
internal object TermsDocumentLinks {
    const val TERMS = "https://teste.abastevo.com.br/termos/"
    const val PRIVACY = "https://teste.abastevo.com.br/privacidade/"
}

/**
 * First-use terms gate state (B-BR-TA01..TA04). The tick lives here so it
 * survives rotation, is never pre-set and is dropped after every decision.
 */
@HiltViewModel
class TermsGateViewModel @Inject constructor(
    private val terms: TermsAcceptanceUseCase,
) : ViewModel() {
    val status: StateFlow<TermsStatus> = terms.status

    private val _ticked = MutableStateFlow(false)
    val ticked: StateFlow<Boolean> = _ticked.asStateFlow()

    private val _failed = MutableStateFlow(false)
    val failed: StateFlow<Boolean> = _failed.asStateFlow()

    fun tick(value: Boolean) {
        _ticked.value = value
        _failed.value = false
    }

    fun accept() {
        if (!_ticked.value) return
        _failed.value = !terms.accept(ticked = true)
        _ticked.value = false
    }

    /** True when the record is gone; the gate then replaces the app content. */
    fun revoke(): Boolean {
        _ticked.value = false
        _failed.value = false
        return terms.revoke()
    }
}

/**
 * Composes [content] only after the current terms were accepted (B-BR-TA01).
 * Until then the terms screen is the whole app; back and "decline" leave it.
 */
@Composable
fun TermsGate(
    onDecline: () -> Unit,
    viewModel: TermsGateViewModel = hiltViewModel(),
    content: @Composable () -> Unit,
) {
    val status by viewModel.status.collectAsStateWithLifecycle()
    when (val current = status) {
        is TermsStatus.Accepted -> content()
        is TermsStatus.Required -> {
            val ticked by viewModel.ticked.collectAsStateWithLifecycle()
            val failed by viewModel.failed.collectAsStateWithLifecycle()
            BackHandler(onBack = onDecline)
            TermsAcceptanceScreen(
                documents = current.documents,
                changed = current.changed,
                ticked = ticked,
                failed = failed,
                onTick = viewModel::tick,
                onAccept = viewModel::accept,
                onDecline = onDecline,
            )
        }
    }
}

@Composable
internal fun TermsAcceptanceScreen(
    documents: LegalDocuments,
    changed: Boolean,
    ticked: Boolean,
    failed: Boolean,
    onTick: (Boolean) -> Unit,
    onAccept: () -> Unit,
    onDecline: () -> Unit,
    modifier: Modifier = Modifier,
    onOpen: ((String) -> Unit)? = null,
) {
    val uriHandler = LocalUriHandler.current
    var failedUrl by rememberSaveable { mutableStateOf<String?>(null) }
    val open: (String) -> Unit = { url ->
        failedUrl = null
        try {
            (onOpen ?: uriHandler::openUri)(url)
        } catch (_: IllegalArgumentException) {
            // Android UriHandler reports an absent browser with this exception.
            failedUrl = url
        }
    }
    Surface(modifier = modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .safeDrawingPadding()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Column(
                modifier = Modifier.widthIn(max = 560.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Text(
                    text = stringResource(R.string.terms_gate_title),
                    style = MaterialTheme.typography.headlineMedium,
                    modifier = Modifier.semantics { heading() },
                )
                Text(
                    text = stringResource(R.string.terms_gate_intro),
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (changed) {
                    Text(
                        text = stringResource(R.string.terms_gate_changed),
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.tertiary,
                        modifier = Modifier.testTag("terms-changed"),
                    )
                }
                val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
                Card(
                    modifier = Modifier.fillMaxWidth(),
                    shape = AnpContainerDefaults.containerShape(),
                    colors = CardDefaults.cardColors(containerColor = containerColor),
                    border = AnpContainerDefaults.borderStroke(containerColor),
                ) {
                    Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                        listOf(
                            R.string.terms_gate_point_purpose,
                            R.string.terms_gate_point_community,
                            R.string.terms_gate_point_privacy,
                        ).forEach { point ->
                            Text(text = stringResource(point), style = MaterialTheme.typography.bodyMedium)
                        }
                        DocumentLink(R.string.terms_gate_read_terms, "terms-read-terms") { open(TermsDocumentLinks.TERMS) }
                        DocumentLink(R.string.terms_gate_read_privacy, "terms-read-privacy") { open(TermsDocumentLinks.PRIVACY) }
                        Text(
                            text = stringResource(R.string.terms_gate_versions, documents.termsVersion, documents.privacyVersion),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        failedUrl?.let { url ->
                            Text(
                                text = stringResource(R.string.terms_gate_open_failed, url),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.error,
                                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                            )
                        }
                    }
                }
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .toggleable(value = ticked, role = Role.Checkbox, onValueChange = onTick)
                        .padding(vertical = 8.dp)
                        .testTag("terms-check"),
                    verticalAlignment = Alignment.Top,
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    Checkbox(checked = ticked, onCheckedChange = null, modifier = Modifier.padding(top = 2.dp))
                    Text(
                        text = stringResource(R.string.terms_gate_check),
                        style = MaterialTheme.typography.bodyLarge,
                        modifier = Modifier.weight(1f),
                    )
                }
                Text(
                    text = stringResource(R.string.terms_gate_optional_note),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (failed) {
                    Text(
                        text = stringResource(R.string.terms_gate_failed),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                    )
                }
                Button(
                    onClick = onAccept,
                    enabled = ticked,
                    modifier = Modifier.fillMaxWidth().testTag("terms-accept"),
                ) { Text(stringResource(R.string.terms_gate_accept)) }
                TextButton(onClick = onDecline, modifier = Modifier.fillMaxWidth().testTag("terms-decline")) {
                    Text(stringResource(R.string.terms_gate_decline))
                }
            }
        }
    }
}

@Composable
private fun DocumentLink(label: Int, tag: String, onClick: () -> Unit) {
    TextButton(onClick = onClick, modifier = Modifier.fillMaxWidth().testTag(tag)) {
        Text(stringResource(label), modifier = Modifier.weight(1f))
        Icon(Icons.AutoMirrored.Filled.OpenInNew, contentDescription = null)
    }
}
