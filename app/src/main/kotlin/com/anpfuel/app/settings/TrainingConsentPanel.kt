package com.anpfuel.app.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpContainerDefaults

/**
 * CS11B — Training-consent settings panel (B-BR-CS06, BUC-CS05).
 *
 * `Configurações > Privacidade > Melhorar o reconhecimento de
 * preços`: the switch grants (ON) or revokes (OFF) with the reviewed
 * terms the host passes in; revocation starts owned training-asset
 * deletion. Server state, terms/version, pending offline revocation
 * and the rights entry render from [TrainingConsentUiState.Content];
 * the switch never shows a claim the server does not hold (the host
 * reloads after every decision). Guest renders nothing (caller keeps
 * surrounding settings intact) — account state owns the login
 * surface, never this panel. Single-flight updating shows progress
 * with no second tap target.
 */
@Composable
fun TrainingConsentPanel(
    state: TrainingConsentUiState,
    onToggle: (enabled: Boolean) -> Unit,
    onRetry: () -> Unit,
    onSignIn: () -> Unit,
    modifier: Modifier = Modifier,
) {
    when (state) {
        is TrainingConsentUiState.Guest -> Unit
        is TrainingConsentUiState.Loading -> {
            CircularProgressIndicator(modifier = modifier.padding(16.dp))
        }
        is TrainingConsentUiState.Content -> {
            Card(
                modifier = modifier.fillMaxWidth(),
                border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerHighest),
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.training_consent_title),
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.semantics { heading() },
                    )
                    Text(
                        text = stringResource(R.string.training_consent_body),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    val description = trainingConsentTalkBackDescription(
                        state.granted,
                        state.state,
                        state.termsVersion,
                        state.pendingRevocation,
                    )
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(
                            text = stringResource(
                                if (state.granted) {
                                    R.string.training_consent_on
                                } else {
                                    R.string.training_consent_off
                                },
                            ),
                            style = MaterialTheme.typography.bodyLarge,
                            modifier = Modifier.semantics { contentDescription = description },
                        )
                        Switch(
                            checked = state.granted,
                            onCheckedChange = onToggle,
                        )
                    }
                    if (state.termsVersion != null) {
                        Text(
                            text = stringResource(R.string.training_consent_terms, state.termsVersion),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    if (state.needsRegrant) {
                        Text(
                            text = stringResource(R.string.training_consent_regrant),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    if (state.pendingRevocation) {
                        Text(
                            text = stringResource(R.string.training_consent_pending),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    Text(
                        text = stringResource(R.string.training_consent_delete_note),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    TextButton(onClick = onSignIn) {
                        Text(text = stringResource(R.string.training_consent_rights))
                    }
                }
            }
        }
        is TrainingConsentUiState.Updating -> {
            CircularProgressIndicator(modifier = modifier.padding(16.dp))
        }
        is TrainingConsentUiState.Offline -> {
            Card(
                modifier = modifier.fillMaxWidth(),
                border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerHighest),
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.training_consent_offline),
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.semantics { heading() },
                    )
                    Button(onClick = onRetry, modifier = Modifier.fillMaxWidth()) {
                        Text(text = stringResource(R.string.action_retry))
                    }
                }
            }
        }
        is TrainingConsentUiState.Pending -> {
            Card(
                modifier = modifier.fillMaxWidth(),
                border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerHighest),
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.training_consent_pending),
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.semantics { heading() },
                    )
                    Button(onClick = onRetry, modifier = Modifier.fillMaxWidth()) {
                        Text(text = stringResource(R.string.action_retry))
                    }
                }
            }
        }
        is TrainingConsentUiState.Rejected -> {
            Card(
                modifier = modifier.fillMaxWidth(),
                border = AnpContainerDefaults.borderStroke(MaterialTheme.colorScheme.surfaceContainerHighest),
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = stringResource(R.string.training_consent_rejected, state.kind.name),
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.semantics { heading() },
                    )
                    Text(
                        text = state.message,
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Button(onClick = onRetry, modifier = Modifier.fillMaxWidth()) {
                        Text(text = stringResource(R.string.action_retry))
                    }
                }
            }
        }
    }
}

/**
 * CS11B TalkBack description: switch state plus the server state
 * and terms line, with the pending notice when queued — never a
 * bare toggle alone. Pure for unit tests; large text scales through
 * MaterialTheme typography with no fixed heights in the panel above.
 */
internal fun trainingConsentTalkBackDescription(
    granted: Boolean,
    state: String?,
    termsVersion: String?,
    pendingRevocation: Boolean,
): String {
    val parts = mutableListOf(
        if (granted) {
            "Improve price recognition: on"
        } else {
            "Improve price recognition: off"
        },
    )
    if (!state.isNullOrBlank()) {
        parts += state
    }
    if (!termsVersion.isNullOrBlank()) {
        parts += "Terms $termsVersion"
    }
    if (pendingRevocation) {
        parts += "Revocation pending"
    }
    return parts.joinToString(", ")
}
